package app

import (
	"os"
	"path/filepath"
	"strings"
)

func discoverSSHIdentityFiles() []string {
	seen := map[string]struct{}{}
	files := make([]string, 0)
	parseSSHConfig("/etc/ssh/ssh_config", seen, &files)
	parseSSHConfig("/root/.ssh/config", seen, &files)
	return files
}

func parseSSHConfig(path string, seen map[string]struct{}, files *[]string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if _, ok := seen[path]; ok {
		return
	}
	seen[path] = struct{}{}

	baseDir := filepath.Dir(path)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		value := strings.Join(fields[1:], " ")
		switch key {
		case "include":
			pattern := expandHome(value)
			if !filepath.IsAbs(pattern) {
				pattern = filepath.Join(baseDir, pattern)
			}
			matches, _ := filepath.Glob(pattern)
			for _, match := range matches {
				parseSSHConfig(match, seen, files)
			}
		case "identityfile":
			*files = append(*files, expandHome(value))
		}
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join("/root", path[2:])
	}
	return path
}
