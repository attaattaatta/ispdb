package app

import (
	"encoding/csv"
	"encoding/json"
	"strings"
)

func exportSections(sections []Section, mode string) []Section {
	return exportSectionsForScopes(sections, []string{mode})
}

func configuredExportScopeList(exportScope string, listMode string) []string {
	if scopes := configuredScopeList(exportScope, exportScopes); len(scopes) > 0 {
		return scopes
	}

	listScopes := configuredScopeList(listMode, listModes)
	if len(listScopes) == 0 || hasScope(listScopes, "all") {
		return []string{"data"}
	}
	return append([]string{}, listScopes...)
}

func exportSectionsForScopes(sections []Section, scopes []string) []Section {
	if len(scopes) == 0 {
		return nil
	}
	if hasScope(scopes, "all") || hasScope(scopes, "data") {
		return sections
	}

	filtered := make([]Section, 0, len(sections))
	seen := make(map[string]struct{}, len(sections))
	for _, scope := range scopes {
		for _, section := range exportSectionsForScope(sections, scope) {
			if _, ok := seen[section.Title]; ok {
				continue
			}
			seen[section.Title] = struct{}{}
			filtered = append(filtered, section)
		}
	}
	return filtered
}

func exportSectionsForScope(sections []Section, mode string) []Section {
	if mode == "all" || mode == "data" {
		return sections
	}

	filtered := make([]Section, 0, len(sections))
	for _, section := range sections {
		switch mode {
		case "packages":
			if section.Title == "packages" {
				filtered = append(filtered, section)
			}
		case "users":
			if section.Title == "users" || section.Title == "ftp users" || section.Title == "db users" {
				filtered = append(filtered, section)
			}
		case "webdomains":
			if section.Title == "web domains" {
				filtered = append(filtered, section)
			}
		case "databases":
			if section.Title == "database servers" || section.Title == "databases" || section.Title == "db users" {
				filtered = append(filtered, section)
			}
		case "email":
			if section.Title == "email domains" || section.Title == "email boxes" {
				filtered = append(filtered, section)
			}
		case "dns":
			if section.Title == "dns" {
				filtered = append(filtered, section)
			}
		}
	}
	return filtered
}

func renderOrderedExportText(sections []Section, commandGroups []CommandGroup, commands []string, scopes []string, clean bool, noHeaders bool) string {
	if len(scopes) == 0 {
		scopes = []string{"data"}
	}

	parts := make([]string, 0, len(scopes))
	dataRendered := false
	commandsRendered := false

	appendData := func(scopeSections []Section, showTitles bool) {
		if len(scopeSections) == 0 {
			return
		}
		if noHeaders {
			parts = append(parts, renderTextSectionsNoColumnHeaders(scopeSections, showTitles))
			return
		}
		if clean && canUseCleanOutput(scopeSections) {
			parts = append(parts, renderCleanSections(scopeSections))
			return
		}
		parts = append(parts, renderTextSections(scopeSections, showTitles, false))
	}

	for _, scope := range scopes {
		switch scope {
		case "all":
			if !dataRendered {
				appendData(sections, true)
				dataRendered = true
			}
			if !commandsRendered && len(commands) > 0 {
				if noHeaders {
					parts = append(parts, renderCommandSectionNoBlankLines(commandGroups, commands))
				} else {
					parts = append(parts, commandSectionText(commandGroups, false, false))
				}
				commandsRendered = true
			}
		case "data":
			if dataRendered {
				continue
			}
			appendData(sections, true)
			dataRendered = true
		case "commands":
			if commandsRendered || len(commands) == 0 {
				continue
			}
			if noHeaders {
				parts = append(parts, renderCommandSectionNoBlankLines(commandGroups, commands))
			} else {
				parts = append(parts, commandSectionText(commandGroups, false, false))
			}
			commandsRendered = true
		default:
			if dataRendered {
				continue
			}
			scopeSections := exportSectionsForScope(sections, scope)
			showTitles := len(scopes) > 1 || len(scopeSections) > 1
			appendData(scopeSections, showTitles)
		}
	}

	if noHeaders {
		return strings.Join(parts, "\n")
	}
	return strings.Join(parts, "\n\n")
}

func renderCSV(sections []Section, delimiter rune, noHeaders bool) string {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	writer.Comma = delimiter
	for index, section := range sections {
		if index > 0 && !noHeaders {
			builder.WriteString("\n")
		}
		if !noHeaders {
			_ = writer.Write([]string{"section", section.Title})
			_ = writer.Write(section.Headers)
		}
		for _, row := range section.Rows {
			_ = writer.Write(row)
		}
	}
	writer.Flush()
	return builder.String()
}

func renderJSONExport(source string, sections []Section, noHeaders bool) ([]byte, error) {
	type item struct {
		Title   string     `json:"title,omitempty"`
		Headers []string   `json:"headers,omitempty"`
		Rows    [][]string `json:"rows"`
	}
	payload := struct {
		Source   string `json:"source"`
		Sections []item `json:"sections"`
	}{
		Source:   source,
		Sections: make([]item, 0, len(sections)),
	}
	for _, section := range sections {
		entry := item{Rows: section.Rows}
		if !noHeaders {
			entry.Title = section.Title
			entry.Headers = section.Headers
		}
		payload.Sections = append(payload.Sections, entry)
	}
	return json.MarshalIndent(payload, "", "  ")
}

func renderTextSectionsNoColumnHeaders(sections []Section, showTitles bool) string {
	var builder strings.Builder
	firstSection := true
	for _, section := range sections {
		if len(section.Rows) == 0 {
			continue
		}
		if !firstSection {
			builder.WriteByte('\n')
		}
		firstSection = false
		if showTitles {
			builder.WriteString(section.Title)
			builder.WriteByte(':')
			builder.WriteByte('\n')
		}
		builder.WriteString(renderPlainRowsCompact(section.Rows))
	}
	return builder.String()
}

func renderCommandSectionNoBlankLines(groups []CommandGroup, commands []string) string {
	if len(groups) == 0 {
		return strings.Join(commands, "\n")
	}

	var builder strings.Builder
	filteredGroups, _ := splitCommandGroupsForRender(groups)
	for index, group := range filteredGroups {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(renderCommandGroupTitle(group, false, false))
		builder.WriteByte('\n')
		builder.WriteString(strings.Join(group.Commands, "\n"))
	}
	return builder.String()
}
