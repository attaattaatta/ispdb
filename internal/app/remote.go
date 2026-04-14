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

	runner := &remoteRunner{client: client, sftpClient: sftpClient, logger: logger, ui: ui, force: cfg.Force}
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

	for _, command := range commands {
		rewrittenCommand := command
		if runner.primaryIP == "" {
			primaryIP, ipErr := runner.primaryIPAddress(ctx)
			if ipErr == nil {
				runner.primaryIP = primaryIP
			}
		}
		if runner.primaryIP != "" {
			rewrittenCommand = rewriteCommandForRemoteIP(command, runner.primaryIP)
		}

		progressID := randomProgressID()
		commandWithProgress := addProgressID(rewrittenCommand, progressID)
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
		if runErr != nil && isAlreadyExistsOutput(output) {
			ui.Warn("entity already exists on remote side, skipped.")
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
	etcSize, err := r.directoryTreeSize("/etc")
	if err != nil {
		return "", fmt.Errorf("%sfailed to calculate /etc size: %w%s", colorRed, err, colorReset)
	}
	mgrSize, err := r.directoryTreeSize("/usr/local/mgr5")
	if err != nil {
		return "", fmt.Errorf("%sfailed to calculate /usr/local/mgr5 size: %w%s", colorRed, err, colorReset)
	}
	sizeValue := etcSize + mgrSize

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
	command := fmt.Sprintf("sh -lc %s", shellQuote(fmt.Sprintf("mkdir -p %s && cp -a /etc %s/etc && cp -a /usr/local/mgr5 %s/mgr5", target, target, target)))
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

func (r *remoteRunner) checkLogProgress(ctx context.Context, progressID string) error {
	command := fmt.Sprintf("sh -lc %s", shellQuote(fmt.Sprintf("grep -F %q /usr/local/mgr5/var/ispmgr.log | tail -n 20", progressID)))
	output, _ := r.run(ctx, command)
	if strings.TrimSpace(output) != "" {
		r.logger.Debug("panel log check", "progressid", progressID, "output", output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		upper := strings.ToUpper(line)
		if strings.Contains(upper, " ERROR ") || strings.Contains(upper, " CRIT ") || strings.Contains(line, "return code: 1") || strings.Contains(line, "exit code 1") {
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
