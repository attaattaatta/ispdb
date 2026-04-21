package app

import "testing"

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
