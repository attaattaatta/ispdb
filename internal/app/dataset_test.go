package app

import (
	"path/filepath"
	"strings"
	"testing"
)

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

func TestBuildSourceDataLoadsEmailForwards(t *testing.T) {
	t.Parallel()

	raw := rawSource{
		format: "SQLite",
		tables: map[string][]rawRow{
			"emaildomain": {
				{"id": "255", "name": "example.com"},
			},
			"email": {
				{"id": "2", "name": "box", "domain": "255", "active": "on"},
			},
			"email_forward": {
				{"id": "1", "name": "atta.root@gmail.com", "email": "2"},
				{"id": "2", "name": "admin@example.net", "email": "2"},
			},
		},
	}

	data, err := buildSourceData(raw, "")
	if err != nil {
		t.Fatalf("buildSourceData returned error: %v", err)
	}
	if len(data.EmailBoxes) != 1 {
		t.Fatalf("expected one email box, got %d", len(data.EmailBoxes))
	}
	if got := data.EmailBoxes[0].Forward; got != "admin@example.net, atta.root@gmail.com" {
		t.Fatalf("unexpected email forwards: %q", got)
	}
}

func TestBuildSourceDataIgnoresPasswordValuesWithoutPrivateKey(t *testing.T) {
	t.Parallel()

	raw := rawSource{
		format: "SQLite",
		tables: map[string][]rawRow{
			"db_server": {
				{"id": "1", "name": "MySQL", "type": "mysql", "host": "localhost", "username": "root", "password": "ZW5jcnlwdGVk"},
			},
			"ftp_users": {
				{"id": "1", "name": "ftp", "users": "2", "password": "ZW5jcnlwdGVk"},
			},
			"db_users_password": {
				{"id": "1", "name": "dbuser", "db_server": "1", "password": "ZW5jcnlwdGVk"},
			},
			"emaildomain": {
				{"id": "255", "name": "example.com"},
			},
			"email": {
				{"id": "1", "name": "box", "domain": "255", "password": "ZW5jcnlwdGVk", "active": "on"},
			},
			"users": {
				{"id": "2", "name": "owner"},
			},
		},
	}

	data, err := buildSourceData(raw, "")
	if err != nil {
		t.Fatalf("buildSourceData returned error: %v", err)
	}

	if got := data.DBServers[0].Password; got != "" {
		t.Fatalf("expected db server password to be hidden, got %q", got)
	}
	if got := data.FTPUsers[0].Password; got != "" {
		t.Fatalf("expected ftp password to be hidden, got %q", got)
	}
	if got := data.DBUsers[0].Password; got != "" {
		t.Fatalf("expected db user password to be hidden, got %q", got)
	}
	if got := data.EmailBoxes[0].Password; got != "" {
		t.Fatalf("expected mailbox password to be hidden, got %q", got)
	}
	if len(data.Warnings) == 0 || !strings.Contains(data.Warnings[0], "password values were ignored") {
		t.Fatalf("expected warning about ignored password values, got %v", data.Warnings)
	}
}

func TestBuildSourceDataIgnoresPasswordValuesWhenPrivateKeyLoadFails(t *testing.T) {
	t.Parallel()

	raw := rawSource{
		format: "SQLite",
		tables: map[string][]rawRow{
			"db_server": {
				{"id": "1", "name": "MySQL", "type": "mysql", "host": "localhost", "username": "root", "password": "ZW5jcnlwdGVk"},
			},
		},
	}

	data, err := buildSourceData(raw, filepath.Join(t.TempDir(), "missing.pem"))
	if err != nil {
		t.Fatalf("buildSourceData returned error: %v", err)
	}

	if data.PrivateKeyUsed {
		t.Fatalf("expected private key to stay unused")
	}
	if got := data.DBServers[0].Password; got != "" {
		t.Fatalf("expected db server password to be hidden, got %q", got)
	}
	if len(data.Warnings) == 0 || !strings.Contains(data.Warnings[0], "could not be loaded") {
		t.Fatalf("expected key load warning, got %v", data.Warnings)
	}
}
