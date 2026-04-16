package app

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

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

	if a.cfg.MemeMode {
		a.ui.PrintASCII(a.arts)
		return nil
	}

	if a.cfg.ShowVersion {
		a.printBanner()
		return nil
	}

	if err := ensureLinux(); err != nil {
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

	if err := ensureRoot(); err != nil {
		return err
	}

	lockFile, err := acquireLock(defaultLock)
	if err != nil {
		return err
	}
	a.lockFile = lockFile
	defer func() {
		if a.lockFile != nil {
			_ = a.lockFile.Close()
		}
	}()

	if a.cfg.BulkMode != "" {
		return runBulkWorkflow(a.ui, a.cfg)
	}

	ctx := context.Background()
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

	allSections := data.sections(a.cfg.ListMode)
	sections := allSections
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
		a.cfg.ListMode == "commands" ||
		(a.cfg.ListMode == "all" && a.cfg.ListExplicit) ||
		a.cfg.DevMode

	commandScope := a.cfg.ListMode
	if commandScope == "" || commandScope == "commands" {
		commandScope = "all"
	}

	var commandGroups []CommandGroup
	var commands []string
	var commandWarnings []string
	consoleCommandText := ""
	if needCommands {
		commandGroups, commandWarnings = buildCommands(data, commandScope, CommandBuildOptions{
			DefaultIP: detectLocalIPv4(),
			DefaultNS: defaultNameservers,
			TargetOS:  detectLocalOSName(),
		})
		commands = flattenCommandGroups(commandGroups)
		consoleCommandText = commandSectionText(commandGroups, true, true)
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
		if a.cfg.ListMode != "commands" && strings.TrimSpace(consoleDataText) != "" {
			a.ui.Println(consoleDataText)
		}
		if a.cfg.ListMode == "commands" || (a.cfg.ListMode == "all" && a.cfg.ListExplicit) {
			if a.cfg.ListMode != "commands" && strings.TrimSpace(consoleDataText) != "" {
				a.ui.Println("")
				a.ui.Println("")
			}
			a.ui.Println(consoleCommandText)
		}
	}

	if a.cfg.DestHost != "" {
		if err := runRemoteWorkflow(ctx, a.ui, a.logger, a.cfg, data, commands); err != nil {
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

func (a *App) printLoadedSource(backupPath string, data SourceData) {
	if backupPath != "" {
		a.ui.Println("DB backup: " + backupPath)
	}
	a.ui.Println("DB: " + a.cfg.DBDisplay)
	a.ui.Println("DB format: " + data.Format)
	if a.cfg.ISPKey != "" {
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
