package app

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
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

func (a *App) internalLogEnabled() bool {
	return a != nil && a.cfg.LogExplicit && a.logger != nil
}

func (a *App) logDebug(message string, args ...any) {
	if a.internalLogEnabled() {
		a.logger.Debug(message, args...)
	}
}

func (a *App) logInfo(message string, args ...any) {
	if a.internalLogEnabled() {
		a.logger.Info(message, args...)
	}
}

func (a *App) logWarn(message string, args ...any) {
	if a.internalLogEnabled() {
		a.logger.Warn(message, args...)
	}
}

type remoteExecutionPreview struct {
	groups   []CommandGroup
	commands []string
	runner   *remoteRunner
}

type preparedSource struct {
	backupPath      string
	data            SourceData
	allSections     []Section
	listSections    []Section
	consoleDataText string
	commandScopes   []string
	commandGroups   []CommandGroup
	commands        []string
	commandWarnings []string
	listScopes      []string
	showDataConsole bool
	suppressMeta    bool
}

func New(cfg Config, version string, artFS embed.FS, binaryName string) (*App, error) {
	ui := NewUI()
	ui.silent = cfg.Silent
	if strings.TrimSpace(cfg.LogFile) != "" && strings.TrimSpace(cfg.LogLevel) != "off" {
		setProgramOutputLogFile(cfg.LogFile)
	} else {
		setProgramOutputLogFile("")
	}
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
		a.logInfo("starting bulk workflow", "mode", a.cfg.BulkMode, "type", a.cfg.BulkType)
		return runBulkWorkflow(a.ui, a.cfg)
	}

	ctx := context.Background()
	if a.shouldRunRemoteList() {
		a.logInfo("starting remote list workflow", "host", a.cfg.DestHost, "scopes", a.cfg.ListMode)
		return runRemoteListWorkflow(ctx, a.ui, a.logger, a.cfg)
	}
	prepared, err := a.prepareSource(ctx, suppressMetaOutput)
	if err != nil {
		return err
	}
	if err := a.handleExport(prepared); err != nil {
		return err
	}
	a.handleLocalListOutput(prepared)
	if err := a.handleRemoteExecution(ctx, prepared); err != nil {
		return err
	}

	return nil
}

func (a *App) prepareSource(ctx context.Context, suppressMetaOutput bool) (preparedSource, error) {
	a.logInfo("loading source data", "source", a.cfg.DBDisplay)
	raw, err := loadSource(ctx, a.cfg)
	if err != nil {
		return preparedSource{}, fmt.Errorf("%s%s%s", colorRed, a.formatLoadSourceError(err), colorReset)
	}
	a.logDebug("source data loaded", "format", raw.format, "tables", len(raw.tables))

	a.logInfo("creating local source backup")
	backupPath, err := ensureSourceBackup(ctx, a.cfg, raw)
	if err != nil {
		return preparedSource{}, fmt.Errorf("%sfailed to create database backup for %s: %v%s", colorRed, a.cfg.DBDisplay, err, colorReset)
	}
	if backupPath != "" {
		a.logInfo("local source backup ready", "path", backupPath)
	}

	a.logInfo("building source data model")
	data, err := buildSourceData(raw, a.cfg.ISPKey)
	if err != nil {
		return preparedSource{}, fmt.Errorf("%sfailed to build data model: %w%s", colorRed, err, colorReset)
	}
	a.logSourceDataSummary(data)
	data.SourcePath = a.cfg.DBDisplay

	return a.prepareSectionsAndCommands(data, backupPath, suppressMetaOutput)
}

func (a *App) logSourceDataSummary(data SourceData) {
	a.logDebug("source data model built",
		"packages", len(data.Packages),
		"users", len(data.Users),
		"ftp_users", len(data.FTPUsers),
		"web_domains", len(data.WebDomains),
		"db_servers", len(data.DBServers),
		"databases", len(data.Databases),
		"db_users", len(data.DBUsers),
		"email_domains", len(data.EmailDomains),
		"email_boxes", len(data.EmailBoxes),
		"dns", len(data.DNSDomains),
	)
}

