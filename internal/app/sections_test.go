package app

import "testing"

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
	if indexOfHeader(emailBoxes.Headers, "id") >= 0 {
		t.Fatalf("id header should be hidden in list view: %#v", emailBoxes.Headers)
	}
}
