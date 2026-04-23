package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	connectSSHHook             = connectSSH
	newSFTPClientHook          = sftp.NewClient
	localMemoryTotalBytesHook  = localMemoryTotalBytes
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) {
		return r.remoteMemoryTotalBytes(ctx)
	}
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return r.remoteSwapState(ctx)
	}
	ensureSwapfileHook = func(r *remoteRunner, ctx context.Context, state remoteSwapState) error {
		return r.ensureSwapfile(ctx, state)
	}
)

type remoteSwapState struct {
	HasOtherNonZRAMSwap bool
	SwapfileActive      bool
	FstabHasSwapfile    bool
	SwapfileExists      bool
}

const (
	swapfilePath      = "/swapfile"
	swapfileSizeBytes = int64(2 * 1024 * 1024 * 1024)
	swapfileFstabLine = "/swapfile                                 none                    swap    sw,pri=10              0 0"
)

type remoteRunner struct {
	client     *ssh.Client
	sftpClient *sftp.Client
	logger     *slog.Logger
	ui         *UI
	force      bool
	primaryIP  string
	panelName  string
	cfg        Config

	liteDockerPrepared bool
	inventory          *remoteInventory
	failures           []remoteFailure
	summarySuccesses   []string
	summaryWarnings    []string
	printedCommand     bool
	runOverride        func(context.Context, string, bool) (string, error)
}

func runRemoteWorkflow(ctx context.Context, ui *UI, logger *slog.Logger, cfg Config, data SourceData, commands []string) (err error) {
	runner, err := connectRemoteRunner(ui, logger, cfg, true)
	if err != nil {
		return err
	}
	return runRemoteWorkflowWithRunner(ctx, runner, data, commands)
}

func runRemoteWorkflowWithRunner(ctx context.Context, runner *remoteRunner, data SourceData, commands []string) (err error) {
	defer runner.Close()
	cfg := runner.cfg
	ui := runner.ui
	execScopes := destExecutionScopesFromValue(cfg.DestScope)
	if err := runner.ensureDestinationRoot(ctx); err != nil {
		return err
	}
	defer runner.printRemoteSummary(ctx, data, cfg.DestScope, err)

	var backupPath string
	if err := runner.runAction("creating backup on remote side", func() error {
		var backupErr error
		backupPath, backupErr = runner.prepareBackup(ctx)
		return backupErr
	}); err != nil {
		return err
	}
	if consoleLevelEnabled(cfg.LogLevel, "info") {
		runner.infoLine("backup path on remote side: " + backupPath)
	}

	if err := runner.warnOnMemoryMismatch(ctx); err != nil {
		return err
	}

	panelInstalled, err := runner.panelInstalled(ctx)
	if err != nil {
		return err
	}
	if !panelInstalled {
		runner.warnLine("destination server does not have ispmanager installed.")
		install := cfg.Force || cfg.AutoYes
		if !cfg.Force && !cfg.AutoYes {
			install, err = askYesNoWithColor("ispmanager was not found on destination server. Install it?", true, colorYellow)
			if err != nil {
				return err
			}
		}
		if !install {
			return fmt.Errorf("%sdestination server does not have ispmanager installed%s", colorRed, colorReset)
		}
		if err := runner.installPanel(ctx); err != nil {
			return err
		}
	}

	var licInfo map[string]string
	if err := runner.runAction("validating ispmanager licence on remote side", func() error {
		var licenseErr error
		licInfo, licenseErr = runner.licenseInfo(ctx)
		if licenseErr != nil {
			return licenseErr
		}
		return validateLicense(licInfo, len(data.WebDomains))
	}); err != nil {
		return err
	}
	runner.panelName = licInfo["panel_name"]

	if err := runner.runAction("waiting for package operations to finish on remote side", func() error {
		return runner.waitForFeaturesIdle(ctx)
	}); err != nil {
		return err
	}
	if consoleLevelEnabled(cfg.LogLevel, "info") {
		ui.Println("")
	}
	if hasScope(execScopes, "packages") {
		packageSteps, packageWarnings := buildPackageSyncSteps(data.Packages, map[string]struct{}{}, packagePlanOptions{
			TargetOS:         licInfo["os"],
			TargetPanel:      licInfo["panel_name"],
			NoDeletePackages: cfg.NoDeletePackages,
			SkipSatisfied:    false,
		})
		packageSteps = uniquePackageSteps(packageSteps)
		for _, warning := range packageWarnings {
			runner.warnPackageWarning(warning)
		}
		for _, step := range packageSteps {
			if err := runner.runPackageStepNoUpdate(ctx, step, licInfo["os"]); err != nil {
				runner.recordFailure(describePackageStep(step.Title), err)
				if !cfg.Force {
					return err
				}
				runner.warnLine(err.Error())
			} else {
				runner.recordSummarySuccess(describePackageStep(step.Title) + ": OK")
			}
		}
	}

	inventory, err := runner.loadRemoteInventory(ctx)
	if err != nil {
		return err
	}
	runner.inventory = inventory

	entityCommands := uniqueStringsPreserveOrder(filterEntityCommands(commands))
	for _, command := range entityCommands {
		sourceCommand := command
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
		if adjustedCommand, skipReason, err := runner.prepareExistingMySQLPasswordSync(ctx, data, rewrittenCommand); err != nil {
			runner.recordFailure(describeRemoteCommand(rewrittenCommand), err)
			return err
		} else {
			rewrittenCommand = adjustedCommand
			if skipReason != "" {
				runner.warnLine(skipReason)
				runner.recordSummaryWarning(skipReason)
				continue
			}
		}
		skip, skipReason, err := runner.shouldSkipCommand(ctx, rewrittenCommand)
		if err != nil {
			runner.recordFailure(describeRemoteCommand(rewrittenCommand), err)
			return err
		}
		if skip {
			runner.warnOverwriteBlock(skipReason)
			continue
		}
		if err := runner.prepareLiteDockerForAltDB(ctx, rewrittenCommand); err != nil {
			runner.recordFailure(describeRemoteCommand(rewrittenCommand), err)
			return err
		}
		prunedCommand, prunedNotes, err := runner.pruneEntityCommand(ctx, rewrittenCommand)
		if err != nil {
			runner.recordFailure(describeRemoteCommand(rewrittenCommand), err)
			return err
		}
		rewrittenCommand = prunedCommand
		for _, note := range prunedNotes {
			runner.logger.Info("remote command was adjusted to destination form", "note", note, "command", rewrittenCommand)
		}
		rewrittenCommand, err = runner.prepareRemoteSiteCommand(ctx, rewrittenCommand)
		if err != nil {
			runner.recordFailure(describeRemoteCommand(rewrittenCommand), err)
			return err
		}
		runner.logCommandRewrite(sourceCommand, rewrittenCommand)

		progressID := randomProgressID()
		commandWithProgress := addProgressID(rewrittenCommand, progressID)
		logCursor := runner.newRemoteLogCursor(ctx)
		action := describeRemoteCommand(rewrittenCommand)
		runner.printLaunchedCommand(commandWithProgress)
		output, runErr := runner.run(ctx, commandWithProgress)
		if strings.TrimSpace(output) != "" && consoleLevelEnabled(cfg.LogLevel, "info") {
			ui.Println(strings.TrimSpace(output))
		}
		if retriedOutput, retriedErr, handled, retryErr := runner.retryCommandIfNeeded(ctx, rewrittenCommand, progressID, output, runErr); retryErr != nil {
			runner.recordFailure(action, retryErr)
			return retryErr
		} else if handled {
			output = retriedOutput
			runErr = retriedErr
			if strings.TrimSpace(output) != "" && consoleLevelEnabled(cfg.LogLevel, "info") {
				ui.Println(strings.TrimSpace(output))
			}
		}
		if runErr != nil && isAlreadyExistsOutput(output) {
			runner.warnOverwriteBlock(
				action+": skipped (already exists)",
				"entity already exists on remote side, skipped. Run again with --overwrite to modify it.",
			)
			continue
		}
		if runErr != nil && isDBServerAlreadyExistsOutput(rewrittenCommand, output) {
			runner.warnOverwriteBlock(
				action+": skipped (already exists)",
				"DB server already exists on remote side, skipped. Run again with --overwrite to modify it.",
			)
			continue
		}
		if runErr != nil && isFeatureEditCommand(rewrittenCommand) {
			runner.failAction(action)
			runner.recordFailure(action, fmt.Errorf("feature edit form is not supported on remote side"))
			runner.warnLine("feature edit form is not supported on remote side, skipped.")
			continue
		}
		if runErr != nil && isUnavailableFeatureOutput(output) {
			runner.failAction(action)
			runner.recordFailure(action, fmt.Errorf("feature is not available on remote side"))
			runner.warnLine("feature is not available on remote side, skipped.")
			continue
		}
		progressErr := runner.checkLogProgress(ctx, progressID, logCursor.ispmgrOffset)
		logErr := error(nil)
		if progressErr != nil {
			if runErr != nil {
				runErr = progressErr
			} else {
				logErr = progressErr
			}
		}
		if logErr == nil {
			logErr = runner.checkRecentLogErrors(ctx, logCursor.ispmgrOffset, logCursor.pkgOffset)
		}
		if runErr != nil {
			runner.failAction(action)
			runner.recordFailure(action, runErr)
			if !cfg.Force {
				return fmt.Errorf("remote API command failed: %s%s%s", colorRed, strings.TrimSpace(runErr.Error()), colorReset)
			}
			runner.warnLine("API error ignored because --force was used.")
		}
		if logErr != nil {
			runner.failAction(action)
			runner.recordFailure(action, logErr)
			if !cfg.Force {
				return logErr
			}
			runner.warnLine("Panel log error ignored because --force was used.")
		}
		if runErr == nil && logErr == nil {
			if runner.inventory != nil {
				runner.inventory.applyCommand(rewrittenCommand)
			}
			runner.okAction(action)
			runner.recordSummarySuccess(action + ": OK")
		}
	}

	if cfg.CopyConfigs {
		if err := runner.runAction("copying supported configuration files", func() error {
			return runner.copyConfigs(ctx)
		}); err != nil {
			if !cfg.Force {
				return err
			}
			runner.warnLine("configuration copy error ignored because --force was used.")
		}
	}

	return nil
}