func (a *App) prepareSectionsAndCommands(data SourceData, backupPath string, suppressMetaOutput bool) (preparedSource, error) {
	prepared := preparedSource{
		backupPath:      backupPath,
		data:            data,
		showDataConsole: a.cfg.DestHost == "" && a.cfg.ExportFile == "",
		listScopes:      displayScopesFromListMode(a.cfg.ListMode),
		commandScopes:   a.resolveCommandScopes(),
		suppressMeta:    suppressMetaOutput,
	}

	if prepared.showDataConsole && !prepared.suppressMeta {
		a.printLoadedSource(prepared.backupPath, prepared.data)
		a.printDataWarnings(prepared.data.Warnings)
	}

	scopeList := dataScopesFromListMode(a.cfg.ListMode)
	prepared.allSections = data.sectionsForScopes(scopeList)
	listSections, err := a.prepareListSections(data.listSectionsForScopes(scopeList))
	if err != nil {
		return preparedSource{}, err
	}
	prepared.listSections = listSections
	prepared.consoleDataText = a.renderConsoleSections(prepared.listSections)

	if a.needCommands() {
		a.logInfo("building command groups", "scopes", strings.Join(prepared.commandScopes, ","))
		prepared.commandGroups, prepared.commandWarnings = buildCommandsForScopes(data, prepared.commandScopes, CommandBuildOptions{
			DefaultIP:   detectLocalIPv4(),
			DefaultNS:   defaultNameservers,
			TargetOS:    detectLocalOSName(),
			Destination: a.cfg.DestHost != "",
		})
		if a.cfg.DestHost != "" {
			prepared.commandGroups = reorderDestCommandGroups(prepared.commandGroups)
		}
		prepared.commands = flattenCommandGroups(prepared.commandGroups)
		a.logDebug("command groups built", "groups", len(prepared.commandGroups), "commands", len(prepared.commands))
	}

	for _, warning := range prepared.commandWarnings {
		if prepared.showDataConsole && a.cfg.DevMode {
			a.ui.Warn(warning)
		}
	}

	return prepared, nil
}

func (a *App) prepareListSections(sections []Section) ([]Section, error) {
	if len(a.cfg.Columns) == 0 {
		return sections, nil
	}
	a.logDebug("filtering list sections by columns", "columns", strings.Join(a.cfg.Columns, ","))
	filtered, err := filterSectionsByColumns(sections, a.cfg.Columns)
	if err != nil {
		return nil, err
	}
	return filtered, nil
}

func (a *App) renderConsoleSections(sections []Section) string {
	text := renderSections(sections, true)
	if a.cfg.CleanOutput && canUseCleanOutput(sections) {
		return renderCleanSections(sections)
	}
	return text
}

func (a *App) printDataWarnings(warnings []string) {
	for _, warning := range warnings {
		a.ui.Warn(warning)
	}
	if len(warnings) > 0 {
		a.ui.Println("")
	}
}

func (a *App) resolveCommandScopes() []string {
	if a.cfg.DestHost != "" && strings.TrimSpace(a.cfg.DestScope) != "" {
		return destExecutionScopesFromValue(a.cfg.DestScope)
	}
	return commandScopesFromListMode(a.cfg.ListMode)
}

func (a *App) needCommands() bool {
	return a.cfg.DestHost != "" ||
		a.cfg.ExportScope == "commands" ||
		(a.cfg.ExportFile != "" && a.cfg.ExportScope == "all") ||
		hasScope(configuredScopeList(a.cfg.ListMode, listModes), "commands") ||
		(hasScope(configuredScopeList(a.cfg.ListMode, listModes), "all") && a.cfg.ListExplicit) ||
		a.cfg.DevMode
}

func (a *App) handleExport(prepared preparedSource) error {
	if a.cfg.ExportFile == "" {
		return nil
	}
	a.logInfo("writing export file", "file", a.cfg.ExportFile, "scopes", strings.Join(configuredExportScopeList(a.cfg.ExportScope, a.cfg.ListMode), ","))
	exportedCount, err := a.writeExport(prepared.allSections, prepared.commandGroups, prepared.commands)
	if err != nil {
		return err
	}
	a.logInfo("export file written", "file", a.cfg.ExportFile, "entries", exportedCount)
	a.ui.Println(fmt.Sprintf("export successfully %d entries to %s", exportedCount, a.cfg.ExportFile))
	return nil
}

func (a *App) handleLocalListOutput(prepared preparedSource) {
	if !prepared.showDataConsole || a.cfg.ListMode == "" {
		return
	}
	orderedOutput := a.renderOrderedListOutput(prepared.data, prepared.listScopes, prepared.commandGroups)
	if strings.TrimSpace(orderedOutput) == "" {
		if strings.TrimSpace(prepared.consoleDataText) != "" {
			a.ui.Println(prepared.consoleDataText)
		}
		return
	}
	a.ui.Println(orderedOutput)
}

