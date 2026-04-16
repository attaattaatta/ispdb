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
