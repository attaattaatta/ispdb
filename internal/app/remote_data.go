package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type remoteLoadedSource struct {
	Data       SourceData
	Display    string
	KeyDisplay string
	cleanup    func()
}

type remoteRawSourceBundle struct {
	raw          rawSource
	display      string
	keyDisplay   string
	keyLocalPath string
	cleanup      func()
}

type remoteListCommandSections struct {
	localWithRemote []CommandGroup
	remoteWithLocal []CommandGroup
}

type remotePreviewState struct {
	primaryIP       string
	targetOS        string
	targetPanel     string
	currentPackages map[string]struct{}
	inventory       *remoteInventory
}

func runRemoteListWorkflow(ctx context.Context, ui *UI, logger *slog.Logger, cfg Config) error {
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
	loaded, err := runner.loadRemoteSourceData(ctx)
	if err != nil {
		return err
	}
	defer loaded.cleanup()

	ui.Println("DB: " + loaded.Display)
	ui.Println("DB format: " + loaded.Data.Format)
	if loaded.KeyDisplay != "" {
		ui.Println("privkey: " + loaded.KeyDisplay)
	} else if strings.TrimSpace(loaded.Data.KeyStatusMessage) != "" {
		ui.Println("privkey: " + colorYellow + loaded.Data.KeyStatusMessage + colorReset)
		if strings.TrimSpace(loaded.Data.KeyStatusReason) != "" {
			ui.Println(loaded.Data.KeyStatusReason)
		}
	}
	ui.Println("")
	for _, warning := range loaded.Data.Warnings {
		ui.Warn(warning)
	}
	if len(loaded.Data.Warnings) > 0 {
		ui.Println("")
	}

	commandSections := remoteListCommandSections{}
	if hasScope(configuredScopeList(cfg.ListMode, listModes), "commands") || hasScope(configuredScopeList(cfg.ListMode, listModes), "all") {
		localOSTarget := detectLocalOSName()
		remoteOSTarget := ""
		remotePanelTarget := ""
		if info, infoErr := runner.licenseInfo(ctx); infoErr == nil {
			remoteOSTarget = info["os"]
			remotePanelTarget = info["panel_name"]
		}

		if localData, ok := loadLocalCompareSourceData(ctx, cfg, logger); ok {
			localInventory := buildRemoteInventory(localData)
			remoteInventory := buildRemoteInventory(loaded.Data)

			localSyncData := filterSourceDataByMissingInventory(loaded.Data, localInventory)
			localSyncData.Packages = loaded.Data.Packages
			remoteSyncData := filterSourceDataByMissingInventory(localData, remoteInventory)
			remoteSyncData.Packages = localData.Packages

			localDefaultIP, ipErr := runner.primaryIPAddress(ctx)
			if ipErr != nil {
				localDefaultIP = ""
			}
			commandScopes := commandScopesFromListMode(cfg.ListMode)
			commandSections.localWithRemote, _ = buildCommandsForScopes(localSyncData, commandScopes, CommandBuildOptions{
				DefaultIP:        localDefaultIP,
				DefaultNS:        defaultNameservers,
				TargetOS:         localOSTarget,
				CurrentPackages:  localInventory.packages,
				NoDeletePackages: false,
			})
			commandSections.remoteWithLocal, _ = buildCommandsForScopes(remoteSyncData, commandScopes, CommandBuildOptions{
				DefaultIP:        detectLocalIPv4(),
				DefaultNS:        defaultNameservers,
				TargetOS:         remoteOSTarget,
				TargetPanel:      remotePanelTarget,
				CurrentPackages:  remoteInventory.packages,
				NoDeletePackages: cfg.NoDeletePackages,
			})
		} else {
			localDefaultIP, ipErr := runner.primaryIPAddress(ctx)
			if ipErr != nil {
				localDefaultIP = ""
			}
			commandScopes := commandScopesFromListMode(cfg.ListMode)
			commandSections.localWithRemote, _ = buildCommandsForScopes(loaded.Data, commandScopes, CommandBuildOptions{
				DefaultIP: localDefaultIP,
				DefaultNS: defaultNameservers,
				TargetOS:  localOSTarget,
			})
		}
	}

	output, err := renderOrderedRemoteListOutput(loaded.Data, cfg, commandSections)
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) != "" {
		ui.Println(output)
	}
	return nil
}

func buildRemoteExecutionPreviewGroups(data SourceData, scopes []string, cfg Config, state remotePreviewState) ([]CommandGroup, []string) {
	previewData := data
	if !cfg.Overwrite && state.inventory != nil {
		previewData = filterSourceDataByMissingInventory(data, *state.inventory)
		previewData.Packages = data.Packages
	}

	return buildCommandsForScopes(previewData, scopes, CommandBuildOptions{
		DefaultIP:        state.primaryIP,
		DefaultNS:        defaultNameservers,
		TargetOS:         state.targetOS,
		TargetPanel:      state.targetPanel,
		CurrentPackages:  state.currentPackages,
		NoDeletePackages: cfg.NoDeletePackages,
	})
}

