package app

import (
	"regexp"
	"strings"
	"testing"
)

func TestRenderCleanSectionsSingleSection(t *testing.T) {
	t.Parallel()

	sections := []Section{
		{
			Title:   "packages",
			Headers: []string{"name"},
			Rows: [][]string{
				{"nginx"},
				{"php"},
			},
		},
	}

	got := renderCleanSections(sections)
	want := "nginx\nphp"
	if got != want {
		t.Fatalf("renderCleanSections() = %q, want %q", got, want)
	}
}

func TestCommandSectionTextPrefixesAllGroupTitlesInConsole(t *testing.T) {
	t.Parallel()

	got := commandSectionText([]CommandGroup{
		{
			Title:    "packages (web)",
			Commands: []string{"cmd1"},
		},
		{
			Title:    "email",
			Commands: []string{"cmd2"},
		},
	}, true, false)

	if !containsAll(got, []string{"# packages (web)", "# email:"}) {
		t.Fatalf("commandSectionText() did not render expected group titles:\n%s", got)
	}
	if strings.Contains(got, "# packages (web):") {
		t.Fatalf("package group title must not end with colon in console output:\n%s", got)
	}
	if strings.Contains(got, "\nemail:\n") {
		t.Fatalf("non-package group titles must also be commented in console output:\n%s", got)
	}
}

func TestCommandSectionTextUsesRemoteHeader(t *testing.T) {
	t.Parallel()

	got := commandSectionText([]CommandGroup{
		{Title: "users", Commands: []string{"cmd"}},
	}, true, true)
	got = stripANSI(got)

	if !strings.Contains(got, "commands to run at remote server:") {
		t.Fatalf("expected remote-server header, got:\n%s", got)
	}
	if !strings.Contains(got, "\n\n# users:\n") {
		t.Fatalf("expected blank line after header, got:\n%s", got)
	}
}

func stripANSI(value string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(value, "")
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
