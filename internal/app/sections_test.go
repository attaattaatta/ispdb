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
