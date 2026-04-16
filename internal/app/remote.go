package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type remoteRunner struct {
	client     *ssh.Client
	sftpClient *sftp.Client
	logger     *slog.Logger
	ui         *UI
	force      bool
	primaryIP  string
	cfg        Config
}

func runRemoteWorkflow(ctx context.Context, ui *UI, logger *slog.Logger, cfg Config, data SourceData, commands []string) error {
	ui.Info("connecting: " + cfg.DestHost)
	client, err := connectSSH(cfg)
	if err != nil {
		return fmt.Errorf("%sSSH connection failed: %w%s", colorRed, err, colorReset)
	}
	defer client.Close()
	ui.Success("connecting: OK")

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("%sfailed to open SFTP session: %w%s", colorRed, err, colorReset)
	}
	defer sftpClient.Close()

	runner := &remoteRunner{client: client, sftpClient: sftpClient, logger: logger, ui: ui, force: cfg.Force, cfg: cfg}

	if err := runner.warnOnMemoryMismatch(ctx); err != nil {
		logger.Debug("memory check skipped", "error", err)
	}

	panelInstalled, err := runner.panelInstalled(ctx)
	if err != nil {
		return err
	}
	if !panelInstalled {
		ui.Warn("destination server does not have ispmanager installed.")
		install := cfg.Force
		if !cfg.Force {
			install, err = askYesNoWithColor("ispmanager was not found on destination server. Install it?", true, colorYellow)
			if err != nil {
				return err
			}
		}
		if !install {
			return fmt.Errorf("%sdestination server does not have ispmanager installed%s", colorRed, colorReset)
		}
		if err := runner.installPanel(ctx, data.Packages); err != nil {
			return err
		}
		ui.Success("ispmanager installation on remote side: OK")
	}

	backupPath, err := runner.prepareBackup(ctx)
	if err != nil {
		return err
	}
	ui.Success("backup path on remote side: " + backupPath)

	licInfo, err := runner.licenseInfo(ctx)
	if err != nil {
		return err
	}
	if err := validateLicense(licInfo, len(data.WebDomains)); err != nil {
		return err
	}
	ui.Success("validating ispmanager licence on remote side: OK")

	if err := runner.waitForFeaturesIdle(ctx); err != nil {
		return err
	}
	records, err := runner.featureRecords(ctx)
	if err != nil {
		return err
	}
	currentPackages := installedPackagesFromFeatures(records, licInfo["os"])
	packageSteps, packageWarnings := buildPackageSyncSteps(data.Packages, currentPackages, packagePlanOptions{
		TargetOS:         licInfo["os"],
		TargetPanel:      licInfo["panel_name"],
		NoDeletePackages: cfg.NoDeletePackages,
		SkipSatisfied:    true,
	})
	for _, warning := range packageWarnings {
		ui.Warn(warning)
	}
	for _, step := range packageSteps {
		if err := runner.runPackageStep(ctx, step, licInfo["os"]); err != nil {
			if !cfg.Force {
				return err
			}
			ui.Warn(err.Error())
		}
	}

	entityCommands := filterEntityCommands(commands)
	for _, command := range entityCommands {
		rewrittenCommand := command
		if runner.primaryIP == "" {
			primaryIP, ipErr := runner.primaryIPAddress(ctx)
			if ipErr == nil {
				runner.primaryIP = primaryIP
			}
		}
		if runner.primaryIP != "" && !cfg.NoChangeIPAddresses {
			rewrittenCommand = rewriteCommandForRemoteIP(command, runner.primaryIP)
		}

		progressID := randomProgressID()
		commandWithProgress := addProgressID(rewrittenCommand, progressID)
		ispmgrLines, _ := runner.logLineCount(ctx, "/usr/local/mgr5/var/ispmgr.log")
		pkgLines, _ := runner.logLineCount(ctx, "/usr/local/mgr5/var/pkg.log")
		skip, skipReason, err := runner.shouldSkipCommand(ctx, rewrittenCommand)
		if err != nil {
			return err
		}
		if skip {
			ui.Warn(skipReason)
			continue
		}
		ui.Info("running command: " + commandWithProgress)
		output, runErr := runner.run(ctx, commandWithProgress)
		if strings.TrimSpace(output) != "" {
			ui.Println(strings.TrimSpace(output))
		}
		if retriedOutput, retriedErr, handled, retryErr := runner.retryCommandIfNeeded(ctx, rewrittenCommand, progressID, output, runErr); retryErr != nil {
			return retryErr
		} else if handled {
			output = retriedOutput
			runErr = retriedErr
			if strings.TrimSpace(output) != "" {
				ui.Println(strings.TrimSpace(output))
			}
		}
		if runErr != nil && isAlreadyExistsOutput(output) {
			ui.Warn("entity already exists on remote side, skipped.")
			continue
		}
		if runErr != nil && isDBServerAlreadyExistsOutput(rewrittenCommand, output) {
			ui.Warn("DB server already exists on remote side, skipped.")
			continue
		}
		if runErr != nil && isFeatureEditCommand(rewrittenCommand) {
			ui.Warn("feature edit form is not supported on remote side, skipped.")
			continue
		}
		if runErr != nil && isUnavailableFeatureOutput(output) {
			ui.Warn("feature is not available on remote side, skipped.")
			continue
		}
		logErr := runner.checkLogProgress(ctx, progressID)
		if logErr == nil {
			logErr = runner.checkRecentLogErrors(ctx, ispmgrLines, pkgLines)
		}
		if runErr != nil {
			if !cfg.Force {
				return fmt.Errorf("%sremote API command failed: %w%s", colorRed, runErr, colorReset)
			}
			ui.Warn("API error ignored because --force was used.")
		}
		if logErr != nil {
			if !cfg.Force {
				return logErr
			}
			ui.Warn("Panel log error ignored because --force was used.")
		}
		ui.Success("command result: OK")
	}

	if cfg.CopyConfigs {
		if err := runner.copyConfigs(ctx); err != nil {
			if !cfg.Force {
				return err
			}
			ui.Warn("configuration copy error ignored because --force was used.")
		}
	}

	return nil
}

