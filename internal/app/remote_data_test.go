package app

import (
	"strings"
	"testing"
)

func TestRenderOrderedRemoteListOutputKeepsScopeOrderWithCommands(t *testing.T) {
	t.Parallel()

	data := SourceData{
		Databases: []Database{
			{ID: "1", Name: "db1", Owner: "alice", Server: "mysql"},
		},
	}
	commandSections := remoteListCommandSections{
		localWithRemote: []CommandGroup{
			{
				Title:    "databases",
				Commands: []string{"/usr/local/mgr5/sbin/mgrctl -m ispmgr db.edit name=db1 sok=ok"},
			},
		},
	}

	output, err := renderOrderedRemoteListOutput(data, Config{
		ListMode: "databases,commands",
	}, commandSections)
	if err != nil {
		t.Fatalf("renderOrderedRemoteListOutput returned error: %v", err)
	}

	dbIndex := strings.Index(output, "databases:")
	cmdIndex := strings.Index(output, "# TO SYNC LOCAL WITH REMOTE  (RUN IT LOCALLY)")
	if dbIndex == -1 || cmdIndex == -1 {
		t.Fatalf("expected both databases and commands in output, got:\n%s", output)
	}
	if dbIndex > cmdIndex {
		t.Fatalf("expected databases section before commands section, got:\n%s", output)
	}
}

