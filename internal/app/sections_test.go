package app

import (
	"strings"
	"testing"
)

func TestSectionsAllDoesNotDuplicateDBUsers(t *testing.T) {
	t.Parallel()

	data := SourceData{
		DBUsers: []DBUser{
			{ID: "1", Name: "dbuser"},
		},
	}

	sections := data.sections("all")
	count := 0
	for _, section := range sections {
		if section.Title == "db users" {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("expected db users section exactly once for all mode, got %d", count)
	}
}

func TestSectionsUseLowercaseFTPUsersTitle(t *testing.T) {
	t.Parallel()

	data := SourceData{
		FTPUsers: []FTPUser{
			{ID: "1", Name: "ftpuser"},
		},
	}

	sections := data.sections("users")
	found := false
	for _, section := range sections {
		if section.Title == "ftp users" {
			found = true
		}
		if section.Title == "FTP users" {
			t.Fatalf("unexpected legacy FTP users title found")
		}
	}

	if !found {
		t.Fatalf("expected ftp users section title")
	}
}

func TestSectionsForScopesPreserveRequestedOrder(t *testing.T) {
	t.Parallel()

	data := SourceData{
		Packages: []Package{{ID: "1", Name: "nginx"}},
		Users:    []User{{ID: "1", Name: "alice"}},
		EmailDomains: []EmailDomain{
			{ID: "1", Name: "example.com"},
		},
	}

	sections := data.sectionsForScopes([]string{"email", "packages", "users"})
	got := make([]string, 0, len(sections))
	for _, section := range sections {
		got = append(got, section.Title)
	}

	wantPrefix := []string{"email domains", "email boxes", "packages", "users", "ftp users", "db users"}
	if len(got) < len(wantPrefix) {
		t.Fatalf("expected at least %d sections, got %#v", len(wantPrefix), got)
	}
	for index, want := range wantPrefix {
		if got[index] != want {
			t.Fatalf("expected section %d to be %q, got %#v", index, want, got)
		}
	}
}

func TestSectionsForScopesDoesNotDuplicateDBUsersAcrossScopes(t *testing.T) {
	t.Parallel()

	data := SourceData{
		DBUsers: []DBUser{{ID: "1", Name: "dbuser"}},
	}

	sections := data.sectionsForScopes([]string{"users", "databases"})
	count := 0
	for _, section := range sections {
		if section.Title == "db users" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected db users section exactly once across ordered scopes, got %d", count)
	}
}

func TestPrepareListSectionsHidesDisplayColumnsAndSortsByName(t *testing.T) {
	t.Parallel()

	sections := prepareListSections([]Section{{
		Title:   "web domains",
		Headers: []string{"id", "name", "name_idn", "secure", "owner"},
		Rows: [][]string{
			{"2", "zeta.tld", "zeta", "on", "root"},
			{"1", "alpha.tld", "alpha", "off", "root"},
		},
	}})

	if len(sections) != 1 {
		t.Fatalf("expected one section, got %d", len(sections))
	}
	if got := sections[0].Headers; len(got) != 2 || got[0] != "name" || got[1] != "owner" {
		t.Fatalf("unexpected headers: %#v", got)
	}
	if got := sections[0].Rows[0][0]; got != "alpha.tld" {
		t.Fatalf("expected rows sorted by name, got first row %q", got)
	}
}

func TestPrepareListSectionsReordersRequestedColumnsForUsers(t *testing.T) {
	t.Parallel()

	sections := prepareListSections([]Section{{
		Title:   "users",
		Headers: []string{"id", "name", "active", "safepasswd", "level", "home", "fullname", "uid", "gid", "shell", "tag", "create_time", "comment", "backup", "backup_type", "backup_size_limit"},
		Rows: [][]string{
			{"1", "alice", "on", "x", "admin", "/home/alice", "Alice", "1000", "1000", "/bin/bash", "", "", "", "on", "", ""},
		},
	}})

	want := []string{"name", "home", "active", "level", "uid", "gid", "shell", "backup"}
	if got := sections[0].Headers; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected users headers order: %#v", got)
	}
}