func connectRemoteRunner(ui *UI, logger *slog.Logger, cfg Config, announce bool) (*remoteRunner, error) {
	if announce && consoleLevelEnabled(cfg.LogLevel, "info") {
		ui.Info("connecting: " + cfg.DestHost)
		appendRemoteLogLine(cfg, "INFO", "connecting: "+cfg.DestHost)
	}
	client, err := connectSSHHook(cfg)
	if err != nil {
		return nil, fmt.Errorf("%sSSH connection failed: %w%s", colorRed, err, colorReset)
	}

	sftpClient, err := newSFTPClientHook(client)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("%sfailed to open SFTP session: %w%s", colorRed, err, colorReset)
	}
	if announce && consoleLevelEnabled(cfg.LogLevel, "info") {
		ui.Success("connecting: OK")
		appendRemoteLogLine(cfg, "INFO", "connecting: OK")
	}

	return &remoteRunner{
		client:     client,
		sftpClient: sftpClient,
		logger:     logger,
		ui:         ui,
		force:      cfg.Force,
		cfg:        cfg,
	}, nil
}

func (r *remoteRunner) Close() error {
	var closeErr error
	if r.sftpClient != nil {
		if err := r.sftpClient.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		r.sftpClient = nil
	}
	if r.client != nil {
		if err := r.client.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		r.client = nil
	}
	return closeErr
}

func (r *remoteRunner) ensureDestinationRoot(ctx context.Context) error {
	output, err := r.runQuiet(ctx, "id -u")
	if err != nil {
		return fmt.Errorf("%sfailed to verify root privileges on destination server: %w%s", colorRed, err, colorReset)
	}
	if !isRemoteRootUID(output) {
		return fmt.Errorf("%sdestination server root privileges are required%s", colorRed, colorReset)
	}
	return nil
}

func isRemoteRootUID(output string) bool {
	return strings.TrimSpace(output) == "0"
}

func (r *remoteRunner) runAction(action string, fn func() error) error {
	if err := fn(); err != nil {
		r.failAction(action)
		return err
	}
	r.okAction(action)
	return nil
}

func (r *remoteRunner) okAction(action string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "info") {
		r.ui.Success(action + ": OK")
	}
	appendRemoteLogLine(r.cfg, "INFO", action+": OK")
}

func (r *remoteRunner) failAction(action string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "error") {
		r.ui.Error(action + ": FAIL")
	}
	appendRemoteLogLine(r.cfg, "ERROR", action+": FAIL")
}

func (r *remoteRunner) recordFailure(action string, err error) {
	if err == nil {
		return
	}
	r.failures = append(r.failures, remoteFailure{
		Action: action,
		Reason: strings.TrimSpace(err.Error()),
	})
}

func (r *remoteRunner) recordSummaryWarning(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, existing := range r.summaryWarnings {
		if existing == text {
			return
		}
	}
	r.summaryWarnings = append(r.summaryWarnings, text)
}

func (r *remoteRunner) recordSummarySuccess(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, existing := range r.summarySuccesses {
		if existing == text {
			return
		}
	}
	r.summarySuccesses = append(r.summarySuccesses, text)
}

func (r *remoteRunner) infoLine(text string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "info") {
		r.ui.Info(text)
	}
	appendRemoteLogLine(r.cfg, "INFO", text)
}

func (r *remoteRunner) successLine(text string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "info") {
		r.ui.Success(text)
	}
	appendRemoteLogLine(r.cfg, "INFO", text)
}

func (r *remoteRunner) warnLine(text string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "warn") {
		r.ui.Warn(text)
	}
	appendRemoteLogLine(r.cfg, "WARN", text)
}

func (r *remoteRunner) warnPackageWarning(text string) {
	r.warnLine(text)
	if strings.Contains(strings.ToLower(text), "skipped") {
		r.recordSummaryWarning(text)
	}
	if consoleLevelEnabled(r.cfg.LogLevel, "info") && strings.Contains(text, "does not support Docker, package_docker command was skipped.") {
		r.ui.Println("")
	}
}

func (r *remoteRunner) warnOverwriteBlock(lines ...string) {
	if len(lines) == 0 {
		return
	}

	needsSpacing := false
	for _, line := range lines {
		if strings.Contains(line, "Run again with --overwrite to modify it.") {
			needsSpacing = consoleLevelEnabled(r.cfg.LogLevel, "info")
			break
		}
	}

	if needsSpacing {
		r.ui.Println("")
	}
	recorded := false
	for _, line := range lines {
		r.warnLine(line)
		if !recorded {
			r.recordSummaryWarning(line)
			recorded = true
		}
	}
	if needsSpacing {
		r.ui.Println("")
	}
}

func (r *remoteRunner) printLaunchedCommand(command string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "info") {
		if r.printedCommand {
			r.ui.Println("")
		}
		r.ui.Println("pushing command: " + command)
		r.printedCommand = true
	}
	appendRemoteLogLine(r.cfg, "INFO", "pushing command: "+command)
	if consoleLevelEnabled(r.cfg.LogLevel, "debug") {
		r.ui.Println("monitoring files: /usr/local/mgr5/var/ispmgr.log, /usr/local/mgr5/var/pkg.log")
	}
	appendRemoteLogLine(r.cfg, "DEBUG", "monitoring files: /usr/local/mgr5/var/ispmgr.log, /usr/local/mgr5/var/pkg.log")
}

func (r *remoteRunner) printMonitoringCommand(command string) {
	if consoleLevelEnabled(r.cfg.LogLevel, "info") {
		r.ui.Println("monitoring command: " + command)
	}
	appendRemoteLogLine(r.cfg, "INFO", "monitoring command: "+command)
	if consoleLevelEnabled(r.cfg.LogLevel, "debug") {
		r.ui.Println("monitoring files: /usr/local/mgr5/var/ispmgr.log, /usr/local/mgr5/var/pkg.log")
	}
	appendRemoteLogLine(r.cfg, "DEBUG", "monitoring files: /usr/local/mgr5/var/ispmgr.log, /usr/local/mgr5/var/pkg.log")
	if consoleLevelEnabled(r.cfg.LogLevel, "info") || consoleLevelEnabled(r.cfg.LogLevel, "debug") {
		r.ui.Println("")
	}
}

func (r *remoteRunner) logSSHCommandFailure(command string, err error) {
	if isExpectedProbeFailure(command) {
		r.logger.Debug("ssh probe command failed", "command", command, "error", err)
		return
	}
	r.logger.Error("ssh command failed", "command", command, "error", err)
}

func isExpectedProbeFailure(command string) bool {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "db.server.edit" {
		return false
	}
	if strings.TrimSpace(params["out"]) != "text" {
		return false
	}
	return strings.TrimSpace(params["elid"]) != ""
}

func (r *remoteRunner) logCommandRewrite(sourceCommand string, remoteCommand string) {
	if !consoleLevelEnabled(r.cfg.LogLevel, "debug") || strings.TrimSpace(sourceCommand) == strings.TrimSpace(remoteCommand) {
		return
	}

	diff := diffMgrctlCommands(sourceCommand, remoteCommand)
	r.ui.Println("source command: " + sourceCommand)
	r.ui.Println("destination command: " + remoteCommand)
	appendRemoteLogLine(r.cfg, "DEBUG", "source command: "+sourceCommand)
	appendRemoteLogLine(r.cfg, "DEBUG", "destination command: "+remoteCommand)
	if diff != "" {
		r.ui.Println("command diff: " + diff)
		appendRemoteLogLine(r.cfg, "DEBUG", "command diff: "+diff)
	}
}

func appendRemoteLogLine(cfg Config, level string, text string) {
	if strings.TrimSpace(cfg.LogFile) == "" || strings.TrimSpace(text) == "" {
		return
	}
	file, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "time=%s level=%s msg=%q\n", time.Now().Format(time.RFC3339Nano), strings.ToUpper(strings.TrimSpace(level)), text)
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

func (r *remoteRunner) installPanel(ctx context.Context) error {
	if err := r.runAction("installing wget on remote side", func() error {
		return r.ensureWget(ctx)
	}); err != nil {
		return err
	}
	command := "INSTALL_MINI=yes bash <(timeout 30 wget --timeout 30 --no-check-certificate -q -O- https://download.ispmanager.com/install.sh) --ignore-hostname --dbtype sqlite --release stable --ispmgr6 ispmanager-lite-common"
	if err := r.runAction("starting control panel installation", func() error {
		r.printLaunchedCommand("bash -lc " + shellQuote(command))
		suppressedCount := 0
		_, err := r.runStreamingWithTrace(ctx, "bash -lc "+shellQuote(command), true, func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return
			}
			if isBenignRemoteGrepWarning(trimmed) {
				suppressedCount++
				return
			}
			if consoleLevelEnabled(r.cfg.LogLevel, "info") {
				r.ui.Println(trimmed)
			}
		})
		if suppressedCount > 0 {
			r.warnLine(fmt.Sprintf("installer emitted %d non-fatal /proc/sys grep warnings, suppressed from console output.", suppressedCount))
		}
		if err != nil {
			return fmt.Errorf("%sfailed to install ispmanager on destination: %w%s", colorRed, err, colorReset)
		}
		return nil
	}); err != nil {
		return err
	}
	return r.runAction("waiting for control panel installation to finish", func() error {
		return r.waitForPanelReady(ctx)
	})
}

