package app

import (
	"fmt"
	"strings"
)

func renderTable(headers []string, rows [][]string, totalIndex int) string {
	if len(headers) == 0 {
		return ""
	}
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len([]rune(header))
	}
	for _, row := range rows {
		for i, value := range row {
			if i < len(widths) {
				if length := len([]rune(value)); length > widths[i] {
					widths[i] = length
				}
			}
		}
	}

	var builder strings.Builder
	builder.WriteString(border(widths))
	builder.WriteByte('\n')
	builder.WriteString(tableRow(headers, widths))
	builder.WriteByte('\n')
	builder.WriteString(border(widths))
	for i, row := range rows {
		if totalIndex >= 0 && i == totalIndex {
			builder.WriteByte('\n')
			builder.WriteString(border(widths))
		}
		builder.WriteByte('\n')
		builder.WriteString(tableRow(row, widths))
	}
	builder.WriteByte('\n')
	builder.WriteString(border(widths))
	return builder.String()
}

func border(widths []int) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("-", width+2))
	}
	return "+" + strings.Join(parts, "+") + "+"
}

func tableRow(values []string, widths []int) string {
	cells := make([]string, 0, len(widths))
	for i, width := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		cells = append(cells, " "+padRight(value, width)+" ")
	}
	return "|" + strings.Join(cells, "|") + "|"
}

func padRight(value string, width int) string {
	length := len([]rune(value))
	if length >= width {
		return value
	}
	return value + strings.Repeat(" ", width-length)
}

func renderSections(sections []Section, colorize bool) string {
	var builder strings.Builder
	for i, section := range sections {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(formatTitle(section.Title+":", colorize))
		if section.Subtitle != "" {
			builder.WriteByte('\n')
			builder.WriteString(section.Subtitle)
		}
		builder.WriteByte('\n')
		if len(section.Rows) == 0 {
			builder.WriteString(section.EmptyMessage)
			continue
		}
		rows, totalIndex := rowsWithTotal(section.Headers, section.Rows)
		builder.WriteString(renderTable(section.Headers, rows, totalIndex))
	}
	return builder.String()
}