func TestPrepareListSectionsReordersRequestedColumnsForFTPAndWebAndEmailAndDatabases(t *testing.T) {
	t.Parallel()

	sections := prepareListSections([]Section{
		{
			Title:   "ftp users",
			Headers: []string{"id", "name", "active", "enabled", "home", "password", "owner"},
			Rows:    [][]string{{"1", "ftp1", "on", "on", "/home/ftp1", "secret", "alice"}},
		},
		{
			Title:   "web domains",
			Headers: []string{"id", "name", "name_idn", "aliases", "docroot", "secure", "ssl_cert", "autosubdomain", "php_mode", "php_version", "active", "owner", "ipaddr", "redirect_http"},
			Rows:    [][]string{{"1", "site.tld", "", "www.site.tld", "/var/www/site", "on", "site.tld", "off", "php_mode_mod", "isp-php83", "on", "alice", "192.0.2.1", "off"}},
		},
		{
			Title:   "email domains",
			Headers: []string{"id", "name", "name_idn", "ip", "active", "owner", "secure", "secure_alias"},
			Rows:    [][]string{{"1", "example.com", "", "192.0.2.10", "on", "alice", "on", "mail.example.com"}},
		},
		{
			Title:   "email boxes",
			Headers: []string{"id", "name", "domain", "email_forward", "password", "path", "active", "maxsize", "used", "note"},
			Rows:    [][]string{{"1", "info", "example.com", "dest@example.net", "mailpass", "/mail/info", "on", "0", "123", "note"}},
		},
		{
			Title:   "databases",
			Headers: []string{"id", "name", "unaccounted", "owner", "db_server"},
			Rows:    [][]string{{"1", "appdb", "off", "alice", "MySQL"}},
		},
	})

	ftpWant := []string{"name", "password", "home", "active", "enabled", "owner"}
	if got := sections[0].Headers; strings.Join(got, ",") != strings.Join(ftpWant, ",") {
		t.Fatalf("unexpected ftp users headers order: %#v", got)
	}

	webWant := []string{"name", "aliases", "docroot", "php_version", "php_mode", "owner", "ssl_cert", "autosubdomain", "active", "ipaddr", "redirect_http"}
	if got := sections[1].Headers; strings.Join(got, ",") != strings.Join(webWant, ",") {
		t.Fatalf("unexpected web domains headers order: %#v", got)
	}

	emailDomainWant := []string{"name", "ip", "active", "owner", "secure", "secure_alias"}
	if got := sections[2].Headers; strings.Join(got, ",") != strings.Join(emailDomainWant, ",") {
		t.Fatalf("unexpected email domains headers order: %#v", got)
	}

	emailWant := []string{"name", "domain", "password", "email_forward", "path", "active", "maxsize", "used_mb", "note"}
	if got := sections[3].Headers; strings.Join(got, ",") != strings.Join(emailWant, ",") {
		t.Fatalf("unexpected email boxes headers order: %#v", got)
	}

	databaseWant := []string{"name", "owner", "db_server", "unaccounted"}
	if got := sections[4].Headers; strings.Join(got, ",") != strings.Join(databaseWant, ",") {
		t.Fatalf("unexpected databases headers order: %#v", got)
	}
}

func TestListSectionsForScopesAddsEmailForwardColumn(t *testing.T) {
	t.Parallel()

	data := SourceData{
		EmailBoxes: []EmailBox{{
			ID:      "1",
			Name:    "box",
			Domain:  "example.com",
			Forward: "atta.root@gmail.com",
			Active:  "on",
		}},
	}

	sections := data.listSectionsForScopes([]string{"email"})
	var emailBoxes Section
	found := false
	for _, section := range sections {
		if section.Title == "email boxes" {
			emailBoxes = section
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("email boxes section not found")
	}
	if idx := indexOfHeader(emailBoxes.Headers, "email_forward"); idx < 0 {
		t.Fatalf("email_forward header not found: %#v", emailBoxes.Headers)
	} else if got := emailBoxes.Rows[0][idx]; got != "atta.root@gmail.com" {
		t.Fatalf("unexpected email_forward value: %q", got)
	}
	if indexOfHeader(emailBoxes.Headers, "used_mb") < 0 {
		t.Fatalf("used_mb header not found: %#v", emailBoxes.Headers)
	}
	if indexOfHeader(emailBoxes.Headers, "id") >= 0 {
		t.Fatalf("id header should be hidden in list view: %#v", emailBoxes.Headers)
	}
}

func TestListSectionsForScopesUsesNoForEmptyEmailForward(t *testing.T) {
	t.Parallel()

	data := SourceData{
		EmailBoxes: []EmailBox{{
			ID:     "1",
			Name:   "box",
			Domain: "example.com",
			Active: "on",
		}},
	}

	sections := data.listSectionsForScopes([]string{"email"})
	var emailBoxes Section
	found := false
	for _, section := range sections {
		if section.Title == "email boxes" {
			emailBoxes = section
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("email boxes section not found")
	}
	idx := indexOfHeader(emailBoxes.Headers, "email_forward")
	if idx < 0 {
		t.Fatalf("email_forward header not found: %#v", emailBoxes.Headers)
	}
	if got := emailBoxes.Rows[0][idx]; got != "no" {
		t.Fatalf("expected empty email_forward to be rendered as no, got %q", got)
	}
}

func TestPrepareListSectionsTrimsPostgreSQLSavedVerForDisplay(t *testing.T) {
	t.Parallel()

	sections := prepareListSections([]Section{{
		Title:   "database servers",
		Headers: []string{"id", "name", "type", "host", "username", "password", "remote_access", "savedver"},
		Rows: [][]string{{
			"1",
			"PostgreSQL",
			"postgresql",
			"localhost",
			"postgres",
			"",
			"off",
			"PostgreSQL 16.13 (Ubuntu 16.13-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit",
		}},
	}})

	savedverIndex := indexOfHeader(sections[0].Headers, "savedver")
	if savedverIndex < 0 {
		t.Fatalf("savedver header not found: %#v", sections[0].Headers)
	}
	got := sections[0].Rows[0][savedverIndex]
	want := "PostgreSQL 16.13 (Ubuntu 16.13-0ubuntu0.24.04.1)"
	if got != want {
		t.Fatalf("unexpected trimmed savedver: %q", got)
	}
}