func (r *remoteRunner) ensureWget(ctx context.Context) error {
	output, err := r.run(ctx, "sh -lc 'command -v wget >/dev/null 2>&1 && echo yes || echo no'")
	if err == nil && strings.TrimSpace(output) == "yes" {
		return nil
	}
	install := buildWgetInstallCommand()
	r.printLaunchedCommand(install)
	installOutput, err := r.run(ctx, install)
	if err != nil {
		return fmt.Errorf("%sfailed to install wget on destination server: %w; output: %s%s", colorRed, err, strings.TrimSpace(installOutput), colorReset)
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

func remoteCommandTimeout(command string) time.Duration {
	if strings.Contains(command, "/usr/local/mgr5/sbin/mgrctl -m ispmgr") {
		return 5 * time.Minute
	}
	return 0
}

func normalizeRemoteCommandError(command string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("remote command timed out or stalled under load: %s", command)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || isSSHConnectionLostError(err) {
		return fmt.Errorf("SSH connection to destination server was lost; the server may have rebooted or become unavailable: %w", err)
	}
	return err
}

func isSSHConnectionLostError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, token := range []string{
		"connection reset by peer",
		"broken pipe",
		"closed network connection",
		"unexpected eof",
		"handshake failed",
		"connection refused",
		"no route to host",
		"connection timed out",
		"i/o timeout",
		"session is not active",
		"transport is closing",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func sanitizeRemoteConsoleOutput(output string) (string, int) {
	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	suppressed := 0
	for _, line := range lines {
		if isBenignRemoteGrepWarning(line) {
			suppressed++
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n"), suppressed
}

func isBenignRemoteGrepWarning(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToLower(line), "grep: /proc/sys/") {
		return false
	}
	for _, marker := range []string{
		"invalid argument",
		"permission denied",
		"input/output error",
	} {
		if strings.Contains(strings.ToLower(line), marker) {
			return true
		}
	}
	return false
}

func (r *remoteRunner) run(ctx context.Context, command string) (string, error) {
	return r.runWithTrace(ctx, command, true)
}

func (r *remoteRunner) runQuiet(ctx context.Context, command string) (string, error) {
	return r.runWithTrace(ctx, command, false)
}

func (r *remoteRunner) runWithTrace(ctx context.Context, command string, trace bool) (string, error) {
	if trace {
		r.logger.Debug("ssh command", "command", command)
	}
	commandCtx := ctx
	cancel := func() {}
	if timeout := remoteCommandTimeout(command); timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	if r.runOverride != nil {
		output, err := r.runOverride(commandCtx, command, trace)
		if trace && strings.TrimSpace(output) != "" {
			r.logger.Debug("ssh output", "command", command, "output", output)
		}
		if trace && err != nil {
			r.logSSHCommandFailure(command, err)
		}
		return output, normalizeRemoteCommandError(command, err)
	}

	session, err := r.client.NewSession()
	if err != nil {
		r.logger.Error("failed to create SSH session", "error", err)
		return "", normalizeRemoteCommandError(command, err)
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
	case <-commandCtx.Done():
		_ = session.Close()
		if trace {
			r.logger.Error("ssh command cancelled", "command", command, "error", commandCtx.Err())
		}
		return "", normalizeRemoteCommandError(command, commandCtx.Err())
	case res := <-ch:
		if trace && strings.TrimSpace(string(res.data)) != "" {
			r.logger.Debug("ssh output", "command", command, "output", string(res.data))
		}
		if trace && res.err != nil {
			r.logSSHCommandFailure(command, res.err)
		}
		return string(res.data), normalizeRemoteCommandError(command, res.err)
	}
}

type streamedSSHEvent struct {
	line string
	err  error
}

func readSSHStreamLines(reader io.Reader, events chan<- streamedSSHEvent) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		events <- streamedSSHEvent{line: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		events <- streamedSSHEvent{err: err}
	}
}

func (r *remoteRunner) runStreamingWithTrace(ctx context.Context, command string, trace bool, onLine func(string)) (string, error) {
	if trace {
		r.logger.Debug("ssh command", "command", command)
	}
	commandCtx := ctx
	cancel := func() {}
	if timeout := remoteCommandTimeout(command); timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	session, err := r.client.NewSession()
	if err != nil {
		r.logger.Error("failed to create SSH session", "error", err)
		return "", normalizeRemoteCommandError(command, err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", normalizeRemoteCommandError(command, err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", normalizeRemoteCommandError(command, err)
	}
	if err := session.Start(command); err != nil {
		if trace {
			r.logger.Error("ssh command failed to start", "command", command, "error", err)
		}
		return "", normalizeRemoteCommandError(command, err)
	}

	events := make(chan streamedSSHEvent, 64)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		readSSHStreamLines(stdout, events)
	}()
	go func() {
		defer wg.Done()
		readSSHStreamLines(stderr, events)
	}()
	go func() {
		wg.Wait()
		close(events)
	}()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- session.Wait()
	}()

	var output strings.Builder
	var waitErr error
	var streamErr error
	eventCh := events
	waitChan := waitCh
	for eventCh != nil || waitChan != nil {
		select {
		case <-commandCtx.Done():
			_ = session.Close()
			if trace {
				r.logger.Error("ssh command cancelled", "command", command, "error", commandCtx.Err())
			}
			return output.String(), normalizeRemoteCommandError(command, commandCtx.Err())
		case event, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			if event.err != nil {
				if streamErr == nil {
					streamErr = event.err
				}
				continue
			}
			line := strings.TrimRight(event.line, "\r")
			output.WriteString(line)
			output.WriteString("\n")
			if onLine != nil {
				onLine(line)
			}
			if trace && strings.TrimSpace(line) != "" {
				r.logger.Debug("ssh output", "command", command, "output", line)
			}
		case err := <-waitChan:
			waitErr = err
			waitChan = nil
		}
	}
	if waitErr == nil && streamErr != nil {
		waitErr = streamErr
	}
	if trace && waitErr != nil {
		r.logger.Error("ssh command failed", "command", command, "error", waitErr)
	}
	return output.String(), normalizeRemoteCommandError(command, waitErr)
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
		if !r.cfg.AutoYes {
			answer, askErr := askYesNo("Backup failed. Continue anyway?", true)
			if askErr != nil {
				return "", askErr
			}
			if !answer {
				return "", fmt.Errorf("%sbackup failed and execution was cancelled%s", colorRed, colorReset)
			}
		} else {
			r.warnLine("Backup failed. Continuing because -y, --yes was used.")
		}
		r.warnLine("Continuing without a successful backup.")
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
	output, err := r.runQuiet(ctx, "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")
	if err != nil {
		return nil, fmt.Errorf("%sfeature list request failed: %w; output: %s%s", colorRed, err, strings.TrimSpace(output), colorReset)
	}
	return parseFeatureRecords(output), nil
}

func (r *remoteRunner) featureRecordsDuringInstall(ctx context.Context) ([]featureRecord, bool, error) {
	output, err := r.runQuiet(ctx, "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")
	if err != nil {
		if isTransientFeatureListFailure(output) {
			r.logger.Warn("feature list request was temporarily rejected during package installation", "output", strings.TrimSpace(output))
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("%sfeature list request failed: %w; output: %s%s", colorRed, err, strings.TrimSpace(output), colorReset)
	}
	return parseFeatureRecords(output), false, nil
}

func (r *remoteRunner) runPackageStep(ctx context.Context, step packageSyncStep, targetOS string) error {
	return r.runPackageStepNoUpdate(ctx, step, targetOS)
}

func (r *remoteRunner) runPackageStepNoUpdate(ctx context.Context, step packageSyncStep, targetOS string) error {
	action := describePackageStep(step.Title)
	progressID := randomProgressID()
	step, err := r.pruneFeatureStep(ctx, step)
	if err != nil {
		return err
	}
	cursor := r.newRemoteLogCursor(ctx)
	command := step.Command
	progressTracked := isDirectFeatureEditCommand(step.Command)
	if progressTracked {
		command = addProgressID(step.Command, progressID)
	}
	return r.runAction(action, func() error {
		r.printLaunchedCommand(command)
		output, runErr := r.run(ctx, command)
		if strings.TrimSpace(output) != "" && consoleLevelEnabled(r.cfg.LogLevel, "info") {
			r.ui.Println(strings.TrimSpace(output))
		}
		if runErr != nil {
			return fmt.Errorf("%spackage step failed (%s): %w%s", colorRed, step.Title, runErr, colorReset)
		}
		if progressTracked {
			if err := r.checkLogProgress(ctx, progressID, cursor.ispmgrOffset); err != nil {
				return err
			}
		}
		if err := r.checkRecentLogErrors(ctx, cursor.ispmgrOffset, cursor.pkgOffset); err != nil {
			return err
		}
		return r.waitFeatureStep(ctx, step, targetOS, progressID, &cursor)
	})
}

func (r *remoteRunner) waitFeatureStep(ctx context.Context, step packageSyncStep, targetOS string, progressID string, cursor *remoteLogCursor) error {
	deadline := time.Now().Add(20 * time.Minute)
	nextFeaturePoll := time.Time{}
	nextLogCheck := time.Time{}
	lastInstalled := map[string]struct{}{}
	altPHPFeatures := altPHPFeatureIDsFromStepCommand(step.Command)
	r.printMonitoringCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")
	for time.Now().Before(deadline) {
		now := time.Now()
		if cursor != nil && (nextLogCheck.IsZero() || !now.Before(nextLogCheck)) {
			if err := r.scanRecentLogs(ctx, cursor); err != nil {
				return err
			}
			nextLogCheck = now.Add(5 * time.Second)
		}
		if nextFeaturePoll.IsZero() || !now.Before(nextFeaturePoll) {
			records, transient, err := r.featureRecordsDuringInstall(ctx)
			if err != nil {
				return err
			}
			if transient {
				nextFeaturePoll = now.Add(20 * time.Second)
				time.Sleep(1 * time.Second)
				continue
			}
			if bad, ok := featureStepBadState(records, step.Feature, altPHPFeatures); ok {
				return fmt.Errorf("%sfeature step %s entered badstate=%s for %s; check /usr/local/mgr5/var/pkg.log%s", colorRed, step.Title, bad.BadState, bad.Name, colorReset)
			}
			if len(altPHPFeatures) > 0 {
				if altPHPFeaturesBusy(records, altPHPFeatures) {
					nextFeaturePoll = now.Add(15 * time.Second)
					time.Sleep(1 * time.Second)
					continue
				}
			} else if strings.TrimSpace(step.Feature) != "" {
				record := findFeatureRecord(records, step.Feature)
				if strings.EqualFold(record.Status, "install") {
					nextFeaturePoll = now.Add(15 * time.Second)
					time.Sleep(1 * time.Second)
					continue
				}
			} else if featuresBusy(records) {
				nextFeaturePoll = now.Add(15 * time.Second)
				time.Sleep(1 * time.Second)
				continue
			}
			if len(step.ExpectedPackages) == 0 {
				return nil
			}
			lastInstalled = installedPackagesFromFeatures(records, targetOS)
			if packageSubsetPresent(lastInstalled, step.ExpectedPackages) {
				return nil
			}
			r.logger.Debug("package step still waiting for expected packages", "step", step.Title, "missing", missingExpectedPackages(lastInstalled, step.ExpectedPackages))
			nextFeaturePoll = now.Add(15 * time.Second)
		}
		time.Sleep(1 * time.Second)
	}
	missing := missingExpectedPackages(lastInstalled, step.ExpectedPackages)
	if len(missing) > 0 {
		return fmt.Errorf("%sfeature step %s did not finish in time; missing packages: %s%s", colorRed, step.Title, strings.Join(missing, ", "), colorReset)
	}
	return fmt.Errorf("%sfeature step %s did not finish in time%s", colorRed, step.Title, colorReset)
}

func altPHPFeatureIDsFromStepCommand(command string) map[string]struct{} {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "feature.resume" {
		return nil
	}

	raw := strings.TrimSpace(params["elid"])
	if raw == "" {
		return nil
	}

	result := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(name, "altphp") {
			result[name] = struct{}{}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func altPHPFeaturesBusy(records []featureRecord, wanted map[string]struct{}) bool {
	for _, record := range records {
		name := strings.ToLower(strings.TrimSpace(record.Name))
		if _, ok := wanted[name]; !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(record.Status), "install") {
			return true
		}
	}
	return false
}

func (r *remoteRunner) waitForFeaturesIdle(ctx context.Context) error {
	deadline := time.Now().Add(20 * time.Minute)
	nextFeaturePoll := time.Time{}
	nextLogCheck := time.Time{}
	cursor := r.newRemoteLogCursor(ctx)
	idlePolls := 0
	r.printMonitoringCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")
	for time.Now().Before(deadline) {
		now := time.Now()
		if !now.Before(nextLogCheck) {
			if err := r.scanRecentLogs(ctx, &cursor); err != nil {
				return err
			}
			nextLogCheck = now.Add(10 * time.Second)
		}
		if nextFeaturePoll.IsZero() || !now.Before(nextFeaturePoll) {
			records, transient, err := r.featureRecordsDuringInstall(ctx)
			if err != nil {
				return err
			}
			if transient {
				idlePolls = 0
				nextFeaturePoll = now.Add(20 * time.Second)
				time.Sleep(1 * time.Second)
				continue
			}
			if bad, ok := firstBadFeatureState(records); ok {
				return fmt.Errorf("%sdestination panel feature %s entered badstate=%s; check /usr/local/mgr5/var/pkg.log%s", colorRed, bad.Name, bad.BadState, colorReset)
			}
			idlePolls = nextIdlePollCount(records, idlePolls)
			if idlePolls >= requiredStableIdlePolls {
				return nil
			}
			if idlePolls == 0 {
				nextFeaturePoll = now.Add(15 * time.Second)
			} else {
				nextFeaturePoll = now.Add(10 * time.Second)
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%sdestination panel still has unfinished feature installation tasks%s", colorRed, colorReset)
}

const requiredStableIdlePolls = 2

func (r *remoteRunner) waitForPanelReady(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		installed, checkErr := r.panelInstalled(ctx)
		if checkErr == nil && installed {
			if _, infoErr := r.licenseInfo(ctx); infoErr == nil {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("%sispmanager installation did not finish in time%s", colorRed, colorReset)
}

func nextIdlePollCount(records []featureRecord, current int) int {
	if featuresBusy(records) {
		return 0
	}
	return current + 1
}

func featuresBusy(records []featureRecord) bool {
	for _, record := range records {
		if strings.EqualFold(record.Status, "install") {
			return true
		}
	}
	return false
}

type featureBadState struct {
	Name     string
	BadState string
}

func featureStepBadState(records []featureRecord, feature string, wantedAlt map[string]struct{}) (featureBadState, bool) {
	if len(wantedAlt) > 0 {
		for _, record := range records {
			name := strings.ToLower(strings.TrimSpace(record.Name))
			if _, ok := wantedAlt[name]; !ok {
				continue
			}
			if strings.TrimSpace(record.BadState) != "" {
				return featureBadState{
					Name:     firstNonEmpty(strings.TrimSpace(record.Name), "altphp"),
					BadState: strings.TrimSpace(record.BadState),
				}, true
			}
		}
		return featureBadState{}, false
	}

	feature = strings.TrimSpace(strings.ToLower(feature))
	if feature == "" {
		return featureBadState{}, false
	}
	for _, record := range records {
		if strings.TrimSpace(strings.ToLower(record.Name)) != feature {
			continue
		}
		if strings.TrimSpace(record.BadState) != "" {
			return featureBadState{
				Name:     firstNonEmpty(strings.TrimSpace(record.Name), feature),
				BadState: strings.TrimSpace(record.BadState),
			}, true
		}
	}
	return featureBadState{}, false
}

func firstBadFeatureState(records []featureRecord) (featureBadState, bool) {
	for _, record := range records {
		if strings.TrimSpace(record.BadState) == "" {
			continue
		}
		return featureBadState{
			Name:     firstNonEmpty(strings.TrimSpace(record.Name), "<unknown>"),
			BadState: strings.TrimSpace(record.BadState),
		}, true
	}
	return featureBadState{}, false
}

func missingExpectedPackages(values map[string]struct{}, expected []string) []string {
	missing := make([]string, 0, len(expected))
	for _, item := range expected {
		if !hasEquivalentPackage(values, item) {
			missing = append(missing, item)
		}
	}
	return missing
}

func (r *remoteRunner) pruneFeatureStep(ctx context.Context, step packageSyncStep) (packageSyncStep, error) {
	command := step.Command
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "feature.edit" {
		return step, nil
	}
	elid := strings.TrimSpace(params["elid"])
	if elid == "" {
		return step, nil
	}
	output, err := r.run(ctx, buildMgrctlCommand("feature.edit", map[string]string{
		"elid": elid,
		"out":  "text",
	}))
	if err != nil {
		return step, nil
	}
	form := parseFeatureForm(output)
	if len(form) == 0 {
		return step, nil
	}
	pruned, retainedParams, _ := pruneMgrctlParams(params, form)
	step.Command = buildMgrctlCommand(function, pruned)
	step.ExpectedPackages = pruneExpectedPackages(step.Feature, step.ExpectedPackages, retainedParams)
	return step, nil
}

func (r *remoteRunner) pruneEntityCommand(ctx context.Context, command string) (string, []string, error) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok {
		return command, nil, nil
	}
	if function == "emaildomain.edit" {
		delete(params, "ipsrc")
		delete(params, "ip")
		command = buildMgrctlCommand(function, params)
	}
	if !supportsFormPruning(function) {
		return command, nil, nil
	}

	formParams := map[string]string{"out": "text"}
	if elid := strings.TrimSpace(params["elid"]); elid != "" {
		formParams["elid"] = elid
	}
	output, err := r.run(ctx, buildMgrctlCommand(function, formParams))
	if err != nil {
		return command, nil, nil
	}
	form := parseFeatureForm(output)
	if len(form) == 0 {
		return command, nil, nil
	}

	pruned, _, dropped := pruneMgrctlParams(params, form)
	if len(dropped) == 0 {
		return command, nil, nil
	}

	notes := make([]string, 0, len(dropped))
	for _, key := range dropped {
		notes = append(notes, fmt.Sprintf("%s does not support %s, removed before execution", function, key))
	}
	return buildMgrctlCommand(function, pruned), notes, nil
}

func pruneMgrctlParams(params map[string]string, form map[string]string) (map[string]string, map[string]struct{}, []string) {
	pruned := map[string]string{}
	retainedParams := map[string]struct{}{}
	dropped := make([]string, 0)
	for key, value := range params {
		if retainMgrctlParam(key) {
			pruned[key] = value
			retainedParams[key] = struct{}{}
			continue
		}
		if _, ok := form[key]; ok {
			pruned[key] = value
			retainedParams[key] = struct{}{}
			continue
		}
		if shouldRetainImplicitFeatureParam(key, value, params, form) {
			pruned[key] = value
			retainedParams[key] = struct{}{}
			continue
		}
		dropped = append(dropped, key)
	}
	return pruned, retainedParams, sortedStrings(dropped)
}

func shouldRetainImplicitFeatureParam(key string, value string, params map[string]string, form map[string]string) bool {
	switch key {
	case "package_openlitespeed-php":
		return strings.EqualFold(strings.TrimSpace(value), "on") &&
			strings.EqualFold(strings.TrimSpace(params["package_openlitespeed"]), "on") &&
			hasFormKeyCI(form, "package_openlitespeed")
	default:
		return false
	}
}

func hasFormKeyCI(values map[string]string, key string) bool {
	_, ok := values[key]
	return ok
}

func retainMgrctlParam(key string) bool {
	switch key {
	case "elid", "out", "sok", "progressid":
		return true
	default:
		return false
	}
}

func supportsFormPruning(function string) bool {
	switch function {
	case "user.edit", "ftp.user.edit", "site.edit", "emaildomain.edit", "email.edit", "domain.edit", "db.edit", "db.server.edit":
		return true
	default:
		return false
	}
}

func pruneExpectedPackages(feature string, expected []string, retainedParams map[string]struct{}) []string {
	if len(expected) == 0 {
		return nil
	}
	pruned := make([]string, 0, len(expected))
	for _, item := range expected {
		param := expectedPackageParamName(feature, item)
		if param == "" {
			pruned = append(pruned, item)
			continue
		}
		if _, ok := retainedParams[param]; ok {
			pruned = append(pruned, item)
		}
	}
	return pruned
}

func expectedPackageParamName(feature string, packageName string) string {
	packageName = strings.TrimSpace(strings.ToLower(packageName))
	feature = strings.TrimSpace(strings.ToLower(feature))
	switch packageName {
	case "apache-itk", "apache-itk-ubuntu", "apache-prefork":
		return "packagegroup_apache"
	case "bind", "powerdns":
		return "packagegroup_dns"
	case "pureftp", "proftp":
		return "packagegroup_ftp"
	case "mysql-server", "mariadb-server":
		return "packagegroup_mysql"
	case "exim":
		return "packagegroup_mta"
	case "composer":
		return "package_phpcomposer"
	case "postgresql":
		return "package_postgresql"
	case "phpmyadmin":
		return "package_phpmyadmin"
	case "phppgadmin":
		return "package_phppgadmin"
	case "openlitespeed-php":
		return "package_openlitespeed-php"
	}
	if strings.HasPrefix(feature, "altphp") {
		version := strings.TrimPrefix(feature, "altphp")
		baseName := "ispphp" + version
		switch packageName {
		case baseName:
			return "packagegroup_altphp" + version + "gr"
		case baseName + "_lsapi", baseName + "_fpm", baseName + "_mod_apache":
			return "package_" + packageName
		}
	}
	if strings.HasPrefix(packageName, "isppython") {
		return "package_" + packageName
	}
	return "package_" + packageName
}

func hasEquivalentPackage(values map[string]struct{}, name string) bool {
	if hasPackage(values, name) {
		return true
	}
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "apache-itk":
		return hasPackage(values, "apache-itk-ubuntu")
	case "apache-itk-ubuntu":
		return hasPackage(values, "apache-itk")
	default:
		return false
	}
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

func (r *remoteRunner) prepareLiteDockerForAltDB(ctx context.Context, command string) error {
	if r.liteDockerPrepared {
		return nil
	}
	if !shouldRunDockerInstallBeforeAltDB(r.panelName, command) {
		return nil
	}

	return r.runAction("starting docker support for alternative database servers on remote side", func() error {
		progressID := randomProgressID()
		cursor := r.newRemoteLogCursor(ctx)
		dockerInstallCommand := addProgressID(buildMgrctlCommand("docker.install", map[string]string{
			"sok": "ok",
		}), progressID)
		r.printLaunchedCommand(dockerInstallCommand)
		output, err := r.run(ctx, dockerInstallCommand)
		if strings.TrimSpace(output) != "" && consoleLevelEnabled(r.cfg.LogLevel, "info") {
			r.ui.Println(strings.TrimSpace(output))
		}
		if err != nil {
			return fmt.Errorf("%sdocker.install failed: %w%s", colorRed, err, colorReset)
		}
		if err := r.checkLogProgress(ctx, progressID, cursor.ispmgrOffset); err != nil {
			return err
		}
		if err := r.checkRecentLogErrors(ctx, cursor.ispmgrOffset, cursor.pkgOffset); err != nil {
			return err
		}
		r.liteDockerPrepared = true
		return nil
	})
}

func retryWithoutInvalidUserParam(ctx context.Context, r *remoteRunner, command string, progressID string, output string) (string, error, bool) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok {
		return output, nil, false
	}
	invalidParam := invalidParamName(output)
	if invalidParam == "" || !safeToDropInvalidParam(function, invalidParam) {
		return output, nil, false
	}
	r.logger.Warn("remote API rejected parameter, retrying without it", "command", command, "parameter", invalidParam, "output", output)
	delete(params, invalidParam)
	delete(params, "progressid")
	retryCommand := addProgressID(buildMgrctlCommand(function, params), progressID)
	retryOutput, retryErr := r.run(ctx, retryCommand)
	return retryOutput, retryErr, true
}

func invalidParamName(output string) string {
	line := strings.TrimSpace(output)
	if !strings.HasPrefix(line, "ERROR value(") {
		return ""
	}
	rest := strings.TrimPrefix(line, "ERROR value(")
	name, _, ok := strings.Cut(rest, ")")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}

func safeToDropInvalidParam(function string, name string) bool {
	switch function {
	case "user.edit":
		return name == "limit_php_cgi_version"
	case "site.edit":
		return name == "site_analyzer"
	default:
		return false
	}
}

func retryDBServerAfterDockerInstall(ctx context.Context, r *remoteRunner, command string, progressID string, output string) (string, error, bool, error) {
	function, _, ok := parseMgrctlCommand(command)
	if !ok || function != "db.server.edit" {
		return output, nil, false, nil
	}
	if !needsDockerInstallForDBServerOutput(output) {
		return output, nil, false, nil
	}
	r.logger.Warn("remote API reported nodocker for db.server.edit, enabling docker and retrying", "command", command, "output", output)
	if err := r.ensureDockerInstalled(ctx); err != nil {
		return output, err, true, nil
	}
	retryCommand := addProgressID(command, progressID)
	retryOutput, retryErr := r.run(ctx, retryCommand)
	return retryOutput, retryErr, true, nil
}

func needsDockerInstallForDBServerOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "notconfigured(nodocker)")
}

func shouldRunDockerInstallBeforeAltDB(panelName string, command string) bool {
	if !isLitePanel(panelName) || !isAlternativeMySQLDBServerCommand(command) {
		return false
	}
	return true
}

func isLitePanel(panelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(panelName)), "lite")
}

func isAlternativeMySQLDBServerCommand(command string) bool {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "db.server.edit" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(params["type"]), "mysql") {
		return false
	}
	return isAlternativeMySQLHost(params["host"])
}

func isAlternativeMySQLHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	_, portText, ok := strings.Cut(host, ":")
	if !ok {
		return false
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		return false
	}
	return port >= 3300 && port <= 3399 && port != 3306
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
		if function == "feature.edit" || function == "feature.resume" {
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
	localBytes, err := localMemoryTotalBytesHook()
	if err != nil {
		r.logger.Debug("local memory check skipped", "error", err)
		return nil
	}
	remoteBytes, err := remoteMemoryTotalBytesHook(r, ctx)
	if err != nil {
		r.logger.Debug("remote memory check skipped", "error", err)
		return nil
	}
	const gib = int64(1024 * 1024 * 1024)
	if remoteBytes < 2*gib {
		swapState, err := remoteSwapStateHook(r, ctx)
		if err != nil {
			return fmt.Errorf("%sfailed to inspect swap state on destination server: %w%s", colorRed, err, colorReset)
		}
		if swapState.HasOtherNonZRAMSwap || swapState.SwapfileActive {
			r.warnLine(fmt.Sprintf("destination memory is low: %s (swap already enabled)", humanGiB(remoteBytes)))
		} else {
			r.warnLine(fmt.Sprintf("destination memory is low: %s", humanGiB(remoteBytes)))
		}
		if !swapState.HasOtherNonZRAMSwap && (!swapState.SwapfileActive || !swapState.FstabHasSwapfile) {
			if err := r.runAction("creating swapfile and enabling it", func() error {
				return ensureSwapfileHook(r, ctx, swapState)
			}); err != nil {
				return err
			}
		}
		return nil
	}
	if localBytes-remoteBytes > 2*gib {
		r.warnLine(fmt.Sprintf("destination memory (%s) is more than 2 GiB lower than source memory (%s).", humanGiB(remoteBytes), humanGiB(localBytes)))
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

func (r *remoteRunner) remoteSwapState(ctx context.Context) (remoteSwapState, error) {
	swapsOutput, err := r.runQuiet(ctx, "cat /proc/swaps")
	if err != nil {
		return remoteSwapState{}, err
	}
	fstabOutput, err := r.runQuiet(ctx, "cat /etc/fstab")
	if err != nil {
		return remoteSwapState{}, err
	}
	swapfileExists, err := r.remoteFileExists("/swapfile")
	if err != nil {
		return remoteSwapState{}, err
	}
	return parseRemoteSwapState(swapsOutput, fstabOutput, swapfileExists), nil
}

func parseRemoteSwapState(swapsOutput string, fstabOutput string, swapfileExists bool) remoteSwapState {
	state := remoteSwapState{SwapfileExists: swapfileExists}

	for idx, line := range strings.Split(swapsOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx == 0 && strings.HasPrefix(line, "Filename") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if isZRAMSwapName(name) {
			continue
		}
		if name == "/swapfile" {
			state.SwapfileActive = true
			continue
		}
		state.HasOtherNonZRAMSwap = true
	}

	for _, line := range strings.Split(fstabOutput, "\n") {
		line = strings.TrimSpace(line)
		if hasSwapfileFstabEntry(line) {
			state.FstabHasSwapfile = true
			break
		}
	}

	return state
}

func isZRAMSwapName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "/dev/zram") || strings.HasPrefix(name, "zram")
}

func hasSwapfileFstabEntry(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return false
	}
	return fields[0] == "/swapfile" && fields[2] == "swap"
}

func (r *remoteRunner) ensureSwapfile(ctx context.Context, state remoteSwapState) error {
	if state.HasOtherNonZRAMSwap {
		return nil
	}

	if !state.SwapfileExists {
		r.infoLine("creating remote file: /swapfile (2 GiB)")
		if err := r.createRemoteSwapfile(swapfilePath, swapfileSizeBytes); err != nil {
			return err
		}
		r.successLine("creating remote file: /swapfile (2 GiB): OK")
	}
	if err := r.sftpClient.Chmod(swapfilePath, 0600); err != nil {
		return fmt.Errorf("%sfailed to set permissions on %s: %w%s", colorRed, swapfilePath, err, colorReset)
	}

	if !state.FstabHasSwapfile {
		r.infoLine("updating /etc/fstab for /swapfile")
		if err := r.ensureSwapfileFSTabEntry(); err != nil {
			return err
		}
		r.successLine("updating /etc/fstab for /swapfile: OK")
	}

	if !state.SwapfileActive {
		for _, command := range []string{"mkswap " + swapfilePath, "swapon " + swapfilePath} {
			r.printLaunchedCommand(command)
			output, err := r.run(ctx, command)
			if strings.TrimSpace(output) != "" && consoleLevelEnabled(r.cfg.LogLevel, "info") {
				r.ui.Println(strings.TrimSpace(output))
			}
			if err != nil {
				return fmt.Errorf("%sfailed to enable swapfile on destination server: %w%s", colorRed, err, colorReset)
			}
		}
	}

	return nil
}

func (r *remoteRunner) createRemoteSwapfile(path string, size int64) error {
	file, err := r.sftpClient.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return fmt.Errorf("%sfailed to create %s on destination server: %w%s", colorRed, path, err, colorReset)
	}
	defer file.Close()

	const chunkSize = 8 * 1024 * 1024
	buffer := make([]byte, chunkSize)
	remaining := size
	writtenTotal := int64(0)
	nextProgressMark := int64(512 * 1024 * 1024)
	for remaining > 0 {
		writeSize := len(buffer)
		if remaining < int64(writeSize) {
			writeSize = int(remaining)
		}
		written, err := file.Write(buffer[:writeSize])
		if err != nil {
			return fmt.Errorf("%sfailed while writing %s on destination server: %w%s", colorRed, path, err, colorReset)
		}
		if written != writeSize {
			return fmt.Errorf("%sfailed while writing %s on destination server: short write%s", colorRed, path, colorReset)
		}
		remaining -= int64(written)
		writtenTotal += int64(written)
		if writtenTotal >= nextProgressMark || remaining == 0 {
			if consoleLevelEnabled(r.cfg.LogLevel, "info") {
				r.infoLine(fmt.Sprintf("swapfile write progress: %d MiB / %d MiB", writtenTotal/(1024*1024), size/(1024*1024)))
			}
			nextProgressMark += 512 * 1024 * 1024
		}
	}
	return nil
}