func loadLocalCompareSourceData(ctx context.Context, cfg Config, logger *slog.Logger) (SourceData, bool) {
	localCfg := cfg
	localCfg.DestHost = ""
	localCfg.DestAuth = ""
	localCfg.DestScope = ""
	localCfg.DestPort = 0

	raw, err := loadSource(ctx, localCfg)
	if err != nil {
		if logger != nil {
			logger.Debug("failed to load local compare source", "error", err)
		}
		return SourceData{}, false
	}

	data, err := buildSourceData(raw, localCfg.ISPKey)
	if err != nil {
		if logger != nil {
			logger.Debug("failed to build local compare source data", "error", err)
		}
		return SourceData{}, false
	}
	return data, true
}

func renderOrderedRemoteListOutput(data SourceData, cfg Config, commandSections remoteListCommandSections) (string, error) {
	scopes := displayScopesFromListMode(cfg.ListMode)
	if len(scopes) == 0 {
		scopes = append([]string{}, dataScopeOrder...)
	}

	parts := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		switch scope {
		case "commands":
			commandParts := make([]string, 0, 2)
			if len(commandSections.localWithRemote) > 0 {
				commandParts = append(commandParts, commandSectionTextWithOptions(commandSections.localWithRemote, true, true, "sync local with remote:", true))
			}
			if len(commandSections.remoteWithLocal) > 0 {
				commandParts = append(commandParts, commandSectionTextWithOptions(commandSections.remoteWithLocal, true, true, "sync remote with local:", true))
			}
			if len(commandParts) == 0 {
				parts = append(parts, noSyncCommandsText())
				continue
			}
			parts = append(parts, strings.Join(commandParts, "\n\n"))
		default:
			sections := data.listSectionsForScopes([]string{scope})
			if len(cfg.Columns) > 0 {
				filtered, err := filterSectionsByColumns(sections, cfg.Columns)
				if err != nil {
					return "", err
				}
				sections = filtered
			}
			if len(sections) == 0 {
				continue
			}
			if cfg.CleanOutput && canUseCleanOutput(sections) {
				parts = append(parts, renderCleanSections(sections))
			} else {
				parts = append(parts, renderSections(sections, true))
			}
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func (r *remoteRunner) loadRemoteSourceData(ctx context.Context) (remoteLoadedSource, error) {
	bundle, err := r.loadRemoteRawSource(ctx)
	if err != nil {
		return remoteLoadedSource{}, err
	}
	data, err := buildSourceData(bundle.raw, bundle.keyLocalPath)
	if err != nil {
		if bundle.cleanup != nil {
			bundle.cleanup()
		}
		return remoteLoadedSource{}, err
	}
	data.SourcePath = bundle.display
	return remoteLoadedSource{
		Data:       data,
		Display:    bundle.display,
		KeyDisplay: bundle.keyDisplay,
		cleanup:    bundle.cleanup,
	}, nil
}

func (r *remoteRunner) loadRemoteRawSource(ctx context.Context) (remoteRawSourceBundle, error) {
	const remoteDBPath = "/usr/local/mgr5/etc/ispmgr.db"
	const remoteKeyPath = "/usr/local/mgr5/etc/ispmgr.pem"
	const remoteMyCNFPath = "/root/.my.cnf"

	exists, err := r.remoteFileExists(remoteDBPath)
	if err != nil {
		return remoteRawSourceBundle{}, err
	}

	if exists {
		localDBPath, cleanupDB, err := r.downloadRemoteSQLiteBundle(remoteDBPath)
		if err != nil {
			return remoteRawSourceBundle{}, err
		}

		cleanup := cleanupDB
		keyLocalPath := ""
		keyDisplay := ""
		if keyExists, keyErr := r.remoteFileExists(remoteKeyPath); keyErr == nil && keyExists {
			localKeyPath, cleanupKey, downloadErr := r.downloadRemoteTempFile(remoteKeyPath, "ispdb-remote-*.pem")
			if downloadErr == nil {
				keyLocalPath = localKeyPath
				keyDisplay = remoteKeyPath
				prevCleanup := cleanup
				cleanup = func() {
					prevCleanup()
					cleanupKey()
				}
			}
		}

		raw, err := loadSQLiteSource(ctx, localDBPath)
		if err != nil {
			cleanup()
			return remoteRawSourceBundle{}, err
		}
		return remoteRawSourceBundle{
			raw:          raw,
			display:      remoteDBPath,
			keyDisplay:   keyDisplay,
			keyLocalPath: keyLocalPath,
			cleanup:      cleanup,
		}, nil
	}

	password, err := r.readRemoteMySQLPassword(remoteMyCNFPath)
	if err != nil {
		return remoteRawSourceBundle{}, fmt.Errorf("%sremote SQLite database was not found and MySQL password was not found in %s: %w%s", colorRed, remoteMyCNFPath, err, colorReset)
	}

	keyLocalPath := ""
	keyDisplay := ""
	cleanup := func() {}
	if keyExists, keyErr := r.remoteFileExists(remoteKeyPath); keyErr == nil && keyExists {
		localKeyPath, cleanupKey, downloadErr := r.downloadRemoteTempFile(remoteKeyPath, "ispdb-remote-*.pem")
		if downloadErr == nil {
			keyLocalPath = localKeyPath
			keyDisplay = remoteKeyPath
			cleanup = cleanupKey
		}
	}

	raw, err := loadRemoteMySQLSourceViaSSH(ctx, r.client, password)
	if err != nil {
		cleanup()
		return remoteRawSourceBundle{}, err
	}
	return remoteRawSourceBundle{
		raw:          raw,
		display:      fmt.Sprintf("%s:%d", defaultMySQLHost, defaultMySQLPort),
		keyDisplay:   keyDisplay,
		keyLocalPath: keyLocalPath,
		cleanup:      cleanup,
	}, nil
}

func (r *remoteRunner) remoteFileExists(path string) (bool, error) {
	_, err := r.sftpClient.Stat(path)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (r *remoteRunner) downloadRemoteTempFile(remotePath string, pattern string) (string, func(), error) {
	remoteFile, err := r.sftpClient.Open(remotePath)
	if err != nil {
		return "", nil, err
	}
	defer remoteFile.Close()

	localFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}

	if _, err := io.Copy(localFile, remoteFile); err != nil {
		_ = localFile.Close()
		_ = os.Remove(localFile.Name())
		return "", nil, err
	}
	if err := localFile.Close(); err != nil {
		_ = os.Remove(localFile.Name())
		return "", nil, err
	}

	return localFile.Name(), func() {
		_ = os.Remove(localFile.Name())
	}, nil
}

func (r *remoteRunner) downloadRemoteSQLiteBundle(remotePath string) (string, func(), error) {
	localDir, err := os.MkdirTemp("", "ispdb-remote-sqlite-*")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		_ = os.RemoveAll(localDir)
	}

	localDBPath := filepath.Join(localDir, filepath.Base(remotePath))
	if err := r.copyRemoteFileToPath(remotePath, localDBPath); err != nil {
		cleanup()
		return "", nil, err
	}

	for _, sidecar := range sqliteSidecarPaths(remotePath) {
		exists, statErr := r.remoteFileExists(sidecar)
		if statErr != nil {
			cleanup()
			return "", nil, statErr
		}
		if !exists {
			continue
		}
		localSidecarPath := filepath.Join(localDir, filepath.Base(sidecar))
		if err := r.copyRemoteFileToPath(sidecar, localSidecarPath); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	return localDBPath, cleanup, nil
}

func sqliteSidecarPaths(remotePath string) []string {
	return []string{
		remotePath + "-wal",
		remotePath + "-shm",
		remotePath + "-journal",
	}
}

func (r *remoteRunner) copyRemoteFileToPath(remotePath string, localPath string) error {
	remoteFile, err := r.sftpClient.Open(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(localFile, remoteFile); err != nil {
		_ = localFile.Close()
		_ = os.Remove(localPath)
		return err
	}
	if err := localFile.Close(); err != nil {
		_ = os.Remove(localPath)
		return err
	}
	return nil
}

func (r *remoteRunner) readRemoteMySQLPassword(path string) (string, error) {
	remoteFile, err := r.sftpClient.Open(path)
	if err != nil {
		return "", err
	}
	defer remoteFile.Close()

	content, err := io.ReadAll(remoteFile)
	if err != nil {
		return "", err
	}
	return parseMyCNFPasswordContent(path, string(content))
}

func parseMyCNFPasswordContent(path string, content string) (string, error) {
	inClientSection := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inClientSection = strings.EqualFold(strings.Trim(line, "[]"), "client")
			continue
		}
		if !inClientSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "password") {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("password was not found in %s [client] section", filepath.Clean(path))
}

func loadRemoteMySQLSourceViaSSH(ctx context.Context, client *ssh.Client, password string) (rawSource, error) {
	dialName := fmt.Sprintf("ssh-mysql-%d", time.Now().UnixNano())
	mysqlDriver.RegisterDialContext(dialName, func(ctx context.Context, addr string) (net.Conn, error) {
		return client.Dial("tcp", addr)
	})

	dsn := fmt.Sprintf("root:%s@%s(%s:%d)/ispmgr?charset=utf8mb4&parseTime=true&loc=Local", password, dialName, defaultMySQLHost, defaultMySQLPort)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return rawSource{}, err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return rawSource{}, err
	}
	tables, err := queryTrackedMySQLTables(ctx, db)
	if err != nil {
		return rawSource{}, err
	}
	return rawSource{format: "MySQL", tables: tables}, nil
}