func TestRenderOrderedRemoteListOutputShowsBothSyncDirections(t *testing.T) {
	t.Parallel()

	output, err := renderOrderedRemoteListOutput(SourceData{}, Config{ListMode: "commands"}, remoteListCommandSections{
		localWithRemote: []CommandGroup{{Title: "users", Commands: []string{"cmd-local"}}},
		remoteWithLocal: []CommandGroup{{Title: "users", Commands: []string{"cmd-remote"}}},
	})
	if err != nil {
		t.Fatalf("renderOrderedRemoteListOutput returned error: %v", err)
	}

	for _, want := range []string{"sync local with remote:", "sync remote with local:", "cmd-local", "cmd-remote"} {
		if strings.HasPrefix(want, "sync ") {
			switch want {
			case "sync local with remote:":
				want = "# TO SYNC LOCAL WITH REMOTE  (RUN IT LOCALLY)"
			case "sync remote with local:":
				want = "# TO SYNC REMOTE WITH LOCAL (RUN IT REMOTELY)"
			}
		}
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestRenderOrderedRemoteListOutputShowsNoDifferencesMessageWhenBothDirectionsAreEmpty(t *testing.T) {
	t.Parallel()

	output, err := renderOrderedRemoteListOutput(SourceData{}, Config{ListMode: "commands"}, remoteListCommandSections{})
	if err != nil {
		t.Fatalf("renderOrderedRemoteListOutput returned error: %v", err)
	}

	if !strings.Contains(output, "No differences were found. Nothing to sync.") {
		t.Fatalf("expected no-differences message, got:\n%s", output)
	}
}

func TestSQLiteSidecarPathsIncludeWALAndSHM(t *testing.T) {
	t.Parallel()

	got := sqliteSidecarPaths("/usr/local/mgr5/etc/ispmgr.db")
	want := []string{
		"/usr/local/mgr5/etc/ispmgr.db-wal",
		"/usr/local/mgr5/etc/ispmgr.db-shm",
		"/usr/local/mgr5/etc/ispmgr.db-journal",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d sidecar paths, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected sidecar path %d to be %q, got %#v", i, want[i], got)
		}
	}
}

func TestFilterSourceDataByMissingInventorySkipsEquivalentPackagesAndEntities(t *testing.T) {
	t.Parallel()

	source := SourceData{
		Packages: []Package{
			{ID: "1", Name: "fail2ban"},
			{ID: "2", Name: "apache-itk-ubuntu"},
			{ID: "3", Name: "ispphp74"},
		},
		Users: []User{
			{ID: "1", Name: "alice"},
			{ID: "2", Name: "bob"},
		},
		DBUsers: []DBUser{
			{ID: "1", Name: "dbuser", Server: "mysql"},
			{ID: "2", Name: "other", Server: "mysql"},
		},
	}
	existing := remoteInventory{
		packages: map[string]struct{}{
			"fail2ban":   {},
			"apache-itk": {},
		},
		users: map[string]struct{}{
			"alice": {},
		},
		dbUsers: map[string]struct{}{
			databaseInventoryKey("dbuser", "mysql"): {},
		},
	}

	filtered := filterSourceDataByMissingInventory(source, existing)

	if len(filtered.Packages) != 1 || filtered.Packages[0].Name != "ispphp74" {
		t.Fatalf("expected only missing package to remain, got %#v", filtered.Packages)
	}
	if len(filtered.Users) != 1 || filtered.Users[0].Name != "bob" {
		t.Fatalf("expected only missing user to remain, got %#v", filtered.Users)
	}
	if len(filtered.DBUsers) != 1 || filtered.DBUsers[0].Name != "other" {
		t.Fatalf("expected only missing DB user to remain, got %#v", filtered.DBUsers)
	}
}

func TestBuildCommandsForFilteredRemotePackagesOmitsMatchingLocalPackages(t *testing.T) {
	t.Parallel()

	remote := SourceData{
		Packages: []Package{
			{ID: "1", Name: "fail2ban"},
			{ID: "2", Name: "ispphp56"},
			{ID: "3", Name: "ispphp74"},
		},
	}
	local := SourceData{
		Packages: []Package{
			{ID: "10", Name: "fail2ban"},
			{ID: "11", Name: "ispphp56"},
		},
	}

	filtered := filterSourceDataByMissingInventory(remote, buildRemoteInventory(local))
	groups, warnings := buildCommandsForScopes(filtered, []string{"packages"}, CommandBuildOptions{})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	joined := strings.Join(flattenCommandGroups(groups), "\n")
	if strings.Contains(joined, "package_fail2ban=on") {
		t.Fatalf("did not expect fail2ban install command for matching local package:\n%s", joined)
	}
	if strings.Contains(joined, "altphp56") {
		t.Fatalf("did not expect altphp56 in commands for matching local package:\n%s", joined)
	}
	if !strings.Contains(joined, "altphp74") {
		t.Fatalf("expected altphp74 to remain in commands:\n%s", joined)
	}
}

func TestBuildRemoteExecutionPreviewGroupsUsesRemoteState(t *testing.T) {
	t.Parallel()

	source := SourceData{
		Packages: []Package{
			{ID: "1", Name: "fail2ban"},
			{ID: "2", Name: "nginx"},
		},
		Users: []User{
			{ID: "1", Name: "alice"},
			{ID: "2", Name: "bob"},
		},
	}

	groups, warnings := buildRemoteExecutionPreviewGroups(source, []string{"packages", "users"}, Config{}, remotePreviewState{
		targetOS: "Ubuntu 24.04",
		currentPackages: map[string]struct{}{
			"fail2ban": {},
		},
		inventory: &remoteInventory{
			users: map[string]struct{}{
				"alice": {},
			},
		},
	})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	joined := strings.Join(flattenCommandGroups(groups), "\n")
	if strings.Contains(joined, "package_fail2ban=on") {
		t.Fatalf("did not expect matching remote package in preview commands:\n%s", joined)
	}
	if !strings.Contains(joined, "package_nginx=on") {
		t.Fatalf("expected missing remote package in preview commands:\n%s", joined)
	}
	if strings.Contains(joined, "name=alice") {
		t.Fatalf("did not expect existing remote user in preview commands:\n%s", joined)
	}
	if !strings.Contains(joined, "name=bob") {
		t.Fatalf("expected missing remote user in preview commands:\n%s", joined)
	}
}