func (r *remoteRunner) ensureSwapfileFSTabEntry() error {
	content, mode, err := r.readRemoteTextFile("/etc/fstab")
	if err != nil {
		return err
	}

	updated, changed := appendSwapfileFstabLine(content)
	if !changed {
		return nil
	}
	return r.writeRemoteTextFile("/etc/fstab", updated, mode)
}

func appendSwapfileFstabLine(content string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		if hasSwapfileFstabEntry(line) {
			return content, false
		}
	}

	trimmedRight := strings.TrimRight(content, "\n")
	if trimmedRight == "" {
		return swapfileFstabLine + "\n", true
	}
	return trimmedRight + "\n" + swapfileFstabLine + "\n", true
}

func (r *remoteRunner) readRemoteTextFile(path string) (string, fs.FileMode, error) {
	file, err := r.sftpClient.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("%sfailed to open %s on destination server: %w%s", colorRed, path, err, colorReset)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("%sfailed to stat %s on destination server: %w%s", colorRed, path, err, colorReset)
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return "", 0, fmt.Errorf("%sfailed to read %s on destination server: %w%s", colorRed, path, err, colorReset)
	}
	return string(content), info.Mode(), nil
}

func (r *remoteRunner) writeRemoteTextFile(path string, content string, mode fs.FileMode) error {
	file, err := r.sftpClient.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return fmt.Errorf("%sfailed to open %s for writing on destination server: %w%s", colorRed, path, err, colorReset)
	}
	defer file.Close()

	if _, err := io.WriteString(file, content); err != nil {
		return fmt.Errorf("%sfailed to write %s on destination server: %w%s", colorRed, path, err, colorReset)
	}
	if mode != 0 {
		if err := r.sftpClient.Chmod(path, mode); err != nil {
			return fmt.Errorf("%sfailed to set permissions on %s: %w%s", colorRed, path, err, colorReset)
		}
	}
	return nil
}

