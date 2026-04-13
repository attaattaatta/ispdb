package app

import (
	"embed"
	"path/filepath"
	"sort"
	"strings"
)

func loadASCIIArts(artFS embed.FS) ([]string, error) {
	entries, err := artFS.ReadDir("internal/ascii")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	arts := make([]string, 0, len(names))
	for _, name := range names {
		content, err := artFS.ReadFile("internal/ascii/" + name)
		if err != nil {
			return nil, err
		}
		arts = append(arts, strings.TrimRight(string(content), "\r\n"))
	}
	return arts, nil
}
