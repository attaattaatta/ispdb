package app

import (
	"strings"
	"testing"
)

func TestBuildCommandsUsesDefaultIPAndNS(t *testing.T) {
	t.Parallel()

	data := SourceData{
		WebDomains: []WebDomain{
			{
				ID:    "1",
				Name:  "example.com",
				Owner: "alice",
			},
		},
		EmailDomains: []EmailDomain{
			{
				ID:    "1",
				Name:  "mail.example.com",
				Owner: "alice",
			},
		},
		DNSDomains: []DNSDomain{
			{
				ID:    "1",
				Name:  "example.com",
				Owner: "alice",
			},
		},
	}

	groups, warnings := buildCommands(data, "all", CommandBuildOptions{
		DefaultIP: "203.0.113.10",
		DefaultNS: "ns1.example.com. ns2.example.com.",
	})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	joined := strings.Join(flattenCommandGroups(groups), "\n")
	for _, want := range []string{
		"site_ipaddrs=203.0.113.10",
		"ipsrc=auto",
		"'ns=ns1.example.com. ns2.example.com.'",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("generated commands do not contain %q\n%s", want, joined)
		}
	}
	for _, line := range strings.Split(joined, "\n") {
		if !strings.Contains(line, "emaildomain.edit") {
			continue
		}
		if strings.Contains(line, " ip=") {
			t.Fatalf("email domain commands must not contain explicit ip anymore:\n%s", line)
		}
	}
}

func TestBuildCommandsUsesWebSitesTitleAndNoInvalidUserCGIVersion(t *testing.T) {
	t.Parallel()

	data := SourceData{
		Users: []User{
			{ID: "1", Name: "alice"},
		},
		WebDomains: []WebDomain{
			{ID: "1", Name: "example.com", Owner: "alice"},
		},
	}

	groups, warnings := buildCommands(data, "all", CommandBuildOptions{})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var foundWebSites bool
	joined := strings.Join(flattenCommandGroups(groups), "\n")
	for _, group := range groups {
		if group.Title == "web sites" {
			foundWebSites = true
		}
	}
	if !foundWebSites {
		t.Fatalf("expected web sites command group, got %#v", groups)
	}
	if strings.Contains(joined, "limit_php_cgi_version=") {
		t.Fatalf("user commands without DB limit props must not contain limit_php_cgi_version anymore:\n%s", joined)
	}
	if strings.Contains(joined, "site_analyzer=") {
		t.Fatalf("site commands must not contain site_analyzer anymore:\n%s", joined)
	}
	if !strings.Contains(joined, "sslcert.selfsigned") {
		t.Fatalf("expected self-signed certificate command to be generated before web sites:\n%s", joined)
	}
	if !strings.Contains(joined, "site_ssl_cert=example.com") {
		t.Fatalf("expected site command to reference generated self-signed certificate name:\n%s", joined)
	}
}

func TestBuildCommandsUsesUserLimitsFromProps(t *testing.T) {
	t.Parallel()

	data := SourceData{
		Users: []User{
			{
				ID:     "2",
				Name:   "www-root",
				Preset: "#custom",
				LimitProps: map[string]string{
					"limit_cgi":                "on",
					"limit_dirindex":           "index.php index.html",
					"limit_php_cgi_version":    "isp-php56",
					"limit_php_mode":           "php_mode_none",
					"limit_php_mode_lsapi":     "on",
					"limit_php_lsapi_version":  "isp-php83",
					"limit_shell":              "",
					"limit_webdomains":         "",
					"limit_webdomains_enabled": "on",
				},
			},
		},
	}

	groups, warnings := buildCommands(data, "users", CommandBuildOptions{})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	joined := strings.Join(flattenCommandGroups(groups), "\n")
	for _, want := range []string{
		"limit_cgi=on",
		"'limit_dirindex=index.php index.html'",
		"limit_php_cgi_version=isp-php56",
		"limit_php_mode=php_mode_none",
		"limit_php_mode_lsapi=on",
		"limit_php_lsapi_version=isp-php83",
		"limit_shell=",
		"limit_webdomains=",
		"limit_webdomains_enabled=on",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected generated user command to contain %q\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "limit_emaildomains_enabled=") {
		t.Fatalf("did not expect absent DB limit key to be rendered in user command:\n%s", joined)
	}
}