func (r *remoteRunner) remoteFileSize(path string) (int64, error) {
	info, err := r.sftpClient.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

type remoteLogCursor struct {
	ispmgrOffset int64
	pkgOffset    int64
}

func (r *remoteRunner) newRemoteLogCursor(ctx context.Context) remoteLogCursor {
	cursor := remoteLogCursor{}
	cursor.ispmgrOffset, _ = r.remoteFileSize("/usr/local/mgr5/var/ispmgr.log")
	cursor.pkgOffset, _ = r.remoteFileSize("/usr/local/mgr5/var/pkg.log")
	return cursor
}

func (r *remoteRunner) scanRecentLogs(ctx context.Context, cursor *remoteLogCursor) error {
	if cursor == nil {
		return nil
	}

	ispmgrNext, err := r.forEachLogLineSince(ctx, "/usr/local/mgr5/var/ispmgr.log", cursor.ispmgrOffset, func(line string) error {
		return r.inspectLogLine("/usr/local/mgr5/var/ispmgr.log", line)
	})
	if err == nil {
		cursor.ispmgrOffset = ispmgrNext
	}

	pkgNext, err := r.forEachLogLineSince(ctx, "/usr/local/mgr5/var/pkg.log", cursor.pkgOffset, func(line string) error {
		return r.inspectLogLine("/usr/local/mgr5/var/pkg.log", line)
	})
	if err == nil {
		cursor.pkgOffset = pkgNext
	}

	return nil
}

func (r *remoteRunner) forEachLogLineSince(ctx context.Context, path string, offset int64, fn func(string) error) (int64, error) {
	info, err := r.sftpClient.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return offset, err
	}
	size := info.Size()
	if size < offset {
		offset = 0
	}
	if size == offset {
		return size, nil
	}
	file, err := r.sftpClient.Open(path)
	if err != nil {
		return offset, err
	}
	defer file.Close()

	const maxLogChunkSize = 64 * 1024

	currentOffset := offset
	remainder := ""
	for currentOffset < size {
		if err := ctx.Err(); err != nil {
			return currentOffset, err
		}

		chunkSize := size - currentOffset
		if chunkSize > maxLogChunkSize {
			chunkSize = maxLogChunkSize
		}
		buffer := make([]byte, int(chunkSize))
		readBytes, readErr := file.ReadAt(buffer, currentOffset)
		if readErr != nil && readErr != io.EOF {
			return currentOffset, readErr
		}
		if readBytes == 0 {
			break
		}

		remainder, err = consumeLogTextLines(remainder, string(buffer[:readBytes]), currentOffset+int64(readBytes) >= size, fn)
		if err != nil {
			return currentOffset + int64(readBytes), err
		}
		currentOffset += int64(readBytes)
	}
	return size, nil
}