func buildSSHAuthMethods(auth string) ([]ssh.AuthMethod, error) {
	if auth == "" {
		methods := make([]ssh.AuthMethod, 0)
		for _, path := range discoverSSHIdentityFiles() {
			method, err := publicKeyAuth(path)
			if err == nil {
				methods = append(methods, method)
			}
		}
		return methods, nil
	}

	if strings.HasPrefix(auth, "/") || strings.HasPrefix(auth, "~") {
		method, err := publicKeyAuth(expandHome(auth))
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{method}, nil
	}

	if _, err := os.Stat(auth); err == nil {
		method, err := publicKeyAuth(auth)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{method}, nil
	}

	return []ssh.AuthMethod{ssh.Password(auth)}, nil
}

func connectSSH(cfg Config) (*ssh.Client, error) {
	authMethods, err := buildSSHAuthMethods(cfg.DestAuth)
	if err != nil {
		return nil, err
	}

	tryDial := func(methods []ssh.AuthMethod) (*ssh.Client, error) {
		if len(methods) == 0 {
			return nil, fmt.Errorf("no usable SSH authentication method was found")
		}
		return ssh.Dial("tcp", net.JoinHostPort(cfg.DestHost, strconv.Itoa(cfg.DestPort)), &ssh.ClientConfig{
			User:            "root",
			Auth:            methods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         15 * time.Second,
		})
	}

	client, err := tryDial(authMethods)
	if err == nil {
		return client, nil
	}

	if cfg.DestAuth == "" {
		password, askErr := askSecret("Enter SSH password for root:")
		if askErr != nil {
			return nil, askErr
		}
		return tryDial([]ssh.AuthMethod{ssh.Password(password)})
	}

	return nil, err
}

