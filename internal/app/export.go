package app

import (
	"encoding/csv"
	"encoding/json"
	"strings"
)

func exportSections(sections []Section, mode string) []Section {
	if mode == "all" {
		return sections
	}
	if mode == "data" {
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
			if section.Title == "users" || section.Title == "FTP users" {
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

func renderCSV(sections []Section, delimiter rune) string {
	var builder strings.Builder
	for index, section := range sections {
		if index > 0 {
			builder.WriteString("\n")
		}
		writer := csv.NewWriter(&builder)
		writer.Comma = delimiter
		_ = writer.Write([]string{"section", section.Title})
		_ = writer.Write(section.Headers)
		rows, _ := rowsWithTotal(section.Headers, section.Rows)
		for _, row := range rows {
			_ = writer.Write(row)
		}
		writer.Flush()
	}
	return builder.String()
}

func renderJSONExport(source string, sections []Section) ([]byte, error) {
	type item struct {
		Title   string     `json:"title"`
		Headers []string   `json:"headers"`
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
		rows, _ := rowsWithTotal(section.Headers, section.Rows)
		payload.Sections = append(payload.Sections, item{
			Title:   section.Title,
			Headers: section.Headers,
			Rows:    rows,
		})
	}
	return json.MarshalIndent(payload, "", "  ")
}
