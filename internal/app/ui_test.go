package app

import "testing"

func TestColorizeColonSuffixColorsOnlySuffix(t *testing.T) {
	t.Parallel()

	got := colorizeColonSuffix("users missing on destination side: voplmopple", colorYellow)
	want := "users missing on destination side: " + colorYellow + "voplmopple" + colorReset
	if got != want {
		t.Fatalf("colorizeColonSuffix() = %q, want %q", got, want)
	}
}

func TestColorizeColonSuffixLeavesPlainTextWithoutColon(t *testing.T) {
	t.Parallel()

	text := "entity already exists on remote side, skipped. Run again with --overwrite to modify it."
	got := colorizeColonSuffix(text, colorYellow)
	if got != text {
		t.Fatalf("colorizeColonSuffix() = %q, want %q", got, text)
	}
}

func TestColorizeStatusTextStillHighlightsOKToken(t *testing.T) {
	t.Parallel()

	got := colorizeStatusText("connecting: OK", "OK", colorGreen)
	want := "connecting: " + colorGreen + "OK" + colorReset
	if got != want {
		t.Fatalf("colorizeStatusText() = %q, want %q", got, want)
	}
}