func (r *remoteRunner) panelInstalled(ctx context.Context) (bool, error) {
	output, err := r.run(ctx, "sh -lc 'if [ -x /usr/local/mgr5/sbin/mgrctl ]; then echo yes; else echo no; fi'")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "yes", nil
}

func (r *remoteRunner) installPanel(ctx context.Context, packages []Package) error {
	if err := r.ensureWget(ctx); err != nil {
		return err
	}
	command := "INSTALL_MINI=yes bash <(timeout 4 wget --timeout 4 --no-check-certificate -q -O- https://download.ispmanager.com/install.sh) --ignore-hostname --dbtype mysql --release stable --ispmgr6 ispmanager-lite-common"
	if packageSetHas(packages, "mariadb-server") {
		command += " --mysql-server mariadb"
	}
	output, err := r.run(ctx, "bash -lc "+shellQuote(command))
	if strings.TrimSpace(output) != "" {
		r.ui.Println(strings.TrimSpace(output))
	}
	if err != nil {
		return fmt.Errorf("%sfailed to install ispmanager on destination: %w%s", colorRed, err, colorReset)
	}
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		installed, checkErr := r.panelInstalled(ctx)
		if checkErr == nil && installed {
			if _, infoErr := r.licenseInfo(ctx); infoErr == nil {
				return nil
			}
		}
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("%sispmanager installation did not finish in time%s", colorRed, colorReset)
}

func (r *remoteRunner) ensureWget(ctx context.Context) error {
	output, err := r.run(ctx, "sh -lc 'command -v wget >/dev/null 2>&1 && echo yes || echo no'")
	if err == nil && strings.TrimSpace(output) == "yes" {
		return nil
	}
	install := "sh -lc 'if command -v apt-get >/dev/null 2>&1; then export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install -y wget; " +
		"elif command -v dnf >/dev/null 2>&1; then dnf install -y wget; " +
		"elif command -v yum >/dev/null 2>&1; then yum install -y wget; " +
		"else echo unsupported-package-manager; exit 1; fi'"
	if _, err := r.run(ctx, install); err != nil {
		return fmt.Errorf("%sfailed to install wget on destination server: %w%s", colorRed, err, colorReset)
	}
	return nil
}

func publicKeyAuth(path string) (ssh.AuthMethod, error) {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(content)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

func (r *remoteRunner) run(ctx context.Context, command string) (string, error) {
	r.logger.Debug("ssh command", "command", command)
	session, err := r.client.NewSession()
	if err != nil {
		r.logger.Error("failed to create SSH session", "error", err)
		return "", err
	}
	defer session.Close()

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := session.CombinedOutput(command)
		ch <- result{data: data, err: err}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		r.logger.Error("ssh command cancelled", "command", command, "error", ctx.Err())
		return "", ctx.Err()
	case res := <-ch:
		if strings.TrimSpace(string(res.data)) != "" {
			r.logger.Debug("ssh output", "command", command, "output", string(res.data))
		}
		if res.err != nil {
			r.logger.Error("ssh command failed", "command", command, "error", res.err)
		}
		return string(res.data), res.err
	}
}