func renderCleanSections(sections []Section) string {
	nonEmpty := make([]Section, 0, len(sections))
	for _, section := range sections {
		if len(section.Rows) > 0 {
			nonEmpty = append(nonEmpty, section)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, section := range nonEmpty {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		if len(nonEmpty) > 1 {
			builder.WriteString(section.Title)
			builder.WriteByte(':')
			builder.WriteByte('\n')
		}
		for rowIndex, row := range section.Rows {
			if rowIndex > 0 {
				builder.WriteByte('\n')
			}
			if len(row) > 0 {
				builder.WriteString(row[0])
			}
		}
	}
	return builder.String()
}

func canUseCleanOutput(sections []Section) bool {
	if len(sections) == 0 {
		return false
	}
	for _, section := range sections {
		if len(section.Headers) != 1 {
			return false
		}
	}
	return true
}

func renderTextSections(sections []Section, showTitles bool, includeTotals bool) string {
	var builder strings.Builder
	for i, section := range sections {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		if showTitles {
			builder.WriteString(section.Title + ":")
			if section.Subtitle != "" {
				builder.WriteByte('\n')
				builder.WriteString(section.Subtitle)
			}
			builder.WriteByte('\n')
		} else if section.Subtitle != "" {
			builder.WriteString(section.Subtitle)
			builder.WriteByte('\n')
		}
		if len(section.Rows) == 0 {
			builder.WriteString(section.EmptyMessage)
			continue
		}
		rows, totalIndex := rowsWithOptionalTotal(section.Headers, section.Rows, includeTotals)
		textRows := make([][]string, 0, len(rows)+1)
		textRows = append(textRows, section.Headers)
		for i, row := range rows {
			if totalIndex >= 0 && i == totalIndex {
				textRows = append(textRows, nil)
			}
			textRows = append(textRows, row)
		}
		builder.WriteString(renderPlainRows(textRows))
	}
	return builder.String()
}

func renderPlainRows(rows [][]string) string {
	widths := make([]int, 0)
	for _, row := range rows {
		if row == nil {
			continue
		}
		if len(row) > len(widths) {
			widths = append(widths, make([]int, len(row)-len(widths))...)
		}
		for i, value := range row {
			if length := len([]rune(value)); length > widths[i] {
				widths[i] = length
			}
		}
	}

	var builder strings.Builder
	for index, row := range rows {
		if row == nil {
			builder.WriteByte('\n')
			continue
		}
		if index > 0 {
			builder.WriteByte('\n')
		}
		for i, value := range row {
			if i > 0 {
				builder.WriteString("  ")
			}
			builder.WriteString(padRight(value, widths[i]))
		}
	}
	return builder.String()
}

func renderPlainRowsCompact(rows [][]string) string {
	var builder strings.Builder
	first := true
	for _, row := range rows {
		if row == nil {
			continue
		}
		if !first {
			builder.WriteByte('\n')
		}
		first = false
		builder.WriteString(strings.Join(row, "  "))
	}
	return builder.String()
}

func commandSectionText(groups []CommandGroup, colorize bool, withHeader bool) string {
	return commandSectionTextWithHeader(groups, colorize, withHeader, "commands to run at remote server:")
}

func commandSectionTextWithHeader(groups []CommandGroup, colorize bool, withHeader bool, header string) string {
	return commandSectionTextWithOptions(groups, colorize, withHeader, header, false)
}

func commandSectionTextWithOptions(groups []CommandGroup, colorize bool, withHeader bool, header string, annotateDeletes bool) string {
	if len(groups) == 0 {
		if withHeader {
			return renderCommandHeader(header, colorize, false) + "\n\nNo commands could be generated."
		}
		return "No commands could be generated."
	}

	var builder strings.Builder
	hasDeleteCommands := annotateDeletes && groupsHaveDeleteCommands(groups)
	if withHeader {
		builder.WriteString(renderCommandHeader(header, colorize, hasDeleteCommands))
		builder.WriteString("\n\n")
	}
	filteredGroups, _ := splitCommandGroupsForRender(groups)
	for i, group := range filteredGroups {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(renderCommandGroupTitle(group, colorize, annotateDeletes))
		builder.WriteByte('\n')
		builder.WriteString(renderCommandList(group.Commands, colorize))
	}
	return builder.String()
}

func noSyncCommandsText() string {
	return "No differences were found. Nothing to sync."
}

func renderCommandHeader(header string, colorize bool, hasDeleteCommands bool) string {
	if !isSyncCommandHeader(header) {
		return formatTitle(header, colorize)
	}

	title := syncCommandTitle(header)
	width := len([]rune(title))
	if width < 48 {
		width = 48
	}
	separator := "# " + strings.Repeat("=", width)
	center := "# " + title

	lines := []string{separator, center}
	if hasDeleteCommands {
		lines = append(lines, "# WARNING: SOME COMMANDS ARE DELETE / UNINSTALL")
	}
	lines = append(lines, separator)
	if !colorize {
		return strings.Join(lines, "\n")
	}

	for i, line := range lines {
		if strings.Contains(line, "WARNING: SOME COMMANDS ARE DELETE / UNINSTALL") {
			lines[i] = formatTitle("#", true) + colorRed + " WARNING: SOME COMMANDS ARE DELETE / UNINSTALL" + colorReset
			continue
		}
		lines[i] = formatTitle(line, true)
	}
	return strings.Join(lines, "\n")
}

func isSyncCommandHeader(header string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "sync ")
}

func syncCommandTitle(header string) string {
	switch strings.ToLower(strings.TrimSuffix(strings.TrimSpace(header), ":")) {
	case "sync remote with local":
		return "TO SYNC REMOTE WITH LOCAL (RUN IT REMOTELY)"
	case "sync local with remote":
		return "TO SYNC LOCAL WITH REMOTE  (RUN IT LOCALLY)"
	default:
		return strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(header), ":"))
	}
}

func renderCommandGroupTitle(group CommandGroup, colorize bool, annotateDeletes bool) string {
	title := group.Title + ":"
	if !colorize {
		isPackageGroup := strings.HasPrefix(group.Title, "packages (")
		if isPackageGroup {
			if annotateDeletes && groupHasDeleteCommands(group) && strings.HasSuffix(group.Title, ")") {
				return "# " + strings.TrimSuffix(group.Title, ")") + ", some delete / remove commands exists)"
			}
			return "# " + group.Title
		}
		return "# " + title
	}

	isPackageGroup := strings.HasPrefix(group.Title, "packages (")
	if isPackageGroup {
		if annotateDeletes && groupHasDeleteCommands(group) && strings.HasSuffix(group.Title, ")") {
			base := "# " + strings.TrimSuffix(group.Title, ")")
			return formatTitle(base, true) +
				formatTitle(",", true) +
				colorRed + " some delete / remove commands exists" + colorReset +
				formatTitle(")", true)
		}
		return formatTitle("# "+group.Title, true)
	}
	return formatTitle("# "+title, true)
}

