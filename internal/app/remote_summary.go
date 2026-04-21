package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type remoteInventory struct {
	packages     map[string]struct{}
	users        map[string]struct{}
	ftpUsers     map[string]struct{}
	webSites     map[string]struct{}
	dbServers    map[string]struct{}
	databases    map[string]struct{}
	dbUsers      map[string]struct{}
	emailDomains map[string]struct{}
	emailBoxes   map[string]struct{}
	dnsZones     map[string]struct{}
}

type remoteFailure struct {
	Action string
	Reason string
}

type summaryState int

const (
	summaryStateSuccess summaryState = iota
	summaryStateWarning
	summaryStateError
)

func buildRemoteInventory(data SourceData) remoteInventory {
	inv := remoteInventory{
		packages:     map[string]struct{}{},
		users:        map[string]struct{}{},
		ftpUsers:     map[string]struct{}{},
		webSites:     map[string]struct{}{},
		dbServers:    map[string]struct{}{},
		databases:    map[string]struct{}{},
		dbUsers:      map[string]struct{}{},
		emailDomains: map[string]struct{}{},
		emailBoxes:   map[string]struct{}{},
		dnsZones:     map[string]struct{}{},
	}
	for _, item := range data.Packages {
		inv.packages[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	for _, item := range data.Users {
		inv.users[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	for _, item := range data.FTPUsers {
		inv.ftpUsers[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	for _, item := range data.WebDomains {
		inv.webSites[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	for _, item := range data.DBServers {
		inv.dbServers[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	for _, item := range data.Databases {
		inv.databases[databaseInventoryKey(item.Name, item.Server)] = struct{}{}
	}
	for _, item := range data.DBUsers {
		inv.dbUsers[databaseInventoryKey(item.Name, item.Server)] = struct{}{}
	}
	for _, item := range data.EmailDomains {
		inv.emailDomains[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	for _, item := range data.EmailBoxes {
		inv.emailBoxes[emailInventoryKey(item.Name, item.Domain)] = struct{}{}
	}
	for _, item := range data.DNSDomains {
		inv.dnsZones[strings.ToLower(strings.TrimSpace(item.Name))] = struct{}{}
	}
	return inv
}

func databaseInventoryKey(name string, server string) string {
	return strings.ToLower(strings.TrimSpace(server)) + "::" + strings.ToLower(strings.TrimSpace(name))
}

func emailInventoryKey(name string, domain string) string {
	return strings.ToLower(strings.TrimSpace(domain)) + "::" + strings.ToLower(strings.TrimSpace(name))
}

func databaseDisplayKey(name string, server string) string {
	name = strings.TrimSpace(name)
	server = strings.TrimSpace(server)
	if server == "" {
		return name
	}
	return name + "@" + server
}

func emailDisplayKey(name string, domain string) string {
	name = strings.TrimSpace(name)
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return name
	}
	return name + "@" + domain
}

func (inv *remoteInventory) applyCommand(command string) {
	function, params, ok := parseMgrctlCommand(command)
	if !ok {
		return
	}
	switch function {
	case "user.edit":
		inv.users[strings.ToLower(strings.TrimSpace(params["name"]))] = struct{}{}
	case "ftp.user.edit":
		inv.ftpUsers[strings.ToLower(strings.TrimSpace(params["name"]))] = struct{}{}
	case "site.edit":
		inv.webSites[strings.ToLower(strings.TrimSpace(params["site_name"]))] = struct{}{}
	case "db.server.edit":
		inv.dbServers[strings.ToLower(strings.TrimSpace(params["name"]))] = struct{}{}
	case "db.edit":
		inv.databases[databaseInventoryKey(params["name"], params["server"])] = struct{}{}
		inv.dbUsers[databaseInventoryKey(params["username"], params["server"])] = struct{}{}
	case "emaildomain.edit":
		inv.emailDomains[strings.ToLower(strings.TrimSpace(params["name"]))] = struct{}{}
	case "email.edit":
		inv.emailBoxes[emailInventoryKey(params["name"], params["domainname"])] = struct{}{}
	case "domain.edit":
		inv.dnsZones[strings.ToLower(strings.TrimSpace(params["name"]))] = struct{}{}
	}
}

func (r *remoteRunner) loadRemoteInventory(ctx context.Context) (*remoteInventory, error) {
	loaded, err := r.loadRemoteSourceData(ctx)
	if err != nil {
		return nil, err
	}
	defer loaded.cleanup()
	inventory := buildRemoteInventory(loaded.Data)
	return &inventory, nil
}

func (r *remoteRunner) printRemoteSummary(ctx context.Context, source SourceData, scope string, workflowErr error) {
	if !consoleLevelEnabled(r.cfg.LogLevel, "info") && len(r.failures) == 0 {
		return
	}

	loaded, err := r.loadRemoteSourceData(ctx)
	if err != nil {
		state := summaryStateFromOutcome(r.failures, workflowErr)
		if consoleLevelEnabled(r.cfg.LogLevel, "warn") {
			r.ui.Println(renderSummaryBannerBlock(state))
			r.ui.Println(renderSummaryLine("failed to inspect destination server state", ""))
			r.ui.Println(renderSummaryFooter())
		}
		if consoleLevelEnabled(r.cfg.LogLevel, "debug") {
			r.logger.Debug("remote summary inspection failed", "error", err)
		}
		return
	}
	defer loaded.cleanup()

	diffs := compareSourceToRemote(source, loaded.Data, scope)
	if len(diffs) == 0 && len(r.failures) == 0 && workflowErr == nil {
		if consoleLevelEnabled(r.cfg.LogLevel, "info") {
			r.ui.Println(renderSummaryBannerBlock(summaryStateSuccess))
			r.ui.Println(renderSummaryFooter())
		}
		return
	}

	state := summaryStateFromOutcome(r.failures, workflowErr)
	if consoleLevelEnabled(r.cfg.LogLevel, "info") || (state == summaryStateError && consoleLevelEnabled(r.cfg.LogLevel, "error")) {
		r.ui.Println(renderSummaryBannerBlock(state))
	}
	for _, failure := range r.failures {
		if consoleLevelEnabled(r.cfg.LogLevel, "error") {
			r.ui.Println(renderSummaryLine(fmt.Sprintf("%s: %s", failure.Action, failure.Reason), colorRed))
		}
	}
	for _, diff := range diffs {
		if consoleLevelEnabled(r.cfg.LogLevel, "warn") {
			r.ui.Println(renderSummaryLine(fmt.Sprintf("%s missing on destination side: %s", diff.title, strings.Join(diff.values, ", ")), colorYellow))
		}
	}
	if consoleLevelEnabled(r.cfg.LogLevel, "info") || (state == summaryStateError && consoleLevelEnabled(r.cfg.LogLevel, "error")) {
		r.ui.Println(renderSummaryFooter())
	}
}

func summaryStateFromOutcome(failures []remoteFailure, workflowErr error) summaryState {
	if len(failures) > 0 || workflowErr != nil {
		return summaryStateError
	}
	return summaryStateWarning
}

func renderSummaryBanner(state summaryState) string {
	color := colorGreen
	switch state {
	case summaryStateWarning:
		color = colorYellow
	case summaryStateError:
		color = colorRed
	}

	separator := formatTitle("# ================================================", true)
	center := formatTitle("# ", true) + color + "SUMMARY" + colorReset
	return separator + "\n" + center + "\n" + separator
}

func renderSummaryBannerBlock(state summaryState) string {
	return "\n" + renderSummaryBanner(state)
}

func renderSummaryFooter() string {
	return formatTitle("# ================================================", true)
}

func renderSummaryLine(text string, suffixColor string) string {
	prefix := formatTitle("# ", true)
	if suffixColor == "" {
		return prefix + text
	}
	return prefix + colorizeColonSuffix(text, suffixColor)
}

type remoteDiff struct {
	title  string
	values []string
}

func compareSourceToRemote(source SourceData, remote SourceData, scope string) []remoteDiff {
	remoteInv := buildRemoteInventory(remote)
	diffs := make([]remoteDiff, 0)
	include := func(group string) bool {
		return scope == "" || scope == "all" || scope == group
	}

	if include("packages") {
		missing := make([]string, 0)
		for _, item := range source.Packages {
			if !hasEquivalentPackage(remoteInv.packages, item.Name) {
				missing = append(missing, item.Name)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			diffs = append(diffs, remoteDiff{title: "packages", values: uniqueStringsPreserveOrder(missing)})
		}
	}
	if include("users") {
		if missing := missingBySet(source.Users, func(v User) string {
			return strings.ToLower(strings.TrimSpace(v.Name))
		}, remoteInv.users, func(v User) bool {
			return strings.EqualFold(strings.TrimSpace(v.Name), "root")
		}, func(v User) string {
			return strings.TrimSpace(v.Name)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "users", values: missing})
		}
		if missing := missingBySet(source.FTPUsers, func(v FTPUser) string {
			return strings.ToLower(strings.TrimSpace(v.Name))
		}, remoteInv.ftpUsers, nil, func(v FTPUser) string {
			return strings.TrimSpace(v.Name)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "ftp users", values: missing})
		}
	}
	if include("webdomains") {
		if missing := missingBySet(source.WebDomains, func(v WebDomain) string {
			return strings.ToLower(strings.TrimSpace(v.Name))
		}, remoteInv.webSites, nil, func(v WebDomain) string {
			return strings.TrimSpace(v.Name)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "web sites", values: missing})
		}
	}
	if include("databases") {
		if missing := missingBySet(source.DBServers, func(v DBServer) string {
			return strings.ToLower(strings.TrimSpace(v.Name))
		}, remoteInv.dbServers, nil, func(v DBServer) string {
			return strings.TrimSpace(v.Name)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "database servers", values: missing})
		}
		if missing := missingBySet(source.Databases, func(v Database) string {
			return databaseInventoryKey(v.Name, v.Server)
		}, remoteInv.databases, nil, func(v Database) string {
			return databaseDisplayKey(v.Name, v.Server)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "databases", values: missing})
		}
	}
	if include("email") {
		if missing := missingBySet(source.EmailDomains, func(v EmailDomain) string {
			return strings.ToLower(strings.TrimSpace(v.Name))
		}, remoteInv.emailDomains, nil, func(v EmailDomain) string {
			return strings.TrimSpace(v.Name)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "email domains", values: missing})
		}
		if missing := missingBySet(source.EmailBoxes, func(v EmailBox) string {
			return emailInventoryKey(v.Name, v.Domain)
		}, remoteInv.emailBoxes, nil, func(v EmailBox) string {
			return emailDisplayKey(v.Name, v.Domain)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "email boxes", values: missing})
		}
	}
	if include("dns") {
		if missing := missingBySet(source.DNSDomains, func(v DNSDomain) string {
			return strings.ToLower(strings.TrimSpace(v.Name))
		}, remoteInv.dnsZones, nil, func(v DNSDomain) string {
			return strings.TrimSpace(v.Name)
		}); len(missing) > 0 {
			diffs = append(diffs, remoteDiff{title: "dns", values: missing})
		}
	}

	return diffs
}

func filterSourceDataByMissingInventory(source SourceData, existing remoteInventory) SourceData {
	filtered := source
	filtered.Packages = filterPackagesByMissingInventory(source.Packages, existing.packages)
	filtered.Users = filterByMissingSet(source.Users, existing.users, func(v User) string {
		return strings.ToLower(strings.TrimSpace(v.Name))
	})
	filtered.FTPUsers = filterByMissingSet(source.FTPUsers, existing.ftpUsers, func(v FTPUser) string {
		return strings.ToLower(strings.TrimSpace(v.Name))
	})
	filtered.WebDomains = filterByMissingSet(source.WebDomains, existing.webSites, func(v WebDomain) string {
		return strings.ToLower(strings.TrimSpace(v.Name))
	})
	filtered.DBServers = filterByMissingSet(source.DBServers, existing.dbServers, func(v DBServer) string {
		return strings.ToLower(strings.TrimSpace(v.Name))
	})
	filtered.Databases = filterByMissingSet(source.Databases, existing.databases, func(v Database) string {
		return databaseInventoryKey(v.Name, v.Server)
	})
	filtered.DBUsers = filterByMissingSet(source.DBUsers, existing.dbUsers, func(v DBUser) string {
		return databaseInventoryKey(v.Name, v.Server)
	})
	filtered.EmailDomains = filterByMissingSet(source.EmailDomains, existing.emailDomains, func(v EmailDomain) string {
		return strings.ToLower(strings.TrimSpace(v.Name))
	})
	filtered.EmailBoxes = filterByMissingSet(source.EmailBoxes, existing.emailBoxes, func(v EmailBox) string {
		return emailInventoryKey(v.Name, v.Domain)
	})
	filtered.DNSDomains = filterByMissingSet(source.DNSDomains, existing.dnsZones, func(v DNSDomain) string {
		return strings.ToLower(strings.TrimSpace(v.Name))
	})
	return filtered
}

func filterPackagesByMissingInventory(values []Package, existing map[string]struct{}) []Package {
	filtered := make([]Package, 0, len(values))
	for _, item := range values {
		if hasEquivalentPackage(existing, item.Name) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func filterByMissingSet[T any](values []T, existing map[string]struct{}, keyFn func(T) string) []T {
	filtered := make([]T, 0, len(values))
	for _, item := range values {
		key := keyFn(item)
		if key == "" {
			filtered = append(filtered, item)
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func missingBySet[T any](values []T, keyFn func(T) string, remote map[string]struct{}, skipFn func(T) bool, displayFn func(T) string) []string {
	missing := make([]string, 0)
	for _, item := range values {
		if skipFn != nil && skipFn(item) {
			continue
		}
		key := keyFn(item)
		if key == "" {
			continue
		}
		if _, ok := remote[key]; ok {
			continue
		}
		display := key
		if displayFn != nil {
			display = displayFn(item)
		}
		missing = append(missing, display)
	}
	sort.Strings(missing)
	return uniqueStringsPreserveOrder(missing)
}