func (r *remoteRunner) prepareBackup(ctx context.Context) (string, error) {
	paths := []string{"/etc", "/usr/local/mgr5"}
	sizeValue := int64(0)
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		ok, err := r.dirExists(path)
		if err != nil {
			return "", fmt.Errorf("%sfailed to inspect %s: %w%s", colorRed, path, err, colorReset)
		}
		if !ok {
			continue
		}
		existing = append(existing, path)
		value, err := r.directoryTreeSize(path)
		if err != nil {
			return "", fmt.Errorf("%sfailed to calculate %s size: %w%s", colorRed, path, err, colorReset)
		}
		sizeValue += value
	}
	if len(existing) == 0 {
		return "", fmt.Errorf("%snothing was found to back up on destination server%s", colorRed, colorReset)
	}

	vfs, err := r.sftpClient.StatVFS("/root")
	if err != nil {
		return "", fmt.Errorf("%sfailed to calculate destination free space: %w%s", colorRed, err, colorReset)
	}
	freeValue := int64(vfs.FreeSpace())

	required := sizeValue + sizeValue/10
	if freeValue < required {
		return "", fmt.Errorf("%snot enough free space on destination server for backup%s", colorRed, colorReset)
	}

	stamp := time.Now().UTC().Format("02-Jan-2006-15-04-MST")
	target := "/root/support/" + stamp
	copyParts := []string{fmt.Sprintf("mkdir -p %s", shellQuote(target))}
	for _, path := range existing {
		base := filepath.Base(path)
		copyParts = append(copyParts, fmt.Sprintf("cp -a %s %s", shellQuote(path), shellQuote(target+"/"+base)))
	}
	command := fmt.Sprintf("sh -lc %s", shellQuote(strings.Join(copyParts, " && ")))
	if _, err := r.run(ctx, command); err != nil {
		r.logger.Warn("backup creation failed", "target", target, "error", err)
		answer, askErr := askYesNo("Backup failed. Continue anyway?", true)
		if askErr != nil {
			return "", askErr
		}
		if !answer {
			return "", fmt.Errorf("%sbackup failed and execution was cancelled%s", colorRed, colorReset)
		}
		r.ui.Warn("Continuing without a successful backup.")
	}
	return target, nil
}

func (r *remoteRunner) dirExists(path string) (bool, error) {
	info, err := r.sftpClient.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (r *remoteRunner) directoryTreeSize(root string) (int64, error) {
	r.logger.Debug("sftp size scan started", "path", root)
	walker := r.sftpClient.Walk(root)
	var total int64
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return 0, err
		}
		info := walker.Stat()
		if info == nil || info.IsDir() {
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			continue
		}
		total += info.Size()
	}
	r.logger.Debug("sftp size scan finished", "path", root, "bytes", total)
	return total, nil
}

func (r *remoteRunner) licenseInfo(ctx context.Context) (map[string]string, error) {
	output, err := r.run(ctx, "/usr/local/mgr5/sbin/mgrctl -m ispmgr license.info")
	if err != nil {
		return nil, fmt.Errorf("%slicense.info failed: %w; output: %s%s", colorRed, err, strings.TrimSpace(output), colorReset)
	}
	r.logger.Debug("license info received", "output", output)
	info := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		info[parts[0]] = parts[1]
	}
	return info, nil
}

func validateLicense(info map[string]string, webDomainCount int) error {
	panelName := info["panel_name"]
	if panelName == "" {
		return fmt.Errorf("%spanel_name was not returned by license.info%s", colorRed, colorReset)
	}
	if strings.Contains(strings.ToLower(panelName), "business") {
		return fmt.Errorf("%sdestination panel edition is not supported: %s%s", colorRed, panelName, colorReset)
	}
	allowed := []string{"lite", "pro", "host"}
	ok := false
	for _, item := range allowed {
		if strings.Contains(strings.ToLower(panelName), item) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("%sdestination panel edition is not supported: %s%s", colorRed, panelName, colorReset)
	}

	versionSource := firstNonEmpty(info["core_info"], info["panel_info"], info["repository"])
	if !strings.Contains(versionSource, "5.") && !strings.Contains(versionSource, "6-5.") {
		return fmt.Errorf("%sdestination panel version is lower than ispmanager 5%s", colorRed, colorReset)
	}

	if limitText := strings.TrimSpace(info["webdomain_license_limit"]); limitText != "" {
		if limit, err := strconv.Atoi(limitText); err == nil && limit > 0 && webDomainCount > limit {
			return fmt.Errorf("%sdestination licence allows %d web domains, but source requires %d%s", colorRed, limit, webDomainCount, colorReset)
		}
	}
	return nil
}

func (r *remoteRunner) featureRecords(ctx context.Context) ([]featureRecord, error) {
	output, err := r.run(ctx, "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")
	if err != nil {
		return nil, fmt.Errorf("%sfeature list request failed: %w; output: %s%s", colorRed, err, strings.TrimSpace(output), colorReset)
	}
	return parseFeatureRecords(output), nil
}

