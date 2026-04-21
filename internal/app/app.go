package app

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var (
	ensureRootHook                  = ensureRoot
	ensureLinuxHook                 = ensureLinux
	acquireLockHook                 = acquireLock
	userHomeDirHook                 = os.UserHomeDir
	askYesNoWithColorHook           = askYesNoWithColor
	runRemoteWorkflowHook           = runRemoteWorkflow
	runRemoteWorkflowWithRunnerHook = runRemoteWorkflowWithRunner
	buildRemoteExecutionPreviewHook = func(a *App, ctx context.Context, data SourceData, commandScopes []string) (remoteExecutionPreview, error) {
		return a.buildRemoteExecutionPreview(ctx, data, commandScopes)
	}
)

func CheckRootPreflight() error {
	return ensureRootHook()
}

func RequiresRootForArgs(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			return false
		case "-v", "--version", "-f", "--file", "-d", "--dest":
			return false
		}
	}
	return true
}

func RequiresRootForConfig(cfg Config) bool {
	if cfg.ShowHelp || cfg.ShowVersion {
		return false
	}
	return strings.TrimSpace(cfg.DBFile) == ""
}

func defaultLockPath() (string, error) {
	home, err := userHomeDirHook()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user home directory for lock file: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("failed to resolve user home directory for lock file: empty home path")
	}
	return filepath.Join(home, ".ispdb", "ispdb.lock"), nil
}

type App struct {
	cfg        Config
	ui         *UI
	logger     *slog.Logger
	logCloser  interface{ Close() error }
	arts       []string
	version    string
	lockFile   *os.File
	binaryName string
}

type remoteExecutionPreview struct {
	groups []CommandGroup
	runner *remoteRunner
}

func New(cfg Config, version string, artFS embed.FS, binaryName string) (*App, error) {
	ui := NewUI()
	ui.silent = cfg.Silent
	logger, closer, err := buildLogger(cfg.LogLevel, cfg.LogFile, cfg.Silent)
	if err != nil {
		return nil, err
	}
	arts, err := loadASCIIArts(artFS)
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:        cfg,
		ui:         ui,
		logger:     logger,
		logCloser:  closer,
		arts:       arts,
		version:    version,
		binaryName: sanitizeBinaryName(binaryName),
	}, nil
}