func TestBuildMgrctlCommandPlacesNameBeforeSokWhenNameExists(t *testing.T) {
	t.Parallel()

	command := buildMgrctlCommand("user.edit", map[string]string{
		"sok":     "ok",
		"name":    "alice",
		"backup":  "on",
		"confirm": "secret",
	})

	nameIndex := strings.Index(command, "name=alice")
	sokIndex := strings.Index(command, "sok=ok")
	if nameIndex == -1 || sokIndex == -1 {
		t.Fatalf("expected command to contain both name and sok: %s", command)
	}
	if nameIndex > sokIndex {
		t.Fatalf("expected name= to be rendered before sok=ok: %s", command)
	}
}

func TestBuildCommandsForScopesPreserveRequestedOrder(t *testing.T) {
	t.Parallel()

	data := SourceData{
		Packages: []Package{{ID: "1", Name: "nginx"}},
		Users:    []User{{ID: "1", Name: "alice"}},
		DNSDomains: []DNSDomain{
			{ID: "1", Name: "example.com", Owner: "alice"},
		},
	}

	groups, warnings := buildCommandsForScopes(data, []string{"dns", "packages", "users"}, CommandBuildOptions{
		DefaultIP: "203.0.113.10",
		DefaultNS: "ns1.example.com. ns2.example.com.",
	})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(groups) < 3 {
		t.Fatalf("expected at least 3 command groups, got %#v", groups)
	}
	if groups[0].Title != "dns" {
		t.Fatalf("expected first group to be dns, got %#v", groups)
	}
	if groups[1].Title != "packages (web)" {
		t.Fatalf("expected second group to start package groups, got %#v", groups)
	}
	foundUsers := false
	for _, group := range groups {
		if group.Title == "users" {
			foundUsers = true
			break
		}
	}
	if !foundUsers {
		t.Fatalf("expected users command group, got %#v", groups)
	}
}

func TestCommandScopesFromListModeCommandsAndDataUseOnlyRequestedDataScopes(t *testing.T) {
	t.Parallel()

	got := commandScopesFromListMode("commands,dns,email")
	want := []string{"dns", "email"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestBuildCommandsForDatabasesIncludesAllDBServers(t *testing.T) {
	t.Parallel()

	data := SourceData{
		DBServers: []DBServer{
			{ID: "1", Name: "MySQL", Host: "localhost", Type: "mysql", Username: "root"},
			{ID: "2", Name: "mariadb-10.6", Host: "localhost:3307", Type: "mysql", Username: "root", SavedVer: "mariadb:10.6"},
		},
		Databases: []Database{
			{ID: "1", Name: "appdb", Owner: "alice", Server: "MySQL"},
		},
		DBUsers: []DBUser{
			{ID: "1", Name: "appdb", Server: "MySQL", Password: "secret"},
		},
	}

	groups, warnings := buildCommandsForScopes(data, []string{"databases"}, CommandBuildOptions{})
	if len(warnings) != 2 {
		t.Fatalf("expected database server password warnings, got %v", warnings)
	}

	joined := strings.Join(flattenCommandGroups(groups), "\n")
	for _, want := range []string{
		"db.server.edit name=MySQL",
		"db.server.edit name=mariadb-10.6",
		"version=mariadb:10.6",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected generated database commands to contain %q\n%s", want, joined)
		}
	}
}

func TestBuildCommandsForDatabasesGeneratesPasswordWhenDBServerPasswordIsHidden(t *testing.T) {
	t.Parallel()

	data := SourceData{
		DBServers: []DBServer{
			{ID: "1", Name: "MySQL", Host: "localhost", Type: "mysql", Username: "root"},
		},
	}

	groups, warnings := buildCommandsForScopes(data, []string{"databases"}, CommandBuildOptions{})
	if len(groups) != 1 {
		t.Fatalf("expected one database group, got %#v", groups)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Database server MySQL password was not available") {
		t.Fatalf("expected database server password warning, got %v", warnings)
	}

	command := strings.Join(flattenCommandGroups(groups), "\n")
	if !strings.Contains(command, "db.server.edit name=MySQL") {
		t.Fatalf("expected database server command, got %s", command)
	}
	if strings.Contains(command, "password=''") || !strings.Contains(command, "password=") {
		t.Fatalf("expected generated non-empty password in command, got %s", command)
	}
}