func (r *remoteRunner) runPackageStep(ctx context.Context, step packageSyncStep, targetOS string) error {
	r.ui.Info("running package step: " + step.Title)
	ispmgrLines, _ := r.logLineCount(ctx, "/usr/local/mgr5/var/ispmgr.log")
	pkgLines, _ := r.logLineCount(ctx, "/usr/local/mgr5/var/pkg.log")
	if err := r.runFeatureUpdate(ctx); err != nil {
		return err
	}
	progressID := randomProgressID()
	command, err := r.pruneFeatureCommand(ctx, step.Command)
	if err != nil {
		return err
	}
	command = addProgressID(command, progressID)
	output, runErr := r.run(ctx, command)
	if strings.TrimSpace(output) != "" {
		r.ui.Println(strings.TrimSpace(output))
	}
	if runErr != nil {
		return fmt.Errorf("%spackage step failed (%s): %w%s", colorRed, step.Title, runErr, colorReset)
	}
	if err := r.checkLogProgress(ctx, progressID); err != nil {
		return err
	}
	if err := r.checkRecentLogErrors(ctx, ispmgrLines, pkgLines); err != nil {
		return err
	}
	if err := r.waitFeatureStep(ctx, step, targetOS); err != nil {
		return err
	}
	r.ui.Success(step.Title + ": OK")
	return nil
}

func (r *remoteRunner) runFeatureUpdate(ctx context.Context) error {
	progressID := randomProgressID()
	command := addProgressID(featureUpdateCommand(), progressID)
	output, err := r.run(ctx, command)
	if strings.TrimSpace(output) != "" {
		r.logger.Debug("feature.update output", "output", output)
	}
	if err != nil {
		return fmt.Errorf("%sfeature.update failed: %w%s", colorRed, err, colorReset)
	}
	if err := r.checkLogProgress(ctx, progressID); err != nil {
		return err
	}
	return nil
}

func (r *remoteRunner) waitFeatureStep(ctx context.Context, step packageSyncStep, targetOS string) error {
	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		records, err := r.featureRecords(ctx)
		if err != nil {
			return err
		}
		record := findFeatureRecord(records, step.Feature)
		if strings.EqualFold(record.Status, "install") {
			time.Sleep(5 * time.Second)
			continue
		}
		if len(step.ExpectedPackages) == 0 {
			return nil
		}
		if packageSubsetPresent(installedPackagesFromFeatures(records, targetOS), step.ExpectedPackages) {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("%sfeature step %s did not finish in time%s", colorRed, step.Title, colorReset)
}

func (r *remoteRunner) waitForFeaturesIdle(ctx context.Context) error {
	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		records, err := r.featureRecords(ctx)
		if err != nil {
			return err
		}
		busy := false
		for _, record := range records {
			if strings.EqualFold(record.Status, "install") {
				busy = true
				break
			}
		}
		if !busy {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("%sdestination panel still has unfinished feature installation tasks%s", colorRed, colorReset)
}

func (r *remoteRunner) pruneFeatureCommand(ctx context.Context, command string) (string, error) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "feature.edit" {
		return command, nil
	}
	elid := strings.TrimSpace(params["elid"])
	if elid == "" {
		return command, nil
	}
	output, err := r.run(ctx, buildMgrctlCommand("feature.edit", map[string]string{
		"elid": elid,
		"out":  "text",
	}))
	if err != nil {
		return command, nil
	}
	form := parseFeatureForm(output)
	if len(form) == 0 {
		return command, nil
	}
	pruned := map[string]string{}
	for key, value := range params {
		switch key {
		case "elid", "out", "sok":
			pruned[key] = value
		default:
			if _, ok := form[key]; ok {
				pruned[key] = value
			}
		}
	}
	return buildMgrctlCommand(function, pruned), nil
}

func (r *remoteRunner) retryCommandIfNeeded(ctx context.Context, command string, progressID string, output string, runErr error) (string, error, bool, error) {
	if runErr == nil {
		return output, runErr, false, nil
	}

	if retried, retriedErr, handled := retryWithoutInvalidUserParam(ctx, r, command, progressID, output); handled {
		return retried, retriedErr, true, nil
	}

	if retried, retriedErr, handled, err := retryDBServerAfterDockerInstall(ctx, r, command, progressID, output); err != nil {
		return "", err, true, err
	} else if handled {
		return retried, retriedErr, true, nil
	}

	return output, runErr, false, nil
}

func retryWithoutInvalidUserParam(ctx context.Context, r *remoteRunner, command string, progressID string, output string) (string, error, bool) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "user.edit" {
		return output, nil, false
	}
	if !strings.Contains(output, "ERROR value(limit_php_cgi_version)") {
		return output, nil, false
	}
	r.logger.Warn("remote API rejected limit_php_cgi_version, retrying without it", "command", command, "output", output)
	delete(params, "limit_php_cgi_version")
	delete(params, "progressid")
	retryCommand := addProgressID(buildMgrctlCommand(function, params), progressID)
	retryOutput, retryErr := r.run(ctx, retryCommand)
	return retryOutput, retryErr, true
}