func consumeLogTextLines(remainder string, chunk string, flush bool, fn func(string) error) (string, error) {
	text := remainder + chunk
	parts := strings.Split(text, "\n")
	if !flush {
		remainder = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	} else {
		remainder = ""
	}
	for _, line := range parts {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := fn(line); err != nil {
			return remainder, err
		}
	}
	if flush {
		line := strings.TrimSpace(remainder)
		if line != "" {
			if err := fn(line); err != nil {
				return "", err
			}
		}
		return "", nil
	}
	return remainder, nil
}

func (r *remoteRunner) inspectLogLine(path string, line string) error {
	r.logger.Debug("panel log line", "file", path, "line", line)
	if isLogErrorLine(line) {
		return fmt.Errorf("%srecent panel log error: %s%s", colorRed, line, colorReset)
	}
	return nil
}

func (r *remoteRunner) checkRecentLogErrors(ctx context.Context, ispmgrOffset int64, pkgOffset int64) error {
	if _, err := r.forEachLogLineSince(ctx, "/usr/local/mgr5/var/ispmgr.log", ispmgrOffset, func(line string) error {
		if isLogErrorLine(line) {
			return fmt.Errorf("%srecent panel log error: %s%s", colorRed, line, colorReset)
		}
		return nil
	}); err != nil {
		return err
	}

	if _, err := r.forEachLogLineSince(ctx, "/usr/local/mgr5/var/pkg.log", pkgOffset, func(line string) error {
		if isLogErrorLine(line) {
			return fmt.Errorf("%srecent panel log error: %s%s", colorRed, line, colorReset)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (r *remoteRunner) copyConfigs(ctx context.Context) error {
	r.warnLine("--copy-configs currently copies only the main ispmanager configuration files.")
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

func (r *remoteRunner) checkLogProgress(ctx context.Context, progressID string, offset int64) error {
	const (
		progressDiscoveryTimeout  = 30 * time.Second
		progressCompletionTimeout = 20 * time.Minute
		progressPollInterval      = 2 * time.Second
		requiredIdlePolls         = 2
	)
	discoveryDeadline := time.Now().Add(progressDiscoveryTimeout)
	deadline := time.Now().Add(progressCompletionTimeout)
	state := progressLogState{
		progressID: progressID,
		pids:       map[int]struct{}{},
	}
	currentOffset := offset
	idlePolls := 0
	for time.Now().Before(deadline) {
		nextOffset, err := r.forEachLogLineSince(ctx, "/usr/local/mgr5/var/ispmgr.log", currentOffset, func(line string) error {
			r.logger.Debug("panel progress line", "progressid", progressID, "line", line)
			state.consumeLine(line)
			return nil
		})
		if err != nil {
			return err
		}
		if nextOffset == currentOffset {
			idlePolls++
		} else {
			idlePolls = 0
			currentOffset = nextOffset
		}
		if err := state.outcome(); err != nil {
			return err
		}
		if !state.sawProgress {
			if time.Now().After(discoveryDeadline) {
				return nil
			}
		} else if !state.hasRunningProcess() && (state.sawFinished || idlePolls >= requiredIdlePolls) {
			return nil
		}
		time.Sleep(progressPollInterval)
	}
	if err := state.outcome(); err != nil {
		return err
	}
	if state.sawProgress && !state.hasRunningProcess() {
		return nil
	}
	return fmt.Errorf("%spanel log progress check timed out for progressid %s%s", colorRed, progressID, colorReset)
}

type progressLogState struct {
	progressID   string
	threadID     string
	sawProgress  bool
	sawFinished  bool
	errorLine    string
	finishedLine string
	pids         map[int]struct{}
}

func (s *progressLogState) consumeLine(line string) {
	threadID := extractLogThreadID(line)
	if strings.Contains(line, s.progressID) {
		s.sawProgress = true
		if s.threadID == "" {
			s.threadID = threadID
		}
	}
	if s.threadID == "" || threadID != s.threadID {
		return
	}
	if isLogErrorLine(line) && s.errorLine == "" {
		s.errorLine = line
	}
	if pid, ok := parseLogRunPID(line); ok {
		s.pids[pid] = struct{}{}
	}
	if pid, status, ok := parseLogFinishedPID(line); ok {
		if _, exists := s.pids[pid]; exists {
			s.sawFinished = true
			s.finishedLine = line
			if status != 0 && s.errorLine == "" {
				s.errorLine = line
			}
			delete(s.pids, pid)
		}
	}
}

func (s *progressLogState) hasRunningProcess() bool {
	return len(s.pids) > 0
}

func (s *progressLogState) outcome() error {
	if s.errorLine != "" {
		return fmt.Errorf("%spanel log reported an error for progressid %s: %s%s", colorRed, s.progressID, s.errorLine, colorReset)
	}
	return nil
}

func extractLogThreadID(line string) string {
	start := strings.IndexByte(line, '[')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(line[start:], ']')
	if end < 0 {
		return ""
	}
	return line[start : start+end+1]
}

func parseLogRunPID(line string) (int, bool) {
	if !strings.Contains(line, " proc ") || !strings.Contains(line, " pid ") {
		return 0, false
	}
	index := strings.LastIndex(line, " pid ")
	if index < 0 {
		return 0, false
	}
	value := strings.TrimSpace(line[index+5:])
	pid, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return pid, true
}

func parseLogFinishedPID(line string) (int, int, bool) {
	const prefix = "Process "
	index := strings.Index(line, prefix)
	if index < 0 || !strings.Contains(line, " finished with status ") {
		return 0, 0, false
	}
	rest := line[index+len(prefix):]
	pidText, rest, ok := strings.Cut(rest, " finished with status ")
	if !ok {
		return 0, 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(pidText))
	if err != nil {
		return 0, 0, false
	}
	status, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0, 0, false
	}
	return pid, status, true
}

func (r *remoteRunner) prepareRemoteSiteCommand(ctx context.Context, command string) (string, error) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "site.edit" {
		return command, nil
	}

	siteName := strings.TrimSpace(params["site_name"])
	owner := strings.TrimSpace(params["site_owner"])
	siteCert := strings.TrimSpace(params["site_ssl_cert"])
	if siteName == "" {
		return command, nil
	}

	if !r.cfg.CopyConfigs {
		if siteCert == "" || strings.EqualFold(siteCert, "selfsigned") || strings.EqualFold(siteCert, siteName) {
			certName, err := r.ensureSelfSignedSiteCertificate(ctx, siteName, owner)
			if err != nil {
				return "", err
			}
			siteCert = certName
			params["site_ssl_cert"] = certName
		}
	}

	return buildMgrctlCommand(function, params), nil
}

func (r *remoteRunner) ensureSelfSignedSiteCertificate(ctx context.Context, domain string, owner string) (string, error) {
	if certName, err := r.findSSLCertName(ctx, domain, owner); err == nil && certName != "" {
		return certName, nil
	}
	action := "creating self-signed certificate " + domain
	var certName string
	err := r.runAction(action, func() error {
		command := buildSelfSignedCertCommand(domain, owner)
		progressID := randomProgressID()
		command = addProgressID(command, progressID)
		logCursor := r.newRemoteLogCursor(ctx)
		output, runErr := r.run(ctx, command)
		if strings.TrimSpace(output) != "" && consoleLevelEnabled(r.cfg.LogLevel, "info") {
			r.ui.Println(strings.TrimSpace(output))
		}
		if runErr != nil && isAlreadyExistsOutput(output) {
			return nil
		}
		if runErr != nil {
			return fmt.Errorf("%sfailed to create self-signed certificate for %s: %w%s", colorRed, domain, runErr, colorReset)
		}
		if err := r.checkLogProgress(ctx, progressID, logCursor.ispmgrOffset); err != nil {
			return err
		}
		if err := r.checkRecentLogErrors(ctx, logCursor.ispmgrOffset, logCursor.pkgOffset); err != nil {
			return err
		}
		foundName, err := r.findSSLCertName(ctx, domain, owner)
		if err != nil {
			return err
		}
		if foundName == "" {
			return fmt.Errorf("%sfailed to discover created self-signed certificate for %s%s", colorRed, domain, colorReset)
		}
		certName = foundName
		return nil
	})
	if err != nil {
		return "", err
	}
	return certName, nil
}

func (r *remoteRunner) findSSLCertName(ctx context.Context, domain string, owner string) (string, error) {
	output, err := r.run(ctx, "/usr/local/mgr5/sbin/mgrctl -m ispmgr sslcert")
	if err != nil {
		return "", err
	}
	domain = strings.TrimSpace(domain)
	owner = strings.TrimSpace(owner)
	var fallback string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		values := parseKeyValueLine(line, []string{"owner", "key", "name", "info", "type", "valid_after", "cert_type", "active", "webdomains"})
		if owner != "" && values["owner"] != "" && !strings.EqualFold(strings.TrimSpace(values["owner"]), owner) {
			continue
		}
		name := strings.TrimSpace(values["name"])
		info := strings.TrimSpace(values["info"])
		webdomains := strings.TrimSpace(values["webdomains"])
		if name == domain {
			return name, nil
		}
		if strings.Contains(info, domain) || strings.Contains(webdomains, domain) {
			if strings.EqualFold(strings.TrimSpace(values["active"]), "on") {
				return name, nil
			}
			if fallback == "" {
				fallback = name
			}
		}
	}
	return fallback, nil
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

func isTransientFeatureListFailure(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(lower, "error request_failed:") && strings.Contains(lower, "terminated by administrator")
}

func isLogErrorLine(line string) bool {
	if line == "" {
		return false
	}
	return strings.Contains(line, " ERROR ") ||
		strings.Contains(line, " CRIT ") ||
		strings.Contains(line, "Failed to install package")
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
	if r.cfg.Overwrite {
		return false, "", nil
	}

	if r.inventory != nil {
		switch function {
		case "user.edit":
			name := strings.ToLower(strings.TrimSpace(params["name"]))
			if name != "" {
				if _, ok := r.inventory.users[name]; ok {
					return true, fmt.Sprintf("user %s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["name"])), nil
				}
			}
		case "ftp.user.edit":
			name := strings.ToLower(strings.TrimSpace(params["name"]))
			if name != "" {
				if _, ok := r.inventory.ftpUsers[name]; ok {
					return true, fmt.Sprintf("FTP user %s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["name"])), nil
				}
			}
		case "site.edit":
			name := strings.ToLower(strings.TrimSpace(params["site_name"]))
			if name != "" {
				if _, ok := r.inventory.webSites[name]; ok {
					return true, fmt.Sprintf("web site %s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["site_name"])), nil
				}
			}
		case "db.edit":
			key := databaseInventoryKey(params["name"], params["server"])
			if key != "::" {
				if _, ok := r.inventory.databases[key]; ok {
					return true, fmt.Sprintf("database %s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["name"])), nil
				}
			}
		case "emaildomain.edit":
			name := strings.ToLower(strings.TrimSpace(params["name"]))
			if name != "" {
				if _, ok := r.inventory.emailDomains[name]; ok {
					return true, fmt.Sprintf("email domain %s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["name"])), nil
				}
			}
		case "email.edit":
			key := emailInventoryKey(params["name"], params["domainname"])
			if key != "::" {
				if _, ok := r.inventory.emailBoxes[key]; ok {
					return true, fmt.Sprintf("email box %s@%s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["name"]), strings.TrimSpace(params["domainname"])), nil
				}
			}
		case "domain.edit":
			name := strings.ToLower(strings.TrimSpace(params["name"]))
			if name != "" {
				if _, ok := r.inventory.dnsZones[name]; ok {
					return true, fmt.Sprintf("DNS zone %s already exists on remote side, skipped. Run again with --overwrite to modify it.", strings.TrimSpace(params["name"])), nil
				}
			}
		}
	}

	switch function {
	case "db.server.edit":
		name := strings.TrimSpace(params["name"])
		if name == "" {
			return false, "", nil
		}
		if strings.EqualFold(name, "MySQL") {
			return false, "", nil
		}
		output, err := r.run(ctx, buildMgrctlCommand("db.server.edit", map[string]string{
			"elid": name,
			"out":  "text",
		}))
		if err == nil && strings.Contains(output, "elid="+name) {
			return true, fmt.Sprintf("DB server %s already exists on remote side, skipped. Run again with --overwrite to modify it.", name), nil
		}
	}

	return false, "", nil
}

func (r *remoteRunner) prepareExistingMySQLPasswordSync(ctx context.Context, data SourceData, command string) (string, string, error) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || function != "db.server.edit" {
		return command, "", nil
	}
	name := strings.TrimSpace(params["name"])
	if !strings.EqualFold(name, "MySQL") {
		return command, "", nil
	}
	exists, err := r.remoteDBServerExists(ctx, name)
	if err != nil {
		return "", "", err
	}
	if !exists {
		return command, "", nil
	}
	for _, item := range data.DBServers {
		if !strings.EqualFold(strings.TrimSpace(item.Name), name) {
			continue
		}
		if strings.TrimSpace(item.Password) == "" {
			return command, "DB server MySQL already exists on remote side, source password was not available, skipped.", nil
		}
		host := firstNonEmpty(strings.TrimSpace(item.Host), strings.TrimSpace(params["host"]), "localhost")
		username := firstNonEmpty(strings.TrimSpace(item.Username), strings.TrimSpace(params["username"]), "root")
		adjusted := buildMgrctlCommand("db.server.edit", map[string]string{
			"elid":            name,
			"host":            host,
			"name":            name,
			"username":        username,
			"password":        item.Password,
			"change_password": "on",
			"sok":             "ok",
		})
		r.infoLine("DB server MySQL already exists on remote side, syncing password from source.")
		return adjusted, "", nil
	}
	return command, "DB server MySQL already exists on remote side, source password was not available, skipped.", nil
}

func (r *remoteRunner) remoteDBServerExists(ctx context.Context, name string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return false, nil
	}
	if r.inventory != nil {
		if _, ok := r.inventory.dbServers[normalized]; ok {
			return true, nil
		}
	}
	output, err := r.run(ctx, buildMgrctlCommand("db.server.edit", map[string]string{
		"elid": name,
		"out":  "text",
	}))
	if err != nil {
		return false, nil
	}
	return strings.Contains(output, "elid="+name), nil
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

func isDirectFeatureEditCommand(command string) bool {
	function, _, ok := parseMgrctlCommand(command)
	return ok && function == "feature.edit"
}

func diffMgrctlCommands(sourceCommand string, remoteCommand string) string {
	sourceFunction, sourceParams, sourceOK := parseMgrctlCommand(sourceCommand)
	remoteFunction, remoteParams, remoteOK := parseMgrctlCommand(remoteCommand)
	if !sourceOK || !remoteOK {
		if strings.TrimSpace(sourceCommand) == strings.TrimSpace(remoteCommand) {
			return ""
		}
		return "raw command text differs"
	}
	if sourceFunction != remoteFunction {
		return fmt.Sprintf("function %s -> %s", sourceFunction, remoteFunction)
	}

	changes := make([]string, 0)
	seen := map[string]struct{}{}
	for key, sourceValue := range sourceParams {
		seen[key] = struct{}{}
		remoteValue, ok := remoteParams[key]
		if !ok {
			changes = append(changes, fmt.Sprintf("%s removed", key))
			continue
		}
		if sourceValue != remoteValue {
			changes = append(changes, fmt.Sprintf("%s=%s -> %s", key, sourceValue, remoteValue))
		}
	}
	for key, remoteValue := range remoteParams {
		if _, ok := seen[key]; ok {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s added=%s", key, remoteValue))
	}
	sort.Strings(changes)
	return strings.Join(changes, ", ")
}

func rewriteCommandForRemoteIP(command string, ip string) string {
	function, params, ok := parseMgrctlCommand(command)
	if !ok || strings.TrimSpace(ip) == "" {
		return command
	}

	switch function {
	case "site.edit":
		params["site_ipaddrs"] = ip
		params["site_ssl_cert"] = firstNonEmpty(strings.TrimSpace(params["site_name"]), "selfsigned")
	case "emaildomain.edit":
		delete(params, "ip")
		delete(params, "ipsrc")
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

func uniqueStringsPreserveOrder(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniquePackageSteps(values []packageSyncStep) []packageSyncStep {
	result := make([]packageSyncStep, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		key := value.Feature + "\n" + value.Command
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func buildWgetInstallCommand() string {
	return "sh -lc 'if command -v apt-get >/dev/null 2>&1; then export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install -y wget; " +
		"elif command -v dnf >/dev/null 2>&1; then dnf install -y wget; " +
		"elif command -v yum >/dev/null 2>&1; then yum install -y wget; " +
		"else echo unsupported-package-manager; exit 1; fi'"
}

func describePackageStep(title string) string {
	switch title {
	case "packages (web)":
		return "installing package group web"
	case "packages (email)":
		return "installing package group email"
	case "packages (dns)":
		return "installing package group dns"
	case "packages (ftp)":
		return "installing package group ftp"
	case "packages (mysql)":
		return "installing package group mysql"
	case "packages (postgresql)":
		return "installing package group postgresql"
	case "packages (phpmyadmin)":
		return "installing phpmyadmin"
	case "packages (phppgadmin)":
		return "installing phppgadmin"
	case "packages (altphp)":
		return "installing alternative PHP packages"
	case "packages (others)":
		return "installing other packages"
	default:
		return "installing " + strings.TrimSpace(title)
	}
}

func describeRemoteCommand(command string) string {
	function, params, ok := parseMgrctlCommand(command)
	if !ok {
		return "running remote command"
	}

	switch function {
	case "user.edit":
		return "adding user " + firstNonEmpty(params["name"], "<unknown>")
	case "ftp.user.edit":
		return "adding ftp user " + firstNonEmpty(params["name"], "<unknown>")
	case "site.edit":
		return "adding web site " + firstNonEmpty(params["site_name"], "<unknown>")
	case "db.server.edit":
		return "adding database server " + firstNonEmpty(params["name"], "<unknown>")
	case "db.edit":
		return "adding database " + firstNonEmpty(params["name"], "<unknown>")
	case "emaildomain.edit":
		return "adding email domain " + firstNonEmpty(params["name"], "<unknown>")
	case "email.edit":
		name := firstNonEmpty(params["name"], "<unknown>")
		domain := firstNonEmpty(params["domainname"], "<unknown>")
		return "adding email box " + name + "@" + domain
	case "domain.edit":
		return "adding dns zone " + firstNonEmpty(params["name"], "<unknown>")
	case "letsencrypt.generate":
		return "issuing let's encrypt certificate for " + firstNonEmpty(params["domain_name"], firstNonEmpty(params["domain"], "<unknown>"))
	case "sslcert.selfsigned":
		return "creating self-signed certificate for " + firstNonEmpty(params["domain"], "<unknown>")
	case "sslcert.setcrt":
		return "importing ssl certificate"
	default:
		return "running remote command " + function
	}
}
