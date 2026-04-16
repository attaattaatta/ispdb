package main

import "testing"

func TestShouldShowHelpAfterParseError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "unknown option", message: "unknown option: --foo", want: true},
		{name: "supported values hint", message: "unsupported --list value: x. Supported values: all, dns", want: false},
		{name: "tip hint", message: "failed to load\nTip: use ispdb -h", want: false},
		{name: "requires hint", message: "--bulk create requires --type", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldShowHelpAfterParseError(tc.message)
			if got != tc.want {
				t.Fatalf("shouldShowHelpAfterParseError(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
}