func retryDBServerAfterDockerInstall(ctx context.Context, r *remoteRunner, command string, progressID string, output string) (string, error, bool, error) {
	function, _, ok := parseMgrctlCommand(command)
	if !ok || function != "db.server.edit" {
		return output, nil, false, nil
	}
	if !strings.Contains(strings.ToLower(output), "notconfigured(nodocker)") {
		return output, nil, false, nil
	}
	r.logger.Warn("remote API reported nodocker for db.server.edit, enabling docker and retrying", "command", command, "output", output)
	if err := r.ensureDockerInstalled(ctx); err != nil {
		return "", err, true, err
	}
	retryCommand := addProgressID(command, progressID)
	retryOutput, retryErr := r.run(ctx, retryCommand)
	return retryOutput, retryErr, true, nil
}

func (r *remoteRunner) ensureDockerInstalled(ctx context.Context) error {
	step := packageSyncStep{
		Title:            "packages (docker)",
		Feature:          "docker",
		Command:          buildMgrctlCommand("feature.edit", map[string]string{"sok": "ok", "elid": "docker", "package_docker": "on"}),
		ExpectedPackages: []string{"docker"},
	}
	return r.runPackageStep(ctx, step, "")
}

func findFeatureRecord(records []featureRecord, name string) featureRecord {
	for _, record := range records {
		if record.Name == name {
			return record
		}
	}
	return featureRecord{}
}

func filterEntityCommands(commands []string) []string {
	filtered := make([]string, 0, len(commands))
	for _, command := range commands {
		function, params, ok := parseMgrctlCommand(command)
		if !ok {
			filtered = append(filtered, command)
			continue
		}
		if function == "feature.update" {
			continue
		}
		if function == "feature.edit" {
			elid := strings.TrimSpace(params["elid"])
			if isPackageFeatureElid(elid) {
				continue
			}
		}
		filtered = append(filtered, command)
	}
	return filtered
}

func isPackageFeatureElid(elid string) bool {
	if strings.HasPrefix(elid, "altphp") {
		return true
	}
	switch elid {
	case "web", "email", "dns", "ftp", "mysql", "postgresql", "phpmyadmin", "phppgadmin", "quota", "psacct", "fail2ban", "ansible", "nodejs", "python", "docker", "wireguard":
		return true
	default:
		return false
	}
}

