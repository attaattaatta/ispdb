package app

import "testing"

func TestSQLValueLiteral(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input any
		want  string
	}{
		{name: "nil", input: nil, want: "NULL"},
		{name: "empty string", input: "", want: "''"},
		{name: "plain string", input: "abc", want: "'abc'"},
		{name: "quoted string", input: "a'b", want: "'a\\'b'"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sqlValueLiteral(tc.input); got != tc.want {
				t.Fatalf("sqlValueLiteral(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
