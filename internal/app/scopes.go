package app

import (
	"fmt"
	"strings"
)

var dataScopeOrder = []string{"packages", "users", "dns", "webdomains", "databases", "email"}

var listAllScopeOrder = []string{"packages", "users", "dns", "webdomains", "databases", "email"}

func parseScopeList(value string, supported []string, flag string) ([]string, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !contains(supported, part) {
			return nil, unsupportedValueError(flag, part, supported)
		}
		if part == "all" {
			return []string{"all"}, nil
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("missing value for %s", flag)
	}
	return result, nil
}

func configuredScopeList(value string, supported []string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	scopes, err := parseScopeList(value, supported, "--scope")
	if err != nil {
		return nil
	}
	return scopes
}

func hasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

func dataScopesFromListMode(value string) []string {
	scopes := configuredScopeList(value, listModes)
	if len(scopes) == 0 || hasScope(scopes, "all") {
		return append([]string{}, listAllScopeOrder...)
	}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == "commands" {
			continue
		}
		result = append(result, scope)
	}
	return result
}

func displayScopesFromListMode(value string) []string {
	scopes := configuredScopeList(value, listModes)
	if len(scopes) == 0 || hasScope(scopes, "all") {
		return append([]string{}, dataScopeOrder...)
	}
	return scopes
}

func commandScopesFromListMode(value string) []string {
	scopes := configuredScopeList(value, listModes)
	if len(scopes) == 0 || hasScope(scopes, "all") {
		return append([]string{}, dataScopeOrder...)
	}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == "commands" {
			continue
		}
		result = append(result, scope)
	}
	if len(result) == 0 {
		return append([]string{}, dataScopeOrder...)
	}
	return result
}

func destScopesFromValue(value string) []string {
	scopes := configuredScopeList(value, destModes)
	if len(scopes) == 0 || hasScope(scopes, "all") {
		return append([]string{}, dataScopeOrder...)
	}
	return scopes
}

func destExecutionScopesFromValue(value string) []string {
	scopes := configuredScopeList(value, destModes)
	if len(scopes) == 0 || hasScope(scopes, "all") {
		return append([]string{}, dataScopeOrder...)
	}

	result := make([]string, 0, len(scopes))
	for _, scope := range dataScopeOrder {
		if hasScope(scopes, scope) {
			result = append(result, scope)
		}
	}
	return result
}

func isRemoteListScopeSet(value string) bool {
	scopes := configuredScopeList(value, listModes)
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if scope == "commands" {
			return false
		}
		if scope != "all" && !contains(destModes, scope) {
			return false
		}
	}
	return true
}
