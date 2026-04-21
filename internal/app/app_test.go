package app

import (
	"bytes"
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

func TestRenderOrderedListOutputShowsNoDifferencesMessageForEmptyCommands(t *testing.T) {
	t.Parallel()

	app := &App{}
	output := app.renderOrderedListOutput(SourceData{}, []string{"commands"}, nil)

	if !strings.Contains(output, "No differences were found. Nothing to sync.") {
		t.Fatalf("expected no-differences message, got:\n%s", output)
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
