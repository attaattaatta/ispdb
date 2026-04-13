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

func commandSectionText(groups []CommandGroup, colorize bool, withHeader bool) string {
	if len(groups) == 0 {
		if withHeader {
			return formatTitle("commands to run:", colorize) + "\nNo commands could be generated."
		}
		return "No commands could be generated."
	}

	var builder strings.Builder
	if withHeader {
		builder.WriteString(formatTitle("commands to run:", colorize))
		builder.WriteByte('\n')
	}
	for i, group := range groups {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		if colorize {
			builder.WriteString(formatTitle(group.Title+":", true))
		} else {
			builder.WriteString(group.Title + ":")
		}
		builder.WriteByte('\n')
		builder.WriteString(strings.Join(group.Commands, "\n"))
	}
	return builder.String()
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
