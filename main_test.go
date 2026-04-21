package main

import (
	"testing"

	"ispdb/internal/app"
)

func TestParseErrorTip(t *testing.T) {
	t.Parallel()

	if got := parseErrorTip("unknown option: --oops"); got != helpTip() {
		t.Fatalf("expected standard help tip, got %q", got)
	}
}

func TestParseErrorTipEmptyMessage(t *testing.T) {
	t.Parallel()

	if got := parseErrorTip(""); got != "" {
		t.Fatalf("expected empty tip for empty message, got %q", got)
	}
}

func TestRequiresRootForArgs(t *testing.T) {
	t.Parallel()

	if app.RequiresRootForArgs([]string{"--list", "packages", "-h"}) {
		t.Fatalf("expected -h to bypass root requirement")
	}
	if app.RequiresRootForArgs([]string{"--help"}) {
		t.Fatalf("expected --help to bypass root requirement")
	}
	if app.RequiresRootForArgs([]string{"--version"}) {
		t.Fatalf("expected --version to bypass root requirement")
	}
	if app.RequiresRootForArgs([]string{"-v"}) {
		t.Fatalf("expected -v to bypass root requirement")
	}
	if app.RequiresRootForArgs([]string{"-f", "/tmp/ispmgr.db", "--list", "users"}) {
		t.Fatalf("expected -f to bypass root requirement")
	}
	if app.RequiresRootForArgs([]string{"--dest", "192.0.2.10"}) {
		t.Fatalf("expected --dest to bypass local root preflight")
	}
	if app.RequiresRootForArgs([]string{"--file", "/tmp/ispmgr.sql", "--dest", "192.0.2.10"}) {
		t.Fatalf("expected --dest with --file to bypass local root preflight")
	}
	if !app.RequiresRootForArgs([]string{"--list", "users"}) {
		t.Fatalf("expected regular list without -f to require root")
	}
}

func TestHelpTip(t *testing.T) {
	t.Parallel()

	if got := helpTip(); got != "Tip: -h, --help to show help" {
		t.Fatalf("unexpected help tip %q", got)
	}
}
