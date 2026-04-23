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

func TestConfiguredExportScopeListMirrorsListScopeWhenExportScopeMissing(t *testing.T) {
	t.Parallel()

	scopes := configuredExportScopeList("", "users,commands,dns")
	got := strings.Join(scopes, ",")
	if got != "users,commands,dns" {
		t.Fatalf("expected export scopes to mirror list scope, got %q", got)
	}
}

func TestRenderOrderedExportTextPreservesRequestedScopeOrder(t *testing.T) {
	t.Parallel()

	sections := []Section{
		{Title: "users", Headers: []string{"name"}, Rows: [][]string{{"alice"}}},
		{Title: "ftp users", Headers: []string{"name"}, Rows: [][]string{{"ftp-alice"}}},
		{Title: "db users", Headers: []string{"name"}, Rows: [][]string{{"db-alice"}}},
		{Title: "dns", Headers: []string{"name"}, Rows: [][]string{{"example.com"}}},
	}
	commandGroups := []CommandGroup{
		{Title: "packages (web)", Commands: []string{"cmd-web"}},
	}
	commands := []string{"cmd-web"}

	got := renderOrderedExportText(sections, commandGroups, commands, []string{"users", "commands", "dns"}, false, false)

	usersIndex := strings.Index(got, "users:")
	commandsIndex := strings.Index(got, "# packages (web)")
	dnsIndex := strings.Index(got, "dns:")
	if usersIndex == -1 || commandsIndex == -1 || dnsIndex == -1 {
		t.Fatalf("expected users, commands, and dns blocks in ordered export, got:\n%s", got)
	}
	if !(usersIndex < commandsIndex && commandsIndex < dnsIndex) {
		t.Fatalf("expected users -> commands -> dns order, got:\n%s", got)
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
	}, ',', false)

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
	}, false)
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

func TestRenderCSVNoHeadersOmitsSectionAndHeaderRows(t *testing.T) {
	t.Parallel()

	output := renderCSV([]Section{
		{
			Title:   "dns",
			Headers: []string{"name", "type"},
			Rows: [][]string{
				{"example.com", "master"},
			},
		},
	}, ';', true)

	if strings.Contains(output, "section") || strings.Contains(output, "dns") || strings.Contains(output, "name;type") {
		t.Fatalf("CSV export with no headers must omit section/header rows, got:\n%s", output)
	}
	if !strings.Contains(output, "example.com;master") {
		t.Fatalf("CSV export with no headers lost data row, got:\n%s", output)
	}
}

func TestRenderJSONExportNoHeadersOmitsTitlesAndHeaders(t *testing.T) {
	t.Parallel()

	output, err := renderJSONExport("db", []Section{
		{
			Title:   "dns",
			Headers: []string{"name"},
			Rows:    [][]string{{"example.com"}},
		},
	}, true)
	if err != nil {
		t.Fatalf("renderJSONExport returned error: %v", err)
	}
	text := string(output)
	if strings.Contains(text, "\"title\"") || strings.Contains(text, "\"headers\"") {
		t.Fatalf("JSON export with no headers must omit title/header fields, got:\n%s", text)
	}
	if !strings.Contains(text, "\"rows\"") || !strings.Contains(text, "\"example.com\"") {
		t.Fatalf("JSON export with no headers lost rows, got:\n%s", text)
	}
}

func TestRenderOrderedExportTextNoHeadersKeepsTitlesWithoutBlankLines(t *testing.T) {
	t.Parallel()

	sections := []Section{
		{Title: "users", Headers: []string{"name", "home"}, Rows: [][]string{{"alice", "/home/alice"}}},
		{Title: "dns", Headers: []string{"name", "type"}, Rows: [][]string{{"example.com", "master"}}},
	}
	commandGroups := []CommandGroup{
		{Title: "packages (web)", Commands: []string{"cmd-web"}},
	}
	commands := []string{"cmd-web"}

	got := renderOrderedExportText(sections, commandGroups, commands, []string{"users", "commands", "dns"}, false, true)
	if strings.Contains(got, "\n\n") {
		t.Fatalf("no-headers export must omit blank lines, got:\n%s", got)
	}
	if !strings.Contains(got, "users:\nalice  /home/alice\n# packages (web)\ncmd-web\ndns:\nexample.com  master") {
		t.Fatalf("unexpected no-headers export ordering/content:\n%s", got)
	}
}