func (a *App) Run() error {
	if a.logCloser != nil {
		defer a.logCloser.Close()
	}

	requiresRoot := RequiresRootForConfig(a.cfg)
	if requiresRoot {
		if err := ensureRootHook(); err != nil {
			return err
		}
	}

	if a.cfg.MemeMode {
		a.ui.PrintASCII(a.arts)
		return nil
	}

	if a.cfg.ShowVersion {
		a.printBanner()
		return nil
	}

	if err := ensureLinuxHook(); err != nil {
		return err
	}

	suppressMetaOutput := a.shouldSuppressMetaOutput()
	if !suppressMetaOutput {
		a.printBanner()
	}

	if a.cfg.ShowHelp {
		a.ui.Println("")
		a.ui.Println("")
		a.ui.PrintASCII(a.arts)
		a.ui.Println("")
		a.ui.Println("")
		a.ui.Println(buildHelp(a.version, a.binaryName))
		return nil
	}

	if requiresRoot {
		lockPath, err := defaultLockPath()
		if err != nil {
			return err
		}
		lockFile, err := acquireLockHook(lockPath)
		if err != nil {
			return err
		}
		a.lockFile = lockFile
		defer func() {
			if a.lockFile != nil {
				_ = a.lockFile.Close()
			}
		}()
	}

	if a.cfg.BulkMode != "" {
		return runBulkWorkflow(a.ui, a.cfg)
	}

	ctx := context.Background()
	if a.shouldRunRemoteList() {
		return runRemoteListWorkflow(ctx, a.ui, a.logger, a.cfg)
	}
	raw, err := loadSource(ctx, a.cfg)
	if err != nil {
		return fmt.Errorf("%s%s%s", colorRed, a.formatLoadSourceError(err), colorReset)
	}
	backupPath, err := ensureSourceBackup(ctx, a.cfg, raw)
	if err != nil {
		return fmt.Errorf("%sfailed to create database backup for %s: %v%s", colorRed, a.cfg.DBDisplay, err, colorReset)
	}

	data, err := buildSourceData(raw, a.cfg.ISPKey)
	if err != nil {
		return fmt.Errorf("%sfailed to build data model: %w%s", colorRed, err, colorReset)
	}
	data.SourcePath = a.cfg.DBDisplay

	showDataConsole := a.cfg.DestHost == "" && a.cfg.ExportFile == ""
	listScopes := displayScopesFromListMode(a.cfg.ListMode)
	if showDataConsole {
		if !suppressMetaOutput {
			a.printLoadedSource(backupPath, data)
			for _, warning := range data.Warnings {
				a.ui.Warn(warning)
			}
			if len(data.Warnings) > 0 {
				a.ui.Println("")
			}
		}
	}

	allSections := data.sectionsForScopes(dataScopesFromListMode(a.cfg.ListMode))
	listSections := data.listSectionsForScopes(dataScopesFromListMode(a.cfg.ListMode))
	sections := listSections
	listColumns := a.cfg.Columns
	if len(listColumns) > 0 {
		filtered, err := filterSectionsByColumns(sections, listColumns)
		if err != nil {
			return err
		}
		sections = filtered
	}

	consoleDataText := renderSections(sections, true)
	if a.cfg.CleanOutput && canUseCleanOutput(sections) {
		consoleDataText = renderCleanSections(sections)
	}
	needCommands := a.cfg.DestHost != "" ||
		a.cfg.ExportScope == "commands" ||
		(a.cfg.ExportFile != "" && a.cfg.ExportScope == "all") ||
		hasScope(configuredScopeList(a.cfg.ListMode, listModes), "commands") ||
		(hasScope(configuredScopeList(a.cfg.ListMode, listModes), "all") && a.cfg.ListExplicit) ||
		a.cfg.DevMode

	commandScopes := commandScopesFromListMode(a.cfg.ListMode)
	if a.cfg.DestHost != "" && strings.TrimSpace(a.cfg.DestScope) != "" {
		commandScopes = destScopesFromValue(a.cfg.DestScope)
	}

	var commandGroups []CommandGroup
	var commands []string
	var commandWarnings []string
	if needCommands {
		commandGroups, commandWarnings = buildCommandsForScopes(data, commandScopes, CommandBuildOptions{
			DefaultIP: detectLocalIPv4(),
			DefaultNS: defaultNameservers,
			TargetOS:  detectLocalOSName(),
		})
		commands = flattenCommandGroups(commandGroups)
	}

	for _, warning := range commandWarnings {
		if showDataConsole && a.cfg.DevMode {
			a.ui.Warn(warning)
		}
	}

	if a.cfg.ExportFile != "" {
		exportedCount, err := a.writeExport(allSections, commandGroups, commands)
		if err != nil {
			return err
		}
		a.ui.Println(fmt.Sprintf("export successfully %d entries to %s", exportedCount, a.cfg.ExportFile))
	}

	if showDataConsole && a.cfg.ListMode != "" {
		orderedOutput := a.renderOrderedListOutput(data, listScopes, commandGroups)
		if strings.TrimSpace(orderedOutput) == "" {
			if strings.TrimSpace(consoleDataText) != "" {
				a.ui.Println(consoleDataText)
			}
		} else {
			a.ui.Println(orderedOutput)
		}
	}

	if a.cfg.DestHost != "" {
		if !a.cfg.AutoYes {
			preview, err := buildRemoteExecutionPreviewHook(a, ctx, data, commandScopes)
			if err != nil {
				return err
			}
			if err := a.confirmRemoteExecution(preview.groups); err != nil {
				if preview.runner != nil {
					_ = preview.runner.Close()
				}
				return err
			}
			if preview.runner != nil {
				return runRemoteWorkflowWithRunnerHook(ctx, preview.runner, data, commands)
			}
		}
		if err := runRemoteWorkflowHook(ctx, a.ui, a.logger, a.cfg, data, commands); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) writeExport(sections []Section, commandGroups []CommandGroup, commands []string) (int, error) {
	if _, err := os.Stat(a.cfg.ExportFile); err == nil {
		answer, askErr := askYesNoWithColor(fmt.Sprintf("Export file %s already exists. Overwrite?", a.cfg.ExportFile), true, colorYellow)
		if askErr != nil {
			return 0, askErr
		}
		if !answer {
			return 0, fmt.Errorf("%sexport was cancelled by user%s", colorRed, colorReset)
		}
	}

	exportScope := a.cfg.ExportScope
	if exportScope == "" {
		exportScope = defaultExportScope(a.cfg.ListMode)
	}

	exportSectionsList := exportSections(sections, exportScope)
	exportColumns := a.cfg.Columns
	if len(exportColumns) > 0 && exportScope != "commands" {
		filtered, err := filterSectionsByColumns(exportSectionsList, exportColumns)
		if err != nil {
			return 0, err
		}
		exportSectionsList = filtered
	}

	exportedCount := countExportedItems(exportScope, exportSectionsList, commands)
	var content strings.Builder
	switch exportScope {
	case "commands":
		content.WriteString(commandSectionText(commandGroups, false, false))
	default:
		switch a.cfg.ExportFormat {
		case "csv":
			content.WriteString(renderCSV(exportSectionsList, a.cfg.CSVDelimiter))
		case "json":
			payload, err := renderJSONExport(a.cfg.DBDisplay, exportSectionsList)
			if err != nil {
				return 0, err
			}
			content.Write(payload)
		default:
			if a.cfg.CleanOutput && canUseCleanOutput(exportSectionsList) {
				content.WriteString(renderCleanSections(exportSectionsList))
			} else if exportScope == "data" {
				content.WriteString(renderTextSections(exportSectionsList, true, false))
			} else if exportScope == "all" {
				content.WriteString(renderTextSections(exportSectionsList, true, false))
				if len(commands) > 0 {
					content.WriteString("\n\n")
					content.WriteString(commandSectionText(commandGroups, false, false))
				}
			} else {
				content.WriteString(renderTextSections(exportSectionsList, false, false))
			}
		}
	}

	if err := os.WriteFile(a.cfg.ExportFile, []byte(content.String()), 0644); err != nil {
		return 0, err
	}
	return exportedCount, nil
}

func (a *App) printBanner() {
	a.ui.Println(fmt.Sprintf("%sispmanager 5+%s db dump and export tool version %s%s%s", colorGreen, colorReset, colorYellow, a.version, colorReset))
	if !a.cfg.ShowHelp {
		a.ui.Println("")
	}
}

func (a *App) shouldSuppressMetaOutput() bool {
	return a.cfg.CleanOutput && a.cfg.DestHost == "" && a.cfg.ExportFile == "" && a.cfg.ListMode != ""
}

func (a *App) shouldRunRemoteList() bool {
	return a.cfg.DestHost != "" && a.cfg.ListExplicit
}

func (a *App) renderOrderedListOutput(data SourceData, scopes []string, commandGroups []CommandGroup) string {
	if len(scopes) == 0 {
		return ""
	}

	parts := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		switch scope {
		case "commands":
			if len(commandGroups) == 0 {
				parts = append(parts, noSyncCommandsText())
				continue
			}
			parts = append(parts, commandSectionTextWithOptions(commandGroups, true, true, "commands to run at remote server:", true))
		default:
			sections := data.listSectionsForScopes([]string{scope})
			if len(a.cfg.Columns) > 0 {
				filtered, err := filterSectionsByColumns(sections, a.cfg.Columns)
				if err == nil {
					sections = filtered
				}
			}
			if len(sections) == 0 {
				continue
			}
			if a.cfg.CleanOutput && canUseCleanOutput(sections) {
				parts = append(parts, renderCleanSections(sections))
			} else {
				parts = append(parts, renderSections(sections, true))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func (a *App) confirmRemoteExecution(commandGroups []CommandGroup) error {
	if len(commandGroups) > 0 {
		a.ui.Println(commandSectionText(commandGroups, true, true))
		a.ui.Println("")
	}
	answer, err := askYesNoWithColorHook("Continue with remote sync?", true, colorYellow)
	if err != nil {
		return err
	}
	if !answer {
		return fmt.Errorf("%sdestination execution was cancelled by user%s", colorRed, colorReset)
	}
	return nil
}

func (a *App) buildRemoteExecutionPreview(ctx context.Context, data SourceData, commandScopes []string) (remoteExecutionPreview, error) {
	runner, err := connectRemoteRunner(a.ui, a.logger, a.cfg, true)
	if err != nil {
		return remoteExecutionPreview{}, err
	}
	if err := runner.ensureDestinationRoot(ctx); err != nil {
		_ = runner.Close()
		return remoteExecutionPreview{}, err
	}

	state := remotePreviewState{}
	if primaryIP, err := runner.primaryIPAddress(ctx); err == nil {
		state.primaryIP = primaryIP
	}

	panelInstalled, err := runner.panelInstalled(ctx)
	if err != nil {
		_ = runner.Close()
		return remoteExecutionPreview{}, err
	}
	if panelInstalled {
		info, err := runner.licenseInfo(ctx)
		if err != nil {
			_ = runner.Close()
			return remoteExecutionPreview{}, err
		}
		state.targetOS = info["os"]
		state.targetPanel = info["panel_name"]

		records, err := runner.featureRecords(ctx)
		if err != nil {
			_ = runner.Close()
			return remoteExecutionPreview{}, err
		}
		state.currentPackages = installedPackagesFromFeatures(records, state.targetOS)

		inventory, err := runner.loadRemoteInventory(ctx)
		if err != nil {
			_ = runner.Close()
			return remoteExecutionPreview{}, err
		}
		state.inventory = inventory
	}

	groups, _ := buildRemoteExecutionPreviewGroups(data, commandScopes, a.cfg, state)
	return remoteExecutionPreview{groups: groups, runner: runner}, nil
}

func (a *App) printLoadedSource(backupPath string, data SourceData) {
	if backupPath != "" {
		a.ui.Println("DB backup: " + backupPath)
	}
	a.ui.Println("DB: " + a.cfg.DBDisplay)
	a.ui.Println("DB format: " + data.Format)
	if data.PrivateKeyUsed && a.cfg.ISPKey != "" {
		a.ui.Println("privkey: " + a.cfg.ISPKey)
	}
	a.ui.Println("")
}

func (a *App) formatLoadSourceError(err error) string {
	source := strings.TrimSpace(a.cfg.DBDisplay)
	if source == "" {
		source = strings.TrimSpace(a.cfg.DBFile)
	}
	if source == "" {
		source = "default source"
	}
	message := fmt.Sprintf("failed to load database source %s: %v", source, err)
	if a.cfg.DBFile == "" || a.cfg.UseLocalMySQL {
		message += fmt.Sprintf("\nTip: use %s -h to see examples", sanitizeBinaryName(a.binaryName))
	}
	return message
}

func countExportedItems(scope string, sections []Section, commands []string) int {
	switch scope {
	case "commands":
		return len(commands)
	case "all":
		total := len(commands)
		for _, section := range sections {
			total += len(section.Rows)
		}
		return total
	default:
		total := 0
		for _, section := range sections {
			total += len(section.Rows)
		}
		return total
	}
}
