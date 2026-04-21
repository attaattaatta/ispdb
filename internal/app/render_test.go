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
		{Title: "packages (web)", Commands: []string{"pkg-cmd"}},
		{Title: "users", Commands: []string{"cmd"}},
	}, true, true)
	got = stripANSI(got)

	if !strings.Contains(got, "commands to run at remote server:") {
		t.Fatalf("expected remote-server header, got:\n%s", got)
	}
	if !strings.Contains(got, featureUpdateCommand()) {
		t.Fatalf("expected shared feature.update before package commands, got:\n%s", got)
	}
	if !strings.Contains(got, "\n\n# users:\n") {
		t.Fatalf("expected blank line after header, got:\n%s", got)
	}
}

func TestCommandSectionTextCanUseLocalSyncHeader(t *testing.T) {
	t.Parallel()

	got := commandSectionTextWithHeader([]CommandGroup{
		{Title: "packages (web)", Commands: []string{"pkg-cmd"}},
		{Title: "users", Commands: []string{"cmd"}},
	}, true, true, "sync local with remote:")
	got = stripANSI(got)

	if !strings.Contains(got, "# TO SYNC LOCAL WITH REMOTE  (RUN IT LOCALLY)") {
		t.Fatalf("expected local-sync header, got:\n%s", got)
	}
	if strings.Count(got, featureUpdateCommand()) != 1 {
		t.Fatalf("expected exactly one shared feature.update, got:\n%s", got)
	}
	if !strings.Contains(got, "\n\n# users:\n") {
		t.Fatalf("expected blank line after header, got:\n%s", got)
	}
}

func TestRenderCommandHeaderUsesCommentBannerForSyncSections(t *testing.T) {
	t.Parallel()

	got := stripANSI(renderCommandHeader("sync remote with local:", true, false))

	if !strings.Contains(got, "# TO SYNC REMOTE WITH LOCAL (RUN IT REMOTELY)") {
		t.Fatalf("expected emphasized sync banner, got:\n%s", got)
	}
	if strings.Count(got, "# ") != 3 {
		t.Fatalf("expected three comment banner lines, got:\n%s", got)
	}
}

func TestRenderCommandHeaderAddsDeleteWarningForSyncSections(t *testing.T) {
	t.Parallel()

	got := stripANSI(renderCommandHeader("sync remote with local:", true, true))

	if !strings.Contains(got, "# WARNING: SOME COMMANDS ARE DELETE / UNINSTALL") {
		t.Fatalf("expected delete warning in sync banner, got:\n%s", got)
	}
}

func TestRenderCommandHeaderKeepsWarningHashGreen(t *testing.T) {
	t.Parallel()

	got := renderCommandHeader("sync remote with local:", true, true)
	want := colorGreen + "#" + colorReset + colorRed + " WARNING: SOME COMMANDS ARE DELETE / UNINSTALL" + colorReset
	if !strings.Contains(got, want) {
		t.Fatalf("expected green hash and red warning text, got:\n%q", got)
	}
}

func TestCommandSectionTextAnnotatesPackageDeleteCommandsInListMode(t *testing.T) {
	t.Parallel()

	got := stripANSI(commandSectionTextWithOptions([]CommandGroup{
		{
			Title: "packages (web)",
			Commands: []string{
				"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=web package_nginx=off packagegroup_apache=turn_off",
			},
		},
	}, true, true, "sync remote with local:", true))

	if !strings.Contains(got, "# WARNING: SOME COMMANDS ARE DELETE / UNINSTALL") {
		t.Fatalf("expected top-level delete warning, got:\n%s", got)
	}
	if !strings.Contains(got, "# packages (web, some delete / remove commands exists)") {
		t.Fatalf("expected package delete note, got:\n%s", got)
	}
}

func TestCommandSectionTextKeepsPackageCommaAndParenGreenForDeleteNotes(t *testing.T) {
	t.Parallel()

	got := commandSectionTextWithOptions([]CommandGroup{
		{
			Title: "packages (web)",
			Commands: []string{
				"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=web package_nginx=off",
			},
		},
	}, true, true, "sync remote with local:", true)

	want := formatTitle("# packages (web", true) +
		formatTitle(",", true) +
		colorRed + " some delete / remove commands exists" + colorReset +
		formatTitle(")", true)
	if !strings.Contains(got, want) {
		t.Fatalf("expected green comma/paren around red delete note, got:\n%q", got)
	}
}

func TestCommandHasDeleteActionIgnoresFeatureUpdateOffFlag(t *testing.T) {
	t.Parallel()

	if commandHasDeleteAction(featureUpdateCommand()) {
		t.Fatalf("did not expect feature.update to be marked as delete action")
	}
	if !commandHasDeleteAction("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=web package_nginx=off") {
		t.Fatalf("expected off-valued package command to be marked as delete action")
	}
}

func TestCommandSectionTextSeparatesLongConsoleCommands(t *testing.T) {
	t.Parallel()

	got := stripANSI(commandSectionText([]CommandGroup{
		{
			Title: "users",
			Commands: []string{
				"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit sok=ok backup=on confirm=test name=alice passwd=test php_enable=on preset=#custom",
				"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit sok=ok backup=on confirm=test name=bob passwd=test php_enable=on preset=#custom",
			},
		},
	}, true, false))

	if !strings.Contains(got, "preset=#custom\n\n/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit") {
		t.Fatalf("expected long console commands to be separated by a blank line, got:\n%s", got)
	}
}

func TestCommandSectionTextKeepsExportCommandsCompact(t *testing.T) {
	t.Parallel()

	got := commandSectionText([]CommandGroup{
		{
			Title: "users",
			Commands: []string{
				"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit sok=ok backup=on confirm=test name=alice passwd=test php_enable=on preset=#custom",
				"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit sok=ok backup=on confirm=test name=bob passwd=test php_enable=on preset=#custom",
			},
		},
	}, false, false)

	if strings.Contains(got, "\n\n/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit") {
		t.Fatalf("did not expect export command output to insert blank lines between commands:\n%s", got)
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