func groupsHaveDeleteCommands(groups []CommandGroup) bool {
	for _, group := range groups {
		if groupHasDeleteCommands(group) {
			return true
		}
	}
	return false
}

func groupHasDeleteCommands(group CommandGroup) bool {
	for _, command := range group.Commands {
		if commandHasDeleteAction(command) {
			return true
		}
	}
	return false
}

func commandHasDeleteAction(command string) bool {
	fields, err := splitShellWords(command)
	if err != nil {
		fields = strings.Fields(command)
	}
	for _, field := range fields {
		normalized := strings.ToLower(strings.Trim(field, "\"'"))
		if normalized == "upgradesystem=off" {
			continue
		}
		if strings.HasSuffix(normalized, "=off") || strings.HasSuffix(normalized, "=turn_off") {
			return true
		}
	}
	return false
}

func splitCommandGroupsForRender(groups []CommandGroup) ([]CommandGroup, bool) {
	filtered := make([]CommandGroup, 0, len(groups))
	hasPackageGroup := false

	for _, group := range groups {
		if strings.HasPrefix(group.Title, "packages (") {
			hasPackageGroup = true
		}
		filtered = append(filtered, group)
	}

	return filtered, hasPackageGroup
}

func renderCommandList(commands []string, console bool) string {
	if len(commands) == 0 {
		return ""
	}
	if !console {
		return strings.Join(commands, "\n")
	}

	var builder strings.Builder
	for i, command := range commands {
		if i > 0 {
			if isLongConsoleCommand(commands[i-1]) || isLongConsoleCommand(command) {
				builder.WriteString("\n\n")
			} else {
				builder.WriteByte('\n')
			}
		}
		builder.WriteString(command)
	}
	return builder.String()
}

func isLongConsoleCommand(command string) bool {
	fields, err := splitShellWords(command)
	if err != nil {
		return len(strings.Fields(command)) > 6
	}
	if len(fields) >= 4 && fields[0] == "/usr/local/mgr5/sbin/mgrctl" && fields[1] == "-m" && fields[2] == "ispmgr" {
		return len(fields[4:]) > 5
	}
	return len(fields) > 6
}

func formatTitle(value string, colorize bool) string {
	if !colorize {
		return value
	}
	return colorGreen + value + colorReset
}

func rowsWithTotal(headers []string, rows [][]string) ([][]string, int) {
	return rowsWithOptionalTotal(headers, rows, true)
}

func rowsWithOptionalTotal(headers []string, rows [][]string, includeTotal bool) ([][]string, int) {
	result := make([][]string, 0, len(rows)+1)
	result = append(result, rows...)
	if !includeTotal || len(headers) == 0 || len(rows) == 0 {
		return result, -1
	}
	total := make([]string, len(headers))
	if len(headers) == 1 {
		total[0] = fmt.Sprintf("Total: %d", len(rows))
	} else {
		total[0] = "Total"
		total[1] = fmt.Sprintf("%d", len(rows))
	}
	return append(result, total), len(rows)
}

func filterSectionsByColumns(sections []Section, columns []string) ([]Section, error) {
	if len(columns) == 0 {
		return sections, nil
	}

	filtered := make([]Section, 0, len(sections))
	allSupported := map[string]struct{}{}
	for _, section := range sections {
		indexes := make([]int, 0, len(columns))
		headers := make([]string, 0, len(columns))
		for _, header := range section.Headers {
			allSupported[strings.ToLower(header)] = struct{}{}
		}
		for _, requested := range columns {
			for idx, header := range section.Headers {
				if strings.EqualFold(header, requested) {
					indexes = append(indexes, idx)
					headers = append(headers, header)
					break
				}
			}
		}
		if len(indexes) == 0 {
			continue
		}
		rows := make([][]string, 0, len(section.Rows))
		for _, row := range section.Rows {
			item := make([]string, 0, len(indexes))
			for _, idx := range indexes {
				if idx < len(row) {
					item = append(item, row[idx])
				} else {
					item = append(item, "")
				}
			}
			rows = append(rows, item)
		}
		filtered = append(filtered, Section{
			Title:        section.Title,
			Subtitle:     section.Subtitle,
			Headers:      headers,
			Rows:         rows,
			EmptyMessage: section.EmptyMessage,
		})
	}

	if len(filtered) == 0 {
		supported := make([]string, 0, len(allSupported))
		for header := range allSupported {
			supported = append(supported, header)
		}
		return nil, fmt.Errorf("none of the requested columns are supported. Supported columns: %s", strings.Join(sortedStrings(supported), ", "))
	}

	return filtered, nil
}

func sortedStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	sorted := append([]string(nil), values...)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}
