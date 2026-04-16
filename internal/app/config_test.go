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

	cfg, err := ParseConfig("ispdb", []string{"--dest", "192.0.2.10", "--overwrite", "--copy-configs", "--no-delete-packages", "--no-change-ip-addresses", "--help"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if !cfg.Overwrite || !cfg.CopyConfigs || !cfg.NoDeletePackages || !cfg.NoChangeIPAddresses {
		t.Fatalf("expected all destination flags to be enabled, got %#v", cfg)
	}
}
