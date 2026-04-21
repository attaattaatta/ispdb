package app

import "testing"

func TestBuildSourceDataMergesPresetAndUserLimitProps(t *testing.T) {
	t.Parallel()

	raw := rawSource{
		format: "SQLite",
		tables: map[string][]rawRow{
			"users": {
				{"id": "2", "name": "www-root", "preset": "#custom"},
			},
			"preset": {
				{"id": "10", "name": "#custom", "users": "2"},
			},
			"preset_props": {
				{"preset": "10", "name": "limit_php_mode", "value": "php_mode_none"},
				{"preset": "10", "name": "limit_webdomains_enabled", "value": "on"},
			},
			"userprops": {
				{"users": "2", "name": "limit_php_cgi_version", "value": "isp-php56"},
				{"users": "2", "name": "limit_shell", "value": "on"},
			},
		},
	}

	data, err := buildSourceData(raw, "")
	if err != nil {
		t.Fatalf("buildSourceData returned error: %v", err)
	}
	if len(data.Users) != 1 {
		t.Fatalf("expected one user, got %d", len(data.Users))
	}

	limits := data.Users[0].LimitProps
	if got := limits["limit_php_mode"]; got != "php_mode_none" {
		t.Fatalf("unexpected preset-derived limit_php_mode: %q", got)
	}
	if got := limits["limit_webdomains_enabled"]; got != "on" {
		t.Fatalf("unexpected preset-derived limit_webdomains_enabled: %q", got)
	}
	if got := limits["limit_php_cgi_version"]; got != "isp-php56" {
		t.Fatalf("unexpected user-derived limit_php_cgi_version: %q", got)
	}
	if got := limits["limit_shell"]; got != "on" {
		t.Fatalf("unexpected user-derived limit_shell: %q", got)
	}
}
