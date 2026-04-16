package app

import "testing"

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