func (r *remoteRunner) warnOnMemoryMismatch(ctx context.Context) error {
	localBytes, err := localMemoryTotalBytes()
	if err != nil {
		return err
	}
	remoteBytes, err := r.remoteMemoryTotalBytes(ctx)
	if err != nil {
		return err
	}
	const gib = int64(1024 * 1024 * 1024)
	if remoteBytes < 2*gib {
		r.ui.Warn(fmt.Sprintf("destination memory is low: %s", humanGiB(remoteBytes)))
		return nil
	}
	if localBytes-remoteBytes > 2*gib {
		r.ui.Warn(fmt.Sprintf("destination memory (%s) is more than 2 GiB lower than source memory (%s).", humanGiB(remoteBytes), humanGiB(localBytes)))
	}
	return nil
}

func localMemoryTotalBytes() (int64, error) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	return parseMeminfoTotal(content)
}

func (r *remoteRunner) remoteMemoryTotalBytes(ctx context.Context) (int64, error) {
	output, err := r.run(ctx, "cat /proc/meminfo")
	if err != nil {
		return 0, err
	}
	return parseMeminfoTotal([]byte(output))
}

func parseMeminfoTotal(content []byte) (int64, error) {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		return value * 1024, nil
	}
	return 0, fmt.Errorf("MemTotal was not found")
}

func humanGiB(value int64) string {
	return fmt.Sprintf("%.2f GiB", float64(value)/(1024*1024*1024))
}

func (r *remoteRunner) logLineCount(ctx context.Context, path string) (int, error) {
	output, err := r.run(ctx, "sh -lc "+shellQuote("if [ -f "+path+" ]; then wc -l < "+path+"; else echo 0; fi"))
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (r *remoteRunner) checkRecentLogErrors(ctx context.Context, ispmgrLines int, pkgLines int) error {
	readSince := func(path string, line int) (string, error) {
		if line < 1 {
			line = 1
		}
		command := fmt.Sprintf("sh -lc %s", shellQuote(fmt.Sprintf("if [ -f %s ]; then tail -n +%d %s; fi", path, line, path)))
		output, err := r.run(ctx, command)
		if err != nil {
			return "", err
		}
		return output, nil
	}

	combined := strings.Builder{}
	if output, err := readSince("/usr/local/mgr5/var/ispmgr.log", ispmgrLines); err == nil {
		combined.WriteString(output)
		combined.WriteByte('\n')
	}
	if output, err := readSince("/usr/local/mgr5/var/pkg.log", pkgLines); err == nil {
		combined.WriteString(output)
	}
	for _, line := range strings.Split(combined.String(), "\n") {
		if strings.Contains(line, " ERROR ") || strings.Contains(line, " CRIT ") || strings.Contains(line, "Failed to install package") || strings.Contains(line, "finished with status 1") || strings.Contains(line, "finished with status 2") || strings.Contains(line, "return code: 1") || strings.Contains(line, "exit code 1") {
			return fmt.Errorf("%srecent panel log error: %s%s", colorRed, strings.TrimSpace(line), colorReset)
		}
	}
	return nil
}

func (r *remoteRunner) copyConfigs(ctx context.Context) error {
	r.ui.Warn("--copy-configs currently copies only the main ispmanager configuration files.")
	return r.copyConfigFile(ctx, "/usr/local/mgr5/etc/ispmgr.conf")
}

func (r *remoteRunner) copyConfigFile(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !r.cfg.NoChangeIPAddresses && r.primaryIP != "" {
		sourceIP := detectLocalIPv4()
		if sourceIP != "" && sourceIP != "127.0.0.1" {
			content = []byte(strings.ReplaceAll(string(content), sourceIP, r.primaryIP))
		}
	}
	target, err := r.sftpClient.Create(path)
	if err != nil {
		return err
	}
	defer target.Close()
	if _, err := target.Write(content); err != nil {
		return err
	}
	return r.sftpClient.Chmod(path, info.Mode())
}

func (r *remoteRunner) checkLogProgress(ctx context.Context, progressID string) error {
	command := fmt.Sprintf("sh -lc %s", shellQuote(fmt.Sprintf("grep -F %q /usr/local/mgr5/var/ispmgr.log | tail -n 20", progressID)))
	output, _ := r.run(ctx, command)
	if strings.TrimSpace(output) != "" {
		r.logger.Debug("panel log check", "progressid", progressID, "output", output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.Contains(line, " ERROR ") || strings.Contains(line, " CRIT ") || strings.Contains(line, "return code: 1") || strings.Contains(line, "exit code 1") {
			return fmt.Errorf("%spanel log reported an error for progressid %s: %s%s", colorRed, progressID, strings.TrimSpace(line), colorReset)
		}
	}
	return nil
}

func addProgressID(command string, progressID string) string {
	if strings.Contains(command, "progressid=") {
		return command
	}
	return command + " " + shellQuote("progressid="+progressID)
}

func randomProgressID() string {
	buffer := make([]byte, 6)
	_, _ = rand.Read(buffer)
	return "ispdb_" + hex.EncodeToString(buffer)
}

func isAlreadyExistsOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "error exists(") ||
		strings.Contains(lower, "table_value_exists(") ||
		strings.Contains(lower, "already exists") ||
		strings.Contains(lower, "already exist")
}