func (a *App) handleRemoteExecution(ctx context.Context, prepared preparedSource) error {
	if a.cfg.DestHost == "" {
		return nil
	}
	if !a.cfg.AutoYes {
		a.logInfo("building remote execution preview", "host", a.cfg.DestHost, "scopes", strings.Join(prepared.commandScopes, ","))
		preview, err := buildRemoteExecutionPreviewHook(a, ctx, prepared.data, prepared.commandScopes)
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
			return runRemoteWorkflowWithRunnerHook(ctx, preview.runner, prepared.data, preview.groups)
		}
	}
	return runRemoteWorkflowHook(ctx, a.ui, a.logger, a.cfg, prepared.data, prepared.commandGroups)
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

	exportScopeList := configuredExportScopeList(a.cfg.ExportScope, a.cfg.ListMode)
	exportSectionsList := exportSectionsForScopes(sections, exportScopeList)
	exportColumns := a.cfg.Columns
	if len(exportColumns) > 0 && !hasScope(exportScopeList, "commands") {
		filtered, err := filterSectionsByColumns(exportSectionsList, exportColumns)
		if err != nil {
			return 0, err
		}
		exportSectionsList = filtered
	}

	exportedCount := countExportedItems(exportScopeList, exportSectionsList, commands)
	var content strings.Builder
	switch a.cfg.ExportFormat {
	case "csv":
		content.WriteString(renderCSV(exportSectionsList, a.cfg.CSVDelimiter, a.cfg.NoHeaders))
	case "json":
		payload, err := renderJSONExport(a.cfg.DBDisplay, exportSectionsList, a.cfg.NoHeaders)
		if err != nil {
			return 0, err
		}
		content.Write(payload)
	default:
		content.WriteString(renderOrderedExportText(exportSectionsList, commandGroups, commands, exportScopeList, a.cfg.CleanOutput, a.cfg.NoHeaders))
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

	lastScopeForSection := make(map[string]int)
	for scopeIndex, scope := range scopes {
		if scope == "commands" {
			continue
		}
		for _, section := range data.listSectionsForScopes([]string{scope}) {
			lastScopeForSection[section.Title] = scopeIndex
		}
	}

	parts := make([]string, 0, len(scopes))
	for scopeIndex, scope := range scopes {
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
			sections = filterSectionsForScopeIndex(sections, lastScopeForSection, scopeIndex)
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

func filterSectionsForScopeIndex(sections []Section, lastScopeForSection map[string]int, scopeIndex int) []Section {
	if len(sections) == 0 {
		return sections
	}
	filtered := make([]Section, 0, len(sections))
	for _, section := range sections {
		if last, ok := lastScopeForSection[section.Title]; ok && last != scopeIndex {
			continue
		}
		filtered = append(filtered, section)
	}
	return filtered
}

func reorderDestCommandGroups(groups []CommandGroup) []CommandGroup {
	if len(groups) < 2 {
		return groups
	}

	type scoredGroup struct {
		group CommandGroup
		index int
		score int
	}

	scored := make([]scoredGroup, 0, len(groups))
	for index, group := range groups {
		scored = append(scored, scoredGroup{
			group: group,
			index: index,
			score: destCommandGroupScore(group.Title),
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].index < scored[j].index
		}
		return scored[i].score < scored[j].score
	})

	reordered := make([]CommandGroup, 0, len(groups))
	for _, item := range scored {
		reordered = append(reordered, item.group)
	}
	return reordered
}

func destCommandGroupScore(title string) int {
	switch {
	case strings.HasPrefix(title, "packages ("):
		return 0
	case title == "users":
		return 10
	case title == "dns":
		return 20
	case title == "ftp users":
		return 30
	default:
		return 40
	}
}

func (a *App) buildRemoteExecutionPreview(ctx context.Context, data SourceData, commandScopes []string) (remoteExecutionPreview, error) {
	a.logDebug("connecting remote preview runner", "host", a.cfg.DestHost)
	runner, err := connectRemoteRunner(a.ui, a.logger, a.cfg, true)
	if err != nil {
		return remoteExecutionPreview{}, err
	}
	if err := runner.ensureDestinationRoot(ctx); err != nil {
		_ = runner.Close()
		return remoteExecutionPreview{}, err
	}

	state, panelInstalled, err := a.loadRemotePreviewState(ctx, runner)
	if err != nil {
		_ = runner.Close()
		return remoteExecutionPreview{}, err
	}

	groups, _ := buildRemoteExecutionPreviewGroups(data, commandScopes, a.cfg, state)
	if panelInstalled {
		a.logDebug("pruning remote preview commands using destination forms", "groups", len(groups))
		groups = pruneRemoteExecutionPreviewGroups(ctx, runner, groups)
	}
	a.logInfo("remote execution preview ready", "groups", len(groups))
	return remoteExecutionPreview{groups: groups, commands: flattenCommandGroups(groups), runner: runner}, nil
}

func (a *App) loadRemotePreviewState(ctx context.Context, runner *remoteRunner) (remotePreviewState, bool, error) {
	state := remotePreviewState{}
	if primaryIP, err := runner.primaryIPAddress(ctx); err == nil {
		state.primaryIP = primaryIP
		a.logDebug("destination primary ip detected", "ip", primaryIP)
	}
	if osName, err := runner.remoteOSRelease(ctx); err == nil && strings.TrimSpace(osName) != "" {
		state.targetOS = strings.TrimSpace(osName)
		a.logDebug("destination os detected from os-release", "os", state.targetOS)
	}

	panelInstalled, err := runner.panelInstalled(ctx)
	if err != nil {
		return remotePreviewState{}, false, err
	}
	if !panelInstalled {
		return state, false, nil
	}

	a.logDebug("destination panel detected, collecting state")
	info, err := runner.licenseInfo(ctx)
	if err != nil {
		return remotePreviewState{}, false, err
	}
	if strings.TrimSpace(info["os"]) != "" {
		state.targetOS = info["os"]
	}
	state.targetPanel = info["panel_name"]

	records, err := runner.featureRecords(ctx)
	if err != nil {
		return remotePreviewState{}, false, err
	}
	state.currentPackages = installedPackagesFromFeatures(records, state.targetOS)
	a.logDebug("destination package state loaded", "packages", len(state.currentPackages))

	inventory, err := runner.loadRemoteInventory(ctx)
	if err != nil {
		return remotePreviewState{}, false, err
	}
	state.inventory = inventory
	if certs, certErr := runner.listInactiveSSLCerts(ctx); certErr == nil {
		state.inactiveSSLCerts = certs
		a.logDebug("destination inactive SSL certificates loaded", "count", len(certs))
	} else {
		a.logDebug("failed to load destination inactive SSL certificates", "error", certErr)
	}
	return state, true, nil
}

func pruneRemoteExecutionPreviewGroups(ctx context.Context, runner *remoteRunner, groups []CommandGroup) []CommandGroup {
	if runner == nil || len(groups) == 0 {
		return groups
	}

	result := make([]CommandGroup, 0, len(groups))
	for _, group := range groups {
		prunedGroup := group
		prunedGroup.Commands = append([]string(nil), group.Commands...)
		for index, command := range prunedGroup.Commands {
			function, params, ok := parseMgrctlCommand(command)
			if !ok {
				continue
			}
			switch {
			case function == "feature.edit":
				step := packageSyncStep{
					Title:   group.Title,
					Feature: strings.TrimSpace(params["elid"]),
					Command: command,
				}
				prunedStep, err := runner.pruneFeatureStep(ctx, step)
				if err == nil {
					prunedGroup.Commands[index] = prunedStep.Command
				}
			case supportsFormPruning(function):
				prunedCommand, _, err := runner.pruneEntityCommand(ctx, command)
				if err == nil {
					prunedGroup.Commands[index] = prunedCommand
				}
			}
		}
		result = append(result, prunedGroup)
	}
	return result
}

func (a *App) printLoadedSource(backupPath string, data SourceData) {
	if backupPath != "" {
		a.ui.Println("DB backup: " + backupPath)
	}
	a.ui.Println("DB: " + a.cfg.DBDisplay)
	a.ui.Println("DB format: " + data.Format)
	if data.PrivateKeyUsed && a.cfg.ISPKey != "" {
		a.ui.Println("privkey: " + a.cfg.ISPKey)
	} else if strings.TrimSpace(data.KeyStatusMessage) != "" {
		a.ui.Println("privkey: " + colorYellow + data.KeyStatusMessage + colorReset)
		if strings.TrimSpace(data.KeyStatusReason) != "" {
			a.ui.Println(data.KeyStatusReason)
		}
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

func countExportedItems(scopes []string, sections []Section, commands []string) int {
	total := 0
	for _, section := range sections {
		total += len(section.Rows)
	}
	if hasScope(scopes, "commands") || hasScope(scopes, "all") {
		total += len(commands)
	}
	return total
}
