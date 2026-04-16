package app

import (
	"strings"
	"testing"
)

func TestExportSectionsUsersIncludesDBUsers(t *testing.T) {
	t.Parallel()

	sections := []Section{
		{Title: "users"},
		{Title: "ftp users"},
		{Title: "db users"},
		{Title: "dns"},
	}

	filtered := exportSections(sections, "users")
	if len(filtered) != 3 {
		t.Fatalf("expected 3 user-related sections, got %d", len(filtered))
	}
	if filtered[2].Title != "db users" {
		t.Fatalf("expected db users to be included, got %#v", filtered)
	}
}

func TestRenderCSVDoesNotIncludeTotal(t *testing.T) {
	t.Parallel()

	output := renderCSV([]Section{
		{
			Title:   "dns",
			Headers: []string{"id", "name"},
			Rows: [][]string{
				{"1", "example.com"},
				{"2", "example.net"},
			},
		},
	}, ',')

	if strings.Contains(output, "Total") {
		t.Fatalf("CSV export must not include Total row, got:\n%s", output)
	}
}

func TestRenderJSONDoesNotIncludeTotal(t *testing.T) {
	t.Parallel()

	output, err := renderJSONExport("db", []Section{
		{
			Title:   "dns",
			Headers: []string{"id", "name"},
			Rows: [][]string{
				{"1", "example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("renderJSONExport returned error: %v", err)
	}
	text := string(output)
	if strings.Contains(text, "Total") {
		t.Fatalf("JSON export must not include Total row, got:\n%s", text)
	}
	if !strings.Contains(text, "\"example.com\"") {
		t.Fatalf("JSON export lost the data row, got:\n%s", text)
	}
}
