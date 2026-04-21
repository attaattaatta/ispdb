package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHomeUsesCurrentUserHome(t *testing.T) {
	t.Parallel()

	tempHome := t.TempDir()
	originalUserHomeDir := userHomeDirHook
	defer func() {
		userHomeDirHook = originalUserHomeDir
	}()
	userHomeDirHook = func() (string, error) {
		return tempHome, nil
	}

	got := expandHome("~/id_ed25519")
	want := filepath.Join(tempHome, "id_ed25519")
	if got != want {
		t.Fatalf("expandHome() = %q, want %q", got, want)
	}
}

func TestDiscoverSSHIdentityFilesUsesCurrentUserConfig(t *testing.T) {
	t.Parallel()

	tempHome := t.TempDir()
	sshDir := filepath.Join(tempHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	configPath := filepath.Join(sshDir, "config")
	includePath := filepath.Join(sshDir, "extra.conf")
	if err := os.WriteFile(includePath, []byte("IdentityFile ~/.ssh/id_ecdsa\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(include) failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("IdentityFile ~/.ssh/id_ed25519\nInclude extra.conf\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config) failed: %v", err)
	}

	originalUserHomeDir := userHomeDirHook
	defer func() {
		userHomeDirHook = originalUserHomeDir
	}()
	userHomeDirHook = func() (string, error) {
		return tempHome, nil
	}

	got := discoverSSHIdentityFiles()
	want := []string{
		filepath.Join(tempHome, ".ssh", "id_ed25519"),
		filepath.Join(tempHome, ".ssh", "id_ecdsa"),
	}

	if len(got) < len(want) {
		t.Fatalf("discoverSSHIdentityFiles() returned %v, want at least %v", got, want)
	}
	for i, expected := range want {
		if got[i] != expected {
			t.Fatalf("discoverSSHIdentityFiles()[%d] = %q, want %q (all: %v)", i, got[i], expected, got)
		}
	}
}
