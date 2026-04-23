package app

import (
	"path/filepath"
	"testing"
)

func TestParseConfigCommandsAlias(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--commands", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ListMode != "commands" || !cfg.ListExplicit {
		t.Fatalf("expected commands list mode, got ListMode=%q ListExplicit=%v", cfg.ListMode, cfg.ListExplicit)
	}
}

func TestParseConfigDestPort(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--dest", "192.0.2.10", "-p", "2222", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.DestPort != 2222 {
		t.Fatalf("expected DestPort=2222, got %d", cfg.DestPort)
	}
}

func TestParseConfigClean(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--list", "packages", "--columns", "name", "--clean", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if !cfg.CleanOutput {
		t.Fatalf("expected CleanOutput=true")
	}
}

func TestParseConfigListShortAlias(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"-l", "packages", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ListMode != "packages" || !cfg.ListExplicit {
		t.Fatalf("expected short -l alias to set packages list mode, got ListMode=%q ListExplicit=%v", cfg.ListMode, cfg.ListExplicit)
	}
}

func TestParseConfigVersion(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--version"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatalf("expected ShowVersion=true")
	}
}

func TestParseConfigDestExtendedFlags(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--dest", "192.0.2.10", "-y", "--overwrite", "--copy-configs", "--no-delete-packages", "--no-change-ip-addresses", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if !cfg.AutoYes || !cfg.Overwrite || !cfg.CopyConfigs || !cfg.NoDeletePackages || !cfg.NoChangeIPAddresses {
		t.Fatalf("expected all destination flags to be enabled, got %#v", cfg)
	}
}

func TestParseConfigDestScopeWithoutAuth(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--dest", "192.0.2.10", "packages", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.DestScope != "packages" {
		t.Fatalf("expected DestScope=packages, got %q", cfg.DestScope)
	}
	if cfg.DestAuth != "" {
		t.Fatalf("did not expect DestAuth when trailing dest token is a scope, got %q", cfg.DestAuth)
	}
}

func TestParseConfigDestScopeAfterAuth(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--dest", "192.0.2.10", "/root/.ssh/id_ed25519", "email", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.DestAuth != "/root/.ssh/id_ed25519" {
		t.Fatalf("expected DestAuth to be preserved, got %q", cfg.DestAuth)
	}
	if cfg.DestScope != "email" {
		t.Fatalf("expected DestScope=email, got %q", cfg.DestScope)
	}
}

func TestParseConfigListModeSupportsCommaSeparatedScopes(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--list", "dns,email", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ListMode != "dns,email" {
		t.Fatalf("expected ListMode=dns,email, got %q", cfg.ListMode)
	}
}

func TestParseConfigDestScopeSupportsCommaSeparatedScopes(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--dest", "192.0.2.10", "packages,users", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.DestScope != "packages,users" {
		t.Fatalf("expected DestScope=packages,users, got %q", cfg.DestScope)
	}
	if cfg.DestAuth != "" {
		t.Fatalf("did not expect DestAuth when trailing dest token is a scope list, got %q", cfg.DestAuth)
	}
}

func TestParseConfigExportSupportsInlineScope(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--export", "commands", "/root/out.txt", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ExportScope != "commands" {
		t.Fatalf("expected ExportScope=commands, got %q", cfg.ExportScope)
	}
	if cfg.ExportFile != filepath.Clean("/root/out.txt") {
		t.Fatalf("expected ExportFile=/root/out.txt, got %q", cfg.ExportFile)
	}
}

func TestParseConfigExportSupportsCommaSeparatedInlineScopes(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--export", "users,commands,dns", "/root/out.txt", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ExportScope != "users,commands,dns" {
		t.Fatalf("expected ExportScope=users,commands,dns, got %q", cfg.ExportScope)
	}
	if cfg.ExportFile != filepath.Clean("/root/out.txt") {
		t.Fatalf("expected ExportFile=/root/out.txt, got %q", cfg.ExportFile)
	}
}

func TestParseConfigExportStillSupportsFileOnly(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--list", "users", "--export", "/root/out.txt", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ExportFile != filepath.Clean("/root/out.txt") {
		t.Fatalf("expected ExportFile=/root/out.txt, got %q", cfg.ExportFile)
	}
	if cfg.ExportScope != "users" {
		t.Fatalf("expected ExportScope=users for file-only form with --list users, got %q", cfg.ExportScope)
	}
}

func TestParseConfigExportFileOnlyMirrorsCommaSeparatedListScope(t *testing.T) {
	t.Parallel()

	cfg, err := ParseConfig("ispdb", []string{"--list", "users,commands,dns", "--export", "/root/out.txt", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.ExportScope != "users,commands,dns" {
		t.Fatalf("expected ExportScope to mirror list scope, got %q", cfg.ExportScope)
	}
}

func TestParseConfigRejectsRemovedExportDataFlag(t *testing.T) {
	t.Parallel()

	_, err := ParseConfig("ispdb", []string{"--export-data", "commands"})
	if err == nil {
		t.Fatalf("expected error for removed --export-data flag")
	}
	if err.Error() != "unknown option: --export-data" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigNoHeadersRequiresExport(t *testing.T) {
	t.Parallel()

	_, err := ParseConfig("ispdb", []string{"--no-headers"})
	if err == nil {
		t.Fatalf("expected error for --no-headers without --export")
	}
	if err.Error() != "--no-headers requires --export <file>" {
		t.Fatalf("unexpected error: %v", err)
	}
}
