package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShouldSuppressMetaOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "clean list mode without export or dest",
			cfg: Config{
				CleanOutput: true,
				ListMode:    "packages",
			},
			want: true,
		},
		{
			name: "clean mode with export keeps meta",
			cfg: Config{
				CleanOutput: true,
				ListMode:    "packages",
				ExportFile:  "/tmp/out.txt",
			},
			want: false,
		},
		{
			name: "clean mode with destination keeps meta",
			cfg: Config{
				CleanOutput: true,
				ListMode:    "packages",
				DestHost:    "192.0.2.10",
			},
			want: false,
		},
		{
			name: "without clean mode",
			cfg: Config{
				CleanOutput: false,
				ListMode:    "packages",
			},
			want: false,
		},
		{
			name: "clean mode without list mode",
			cfg: Config{
				CleanOutput: true,
				ListMode:    "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := &App{cfg: tt.cfg}
			got := a.shouldSuppressMetaOutput()
			if got != tt.want {
				t.Fatalf("shouldSuppressMetaOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldRunRemoteListWhenDestAndExplicitListAreSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "remote list with data scopes",
			cfg: Config{
				DestHost:     "192.0.2.10",
				ListExplicit: true,
				ListMode:     "databases,email",
			},
			want: true,
		},
		{
			name: "remote list with commands scope stays list-only",
			cfg: Config{
				DestHost:     "192.0.2.10",
				ListExplicit: true,
				ListMode:     "databases,commands",
			},
			want: true,
		},
		{
			name: "no explicit list",
			cfg: Config{
				DestHost:     "192.0.2.10",
				ListExplicit: false,
				ListMode:     "databases",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := &App{cfg: tt.cfg}
			got := a.shouldRunRemoteList()
			if got != tt.want {
				t.Fatalf("shouldRunRemoteList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunSkipsRootCheckForHelp(t *testing.T) {
	originalEnsureRoot := ensureRootHook
	originalEnsureLinux := ensureLinuxHook
	originalAcquireLock := acquireLockHook
	t.Cleanup(func() {
		ensureRootHook = originalEnsureRoot
		ensureLinuxHook = originalEnsureLinux
		acquireLockHook = originalAcquireLock
	})

	rootErr := errors.New("root required")
	ensureRootHook = func() error { return rootErr }
	ensureLinuxHook = func() error { return nil }
	acquireLockHook = func(string) (*os.File, error) { return nil, nil }

	var out bytes.Buffer
	var errBuf bytes.Buffer
	app := &App{
		cfg:     Config{ShowHelp: true},
		ui:      &UI{out: &out, err: &errBuf, rng: rand.New(rand.NewSource(time.Now().UnixNano()))},
		arts:    []string{"ascii"},
		logger:  slog.Default(),
		version: "0.4.0-beta",
	}

	err := app.Run()
	if err != nil {
		t.Fatalf("expected help to bypass root check, got %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected help output, got stdout=%q stderr=%q", out.String(), errBuf.String())
	}
}

func TestCheckRootPreflightUsesRootHook(t *testing.T) {
	originalEnsureRoot := ensureRootHook
	t.Cleanup(func() {
		ensureRootHook = originalEnsureRoot
	})

	rootErr := errors.New("root required")
	ensureRootHook = func() error { return rootErr }

	err := CheckRootPreflight()
	if !errors.Is(err, rootErr) {
		t.Fatalf("expected root error from preflight, got %v", err)
	}
}

func TestRequiresRootForConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "help bypasses root",
			cfg:  Config{ShowHelp: true},
			want: false,
		},
		{
			name: "version bypasses root",
			cfg:  Config{ShowVersion: true},
			want: false,
		},
		{
			name: "explicit file bypasses root",
			cfg:  Config{DBFile: "/tmp/ispmgr.db"},
			want: false,
		},
		{
			name: "destination with explicit file bypasses local root",
			cfg:  Config{DBFile: "/tmp/ispmgr.db", DestHost: "192.0.2.10"},
			want: false,
		},
		{
			name: "default source requires root",
			cfg:  Config{},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RequiresRootForConfig(tt.cfg); got != tt.want {
				t.Fatalf("RequiresRootForConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultLockPathUsesUserHome(t *testing.T) {
	originalUserHomeDir := userHomeDirHook
	t.Cleanup(func() {
		userHomeDirHook = originalUserHomeDir
	})

	userHomeDirHook = func() (string, error) {
		return "/home/tester", nil
	}

	got, err := defaultLockPath()
	if err != nil {
		t.Fatalf("defaultLockPath() returned error: %v", err)
	}
	want := filepath.Join("/home/tester", ".ispdb", "ispdb.lock")
	if got != want {
		t.Fatalf("defaultLockPath() = %q, want %q", got, want)
	}
}

func TestLocalBackupDirsUseUserHome(t *testing.T) {
	originalUserHomeDir := userHomeDirHook
	t.Cleanup(func() {
		userHomeDirHook = originalUserHomeDir
	})

	userHomeDirHook = func() (string, error) {
		return "/home/tester", nil
	}

	supportDir, stateDir, err := localBackupDirs()
	if err != nil {
		t.Fatalf("localBackupDirs() returned error: %v", err)
	}
	wantSupport := filepath.Join("/home/tester", "support")
	wantState := filepath.Join("/home/tester", ".ispdb")
	if supportDir != wantSupport {
		t.Fatalf("supportDir = %q, want %q", supportDir, wantSupport)
	}
	if stateDir != wantState {
		t.Fatalf("stateDir = %q, want %q", stateDir, wantState)
	}
}

func TestInternalLogEnabledRequiresExplicitLogFlag(t *testing.T) {
	t.Parallel()

	appWithoutExplicitLog := &App{
		cfg:    Config{LogLevel: "debug"},
		logger: slog.Default(),
	}
	if appWithoutExplicitLog.internalLogEnabled() {
		t.Fatalf("expected internal logging to stay disabled without explicit --log")
	}

	appWithExplicitLog := &App{
		cfg:    Config{LogLevel: "debug", LogExplicit: true},
		logger: slog.Default(),
	}
	if !appWithExplicitLog.internalLogEnabled() {
		t.Fatalf("expected internal logging to be enabled with explicit --log")
	}
}

func TestRenderOrderedListOutputShowsNoDifferencesMessageForEmptyCommands(t *testing.T) {
	t.Parallel()

	app := &App{}
	output := app.renderOrderedListOutput(SourceData{}, []string{"commands"}, nil)

	if !strings.Contains(output, "No differences were found. Nothing to sync.") {
		t.Fatalf("expected no-differences message, got:\n%s", output)
	}
}

func TestRenderOrderedListOutputKeepsDBUsersOnlyInLastScopeOccurrence(t *testing.T) {
	t.Parallel()

	app := &App{}
	data := SourceData{
		DBUsers:   []DBUser{{ID: "1", Name: "dbuser", Server: "MySQL"}},
		DBServers: []DBServer{{ID: "1", Name: "MySQL"}},
	}

	output := app.renderOrderedListOutput(data, []string{"users", "databases"}, nil)

	if count := strings.Count(output, "db users:"); count != 1 {
		t.Fatalf("expected db users section once, got %d\n%s", count, output)
	}
	dbUsersIndex := strings.Index(output, "db users:")
	dbServersIndex := strings.Index(output, "database servers:")
	if dbUsersIndex < 0 || dbServersIndex < 0 || dbUsersIndex < dbServersIndex {
		t.Fatalf("expected db users section after database servers\n%s", output)
	}
}

func TestReorderDestCommandGroupsPlacesDNSBetweenUsersAndFTPUsers(t *testing.T) {
	t.Parallel()

	groups := []CommandGroup{
		{Title: "packages (email)", Commands: []string{"pkg"}},
		{Title: "users", Commands: []string{"user"}},
		{Title: "ftp users", Commands: []string{"ftp"}},
		{Title: "web sites", Commands: []string{"site"}},
		{Title: "dns", Commands: []string{"dns"}},
	}

	got := reorderDestCommandGroups(groups)
	wantTitles := []string{"packages (email)", "users", "dns", "ftp users", "web sites"}
	if len(got) != len(wantTitles) {
		t.Fatalf("unexpected reordered groups: %#v", got)
	}
	for i, want := range wantTitles {
		if got[i].Title != want {
			t.Fatalf("reorderDestCommandGroups() titles = %#v, want %v", got, wantTitles)
		}
	}
}

func TestConfirmRemoteExecutionShowsCommandsAndStopsOnNo(t *testing.T) {
	originalAsk := askYesNoWithColorHook
	t.Cleanup(func() {
		askYesNoWithColorHook = originalAsk
	})

	var asked string
	askYesNoWithColorHook = func(question string, defaultNo bool, color string) (bool, error) {
		asked = question
		if !defaultNo {
			t.Fatalf("expected defaultNo=true")
		}
		return false, nil
	}

	var out bytes.Buffer
	app := &App{
		ui: &UI{out: &out, err: &out, rng: rand.New(rand.NewSource(time.Now().UnixNano()))},
	}

	err := app.confirmRemoteExecution([]CommandGroup{
		{Title: "users", Commands: []string{"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=alice sok=ok"}},
	})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if asked != "Continue with remote sync?" {
		t.Fatalf("unexpected confirmation question: %q", asked)
	}
	if !strings.Contains(out.String(), "commands to run at remote server:") {
		t.Fatalf("expected command preview before confirmation, got:\n%s", out.String())
	}
}

func TestPruneRemoteExecutionPreviewGroupsUsesDestinationFeatureForm(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{
		logger: slog.Default(),
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			if strings.Contains(command, "feature.edit") && strings.Contains(command, "out=text") {
				return strings.Join([]string{
					"title=Web-server (WWW)",
					"package_nginx=on",
					"package_logrotate=on",
					"package_awstats=on",
					"package_php=on",
					"package_php-fpm=on",
					"elid=web",
					"package_openlitespeed=off",
					"package_phpcomposer=off",
					"package_nginx_modsecurity=off",
					"package_apache_modsecurity=off",
					"package_openlitespeed_modsecurity=off",
					"packagegroup_apache=apache-itk-ubuntu",
				}, "\n"), nil
			}
			return "", nil
		},
	}

	groups := []CommandGroup{{
		Title: "packages (web)",
		Commands: []string{
			"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=web package_apache_modsecurity=off package_awstats=on package_logrotate=on package_nginx=off package_nginx_modsecurity=off package_openlitespeed=on package_openlitespeed-php=on package_openlitespeed_modsecurity=off package_php=off package_php-fpm=off package_phpcomposer=off packagegroup_apache=turn_off",
		},
	}}

	pruned := pruneRemoteExecutionPreviewGroups(context.Background(), runner, groups)
	got := pruned[0].Commands[0]
	if !strings.Contains(got, "package_openlitespeed-php=on") {
		t.Fatalf("expected package_openlitespeed-php to remain in preview alongside package_openlitespeed=on, got %q", got)
	}
	if !strings.Contains(got, "package_nginx=off") || !strings.Contains(got, "packagegroup_apache=turn_off") {
		t.Fatalf("expected supported diff params to stay in preview, got %q", got)
	}
}