func isDBServerAlreadyExistsOutput(command string, output string) bool {
	function, _, ok := parseMgrctlCommand(command)
	if !ok || function != "db.server.edit" {
		return false
	}
	lower := strings.ToLower(output)
	return strings.Contains(lower, "error value(version):") && strings.Contains(lower, "the 'action' field has invalid value")
}

func isUnavailableFeatureOutput(output string) bool {
	return strings.Contains(strings.ToLower(output), "missed(feature)")
}

func isFeatureEditCommand(command string) bool {
	function, _, ok := parseMgrctlCommand(command)
	return ok && function == "feature.edit"
}

func (r *remoteRunner) primaryIPAddress(ctx context.Context) (string, error) {
	output, err := r.run(ctx, "sh -lc 'set -- $(hostname -I); printf %s \"$1\"'")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (r *remoteRunner) shouldSkipCommand(ctx context.Context, command string) (bool, string, error) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok {
		return false, "", nil
	}

	switch function {
	case "db.server.edit":
		name := strings.TrimSpace(params["name"])
		if name == "" {
			return false, "", nil
		}
		output, err := r.run(ctx, buildMgrctlCommand("db.server.edit", map[string]string{
			"elid": name,
			"out":  "text",
		}))
		if err == nil && strings.Contains(output, "elid="+name) {
			return true, "DB server already exists on remote side, skipped.", nil
		}
	}

	return false, "", nil
}

func parseMgrctlCommand(command string) (string, map[string]string, bool) {
	fields, err := splitShellWords(command)
	if err != nil || len(fields) < 4 {
		return "", nil, false
	}
	if fields[0] != "/usr/local/mgr5/sbin/mgrctl" || fields[1] != "-m" || fields[2] != "ispmgr" {
		return "", nil, false
	}
	function := fields[3]
	params := map[string]string{}
	for _, field := range fields[4:] {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		params[key] = value
	}
	return function, params, true
}

func rewriteCommandForRemoteIP(command string, ip string) string {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || strings.TrimSpace(ip) == "" {
		return command
	}

	switch function {
	case "site.edit":
		params["site_ipaddrs"] = ip
		params["site_ssl_cert"] = "selfsigned"
	case "emaildomain.edit":
		params["ip"] = ip
		params["ipsrc"] = ip
	case "domain.edit":
		params["ip"] = ip
	}

	return buildMgrctlCommand(function, params)
}

func splitShellWords(input string) ([]string, error) {
	var result []string
	var builder strings.Builder
	inSingle := false

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		result = append(result, builder.String())
		builder.Reset()
	}

	for i := 0; i < len(input); i++ {
		ch := input[i]
		switch ch {
		case '\'':
			inSingle = !inSingle
		case ' ', '\t', '\n':
			if inSingle {
				builder.WriteByte(ch)
				continue
			}
			flush()
		default:
			builder.WriteByte(ch)
		}
	}

	if inSingle {
		return nil, fmt.Errorf("failed to parse command: unterminated quote")
	}
	flush()
	return result, nil
}
