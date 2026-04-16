package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type featureRecord struct {
	Name    string
	DName   string
	Content string
	FeatList string
	Active  string
	Promo   string
	Type    string
	Status  string
	BadState string
}

type packageSyncStep struct {
	Title            string
	Feature          string
	Command          string
	ExpectedPackages []string
}

type packagePlanOptions struct {
	TargetOS         string
	TargetPanel      string
	NoDeletePackages bool
	SkipSatisfied    bool
}

func parseFeatureRecords(output string) []featureRecord {
	lines := strings.Split(output, "\n")
	records := make([]featureRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "name=") {
			continue
		}
		values := parseKeyValueLine(line, []string{"name", "dname", "content", "featlist", "active", "promo", "type", "status", "badstate"})
		if values["name"] == "" {
			continue
		}
		records = append(records, featureRecord{
			Name:    values["name"],
			DName:   values["dname"],
			Content: values["content"],
			FeatList: values["featlist"],
			Active:  values["active"],
			Promo:   values["promo"],
			Type:    values["type"],
			Status:  firstNonEmpty(values["status"], values["badstate"]),
			BadState: values["badstate"],
		})
	}
	return records
}

func parseFeatureForm(output string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func parseKeyValueLine(line string, keys []string) map[string]string {
	type marker struct {
		key   string
		index int
	}
	markers := make([]marker, 0, len(keys))
	for _, key := range keys {
		needle := key + "="
		index := strings.Index(line, needle)
		if index >= 0 {
			markers = append(markers, marker{key: key, index: index})
		}
	}
	sort.Slice(markers, func(i, j int) bool {
		return markers[i].index < markers[j].index
	})
	values := map[string]string{}
	for i, item := range markers {
		start := item.index + len(item.key) + 1
		end := len(line)
		if i+1 < len(markers) {
			end = markers[i+1].index
		}
		values[item.key] = strings.TrimSpace(line[start:end])
	}
	return values
}

func installedPackagesFromFeatures(records []featureRecord, targetOS string) map[string]struct{} {
	packages := map[string]struct{}{}
	add := func(name string) {
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" {
			return
		}
		packages[name] = struct{}{}
	}

	for _, record := range records {
		combined := strings.ToLower(strings.TrimSpace(record.Content + " " + record.FeatList))
		active := strings.EqualFold(record.Active, "on")
		if record.Name == "" {
			continue
		}

		switch record.Name {
		case "web":
			if !active && record.Status == "" {
				continue
			}
			if strings.Contains(combined, "apache mpm-itk") {
				add(apachePackageName(targetOS))
			}
			if strings.Contains(combined, "apache prefork") {
				add("apache-prefork")
			}
			if strings.Contains(combined, "nginx") {
				add("nginx")
			}
			if strings.Contains(combined, "openlitespeed") {
				add("openlitespeed")
			}
			if strings.Contains(combined, "openlitespeed-php") {
				add("openlitespeed-php")
			}
			if strings.Contains(combined, "pagespeed module for apache") {
				add("apache_pagespeed")
			}
			if strings.Contains(combined, "pagespeed module for nginx") {
				add("nginx_pagespeed")
			}
			if strings.Contains(combined, "waf module for apache") {
				add("apache_modsecurity")
			}
			if strings.Contains(combined, "waf module for nginx") {
				add("nginx_modsecurity")
			}
			if strings.Contains(combined, "waf module for openlitespeed") {
				add("openlitespeed_modsecurity")
			}
			if strings.Contains(combined, "awstats") {
				add("awstats")
			}
			if strings.Contains(combined, "logrotate") {
				add("logrotate")
			}
			if strings.Contains(combined, "php composer") {
				add("composer")
			}
			if strings.Contains(combined, "php-fpm") {
				add("php-fpm")
			}
			if strings.Contains(combined, "php module") {
				add("php")
			}
		case "email":
			if !active && record.Status == "" {
				continue
			}
			if strings.Contains(combined, "clamav") {
				add("clamav")
			}
			if strings.Contains(combined, "dovecot") {
				add("dovecot")
			}
			if strings.Contains(combined, "exim") {
				add("exim")
			}
			if strings.Contains(combined, "spamassassin") {
				add("spamassassin")
			}
			if strings.Contains(combined, "roundcube") {
				add("roundcube")
			}
			if strings.Contains(combined, "postgrey") {
				add("postgrey")
			}
			if strings.Contains(combined, "sieve") {
				add("sieve")
			}
			if strings.Contains(combined, "opendkim") {
				add("opendkim")
			}
		case "dns":
			if !active && record.Status == "" {
				continue
			}
			if strings.Contains(combined, "bind") {
				add("bind")
			}
			if strings.Contains(combined, "powerdns") {
				add("powerdns")
			}
		case "ftp":
			if !active && record.Status == "" {
				continue
			}
			if strings.Contains(combined, "pureftp") {
				add("pureftp")
			}
			if strings.Contains(combined, "proftp") {
				add("proftp")
			}
		case "mysql":
			if !active && record.Status == "" {
				continue
			}
			if strings.Contains(combined, "maria") {
				add("mariadb-server")
			}
			if strings.Contains(combined, "mysql") {
				add("mysql-server")
			}
		case "postgresql":
			if active || record.Status != "" {
				add("postgresql")
			}
		case "phpmyadmin":
			if active || record.Status != "" {
				add("phpmyadmin")
			}
		case "phppgadmin":
			if active || record.Status != "" {
				add("phppgadmin")
			}
		case "quota":
			if active || record.Status != "" {
				add("quota")
			}
		case "psacct":
			if active || record.Status != "" {
				add("psacct")
			}
		case "fail2ban":
			if active || record.Status != "" {
				add("fail2ban")
			}
		case "ansible":
			if active || record.Status != "" {
				add("ansible")
			}
		case "nodejs":
			if active || record.Status != "" {
				add("nodejs")
			}
		case "docker":
			if active || record.Status != "" {
				add("docker")
			}
		case "wireguard":
			if active || record.Status != "" {
				add("wireguard")
			}
		case "python":
			if !active && record.Status == "" {
				continue
			}
			for _, version := range []string{"38", "39", "310", "311", "312", "313"} {
				if strings.Contains(combined, pythonLabel(version)) {
					add("isppython" + version)
				}
			}
		default:
			if strings.HasPrefix(record.Name, "altphp") {
				version := strings.TrimPrefix(record.Name, "altphp")
				if version == "" || (!active && record.Status == "") {
					continue
				}
				add("ispphp" + version)
				if strings.Contains(combined, "lsapi") {
					add("ispphp" + version + "_lsapi")
				}
			}
		}
	}

	return packages
}

func buildPackageSyncSteps(sourcePackages []Package, currentPackages map[string]struct{}, options packagePlanOptions) ([]packageSyncStep, []string) {
	sourceSet := packageSetFromSource(sourcePackages)
	steps := make([]packageSyncStep, 0, 16)
	warnings := make([]string, 0)

	appendStep := func(title string, feature string, params map[string]string, expected ...string) {
		if len(expected) == 0 && len(currentPackages) == 0 {
			return
		}
		params["elid"] = feature
		params["sok"] = "ok"
		command := buildMgrctlCommand("feature.edit", params)
		if options.SkipSatisfied && len(expected) > 0 && packageSubsetPresent(currentPackages, expected) {
			return
		}
		steps = append(steps, packageSyncStep{
			Title:            title,
			Feature:          feature,
			Command:          command,
			ExpectedPackages: sortedStrings(expected),
		})
	}

	webParams, webExpected, webWarnings := buildWebFeatureParams(sourceSet, currentPackages, options)
	warnings = append(warnings, webWarnings...)
	appendStep("packages (web)", "web", webParams, webExpected...)

	emailParams, emailExpected, emailWarnings := buildEmailFeatureParams(sourceSet, currentPackages, options)
	warnings = append(warnings, emailWarnings...)
	appendStep("packages (email)", "email", emailParams, emailExpected...)

	dnsParams, dnsExpected, dnsWarnings := buildExclusiveGroupParams("dns", "packagegroup_dns", []string{"bind", "powerdns"}, "off", sourceSet, currentPackages, options)
	warnings = append(warnings, dnsWarnings...)
	appendStep("packages (dns)", "dns", dnsParams, dnsExpected...)

	ftpParams, ftpExpected, ftpWarnings := buildExclusiveGroupParams("ftp", "packagegroup_ftp", []string{"pureftp", "proftp"}, "turn_off", sourceSet, currentPackages, options)
	warnings = append(warnings, ftpWarnings...)
	appendStep("packages (ftp)", "ftp", ftpParams, ftpExpected...)

	mysqlParams, mysqlExpected, mysqlWarnings, mysqlShouldRun := buildMySQLFeatureParams(sourceSet, currentPackages, options)
	warnings = append(warnings, mysqlWarnings...)
	if mysqlShouldRun {
		appendStep("packages (mysql)", "mysql", mysqlParams, mysqlExpected...)
	}

	if params, expected := buildSingleFeatureParams("postgresql", "package_postgresql", "postgresql", sourceSet, currentPackages, options); len(params) > 0 {
		appendStep("packages (postgresql)", "postgresql", params, expected...)
	}
	if params, expected := buildSingleFeatureParams("phpmyadmin", "package_phpmyadmin", "phpmyadmin", sourceSet, currentPackages, options); len(params) > 0 {
		appendStep("packages (phpmyadmin)", "phpmyadmin", params, expected...)
	}
	if params, expected := buildSingleFeatureParams("phppgadmin", "package_phppgadmin", "phppgadmin", sourceSet, currentPackages, options); len(params) > 0 {
		appendStep("packages (phppgadmin)", "phppgadmin", params, expected...)
	}

	for _, item := range buildAltPHPSteps(sourceSet, currentPackages, options) {
		appendStep(item.Title, item.Feature, parseFeatureParams(item.Command), item.ExpectedPackages...)
	}

	for _, spec := range []struct {
		feature string
		param   string
		pkg     string
	}{
		{feature: "quota", param: "package_quota", pkg: "quota"},
		{feature: "psacct", param: "package_psacct", pkg: "psacct"},
		{feature: "fail2ban", param: "package_fail2ban", pkg: "fail2ban"},
		{feature: "ansible", param: "package_ansible", pkg: "ansible"},
		{feature: "nodejs", param: "package_nodejs", pkg: "nodejs"},
		{feature: "docker", param: "package_docker", pkg: "docker"},
		{feature: "wireguard", param: "package_wireguard", pkg: "wireguard"},
	} {
		if spec.feature == "docker" && hasPackage(sourceSet, "docker") && !panelSupportsDocker(options.TargetPanel) {
			warnings = append(warnings, fmt.Sprintf("destination panel edition %q does not support Docker, package_docker command was skipped.", strings.TrimSpace(options.TargetPanel)))
			continue
		}
		if params, expected := buildSingleFeatureParams(spec.feature, spec.param, spec.pkg, sourceSet, currentPackages, options); len(params) > 0 {
			appendStep("packages ("+spec.feature+")", spec.feature, params, expected...)
		}
	}

	if params, expected := buildPythonFeatureParams(sourceSet, currentPackages, options); len(params) > 0 {
		appendStep("packages (python)", "python", params, expected...)
	}

	return steps, warnings
}

func buildPackageCommandGroups(sourcePackages []Package, targetOS string, targetPanel string) ([]CommandGroup, []string) {
	steps, warnings := buildPackageSyncSteps(sourcePackages, map[string]struct{}{}, packagePlanOptions{
		TargetOS:         targetOS,
		TargetPanel:      targetPanel,
		NoDeletePackages: false,
		SkipSatisfied:    false,
	})
	groups := make([]CommandGroup, 0, len(steps))
	altPHP := make([]string, 0)
	others := make([]string, 0)
	for _, step := range steps {
		if isAltPHPPackageTitle(step.Title) {
			altPHP = append(altPHP, step.Command)
			continue
		}
		if isOtherPackageTitle(step.Title) {
			others = append(others, step.Command)
			continue
		}
		groups = append(groups, CommandGroup{
			Title: step.Title,
			Commands: []string{
				featureUpdateCommand(),
				step.Command,
			},
		})
	}
	if len(altPHP) > 0 {
		commands := make([]string, 0, len(altPHP)+1)
		commands = append(commands, featureUpdateCommand())
		commands = append(commands, altPHP...)
		groups = append(groups, CommandGroup{
			Title:    "packages (altphp)",
			Commands: commands,
		})
	}
	if len(others) > 0 {
		commands := make([]string, 0, len(others)+1)
		commands = append(commands, featureUpdateCommand())
		commands = append(commands, others...)
		groups = append(groups, CommandGroup{
			Title:    "packages (others)",
			Commands: commands,
		})
	}
	return groups, warnings
}

func featureUpdateCommand() string {
	return buildMgrctlCommand("feature.update", map[string]string{
		"updatesystem":       "on",
		"upgradesystem":      "off",
		"upgradewarning_msg": "",
		"sok":                "ok",
	})
}

func isOtherPackageTitle(title string) bool {
	switch title {
	case "packages (postgresql)", "packages (phpmyadmin)", "packages (phppgadmin)", "packages (quota)", "packages (psacct)", "packages (fail2ban)", "packages (ansible)", "packages (nodejs)", "packages (docker)", "packages (wireguard)", "packages (python)":
		return true
	default:
		return false
	}
}

func isAltPHPPackageTitle(title string) bool {
	return strings.HasPrefix(title, "packages (altphp") && title != "packages (altphp)"
}

func buildWebFeatureParams(sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) (map[string]string, []string, []string) {
	params := map[string]string{}
	expected := make([]string, 0, 16)
	warnings := make([]string, 0)

	desiredApache := desiredApachePackage(sourceSet, options.TargetOS)
	currentApache := currentApachePackage(currentPackages)
	apValue, apacheExpected, conflict := resolveExclusiveValue(desiredApache, currentApache, options.NoDeletePackages, "turn_off")
	if conflict != "" {
		warnings = append(warnings, conflict)
	}
	params["packagegroup_apache"] = apValue
	if apacheExpected != "" {
		expected = append(expected, apacheExpected)
	}

	for _, item := range []struct {
		packageName string
		paramName   string
	}{
		{packageName: "apache_pagespeed", paramName: "package_pagespeed"},
		{packageName: "apache_modsecurity", paramName: "package_apache_modsecurity"},
		{packageName: "nginx", paramName: "package_nginx"},
		{packageName: "nginx_pagespeed", paramName: "package_nginx_pagespeed"},
		{packageName: "nginx_modsecurity", paramName: "package_nginx_modsecurity"},
		{packageName: "openlitespeed", paramName: "package_openlitespeed"},
		{packageName: "openlitespeed-php", paramName: "package_openlitespeed-php"},
		{packageName: "openlitespeed_modsecurity", paramName: "package_openlitespeed_modsecurity"},
		{packageName: "logrotate", paramName: "package_logrotate"},
		{packageName: "awstats", paramName: "package_awstats"},
		{packageName: "php", paramName: "package_php"},
		{packageName: "php-fpm", paramName: "package_php-fpm"},
		{packageName: "composer", paramName: "package_phpcomposer"},
	} {
		value, included := resolveToggleValue(hasPackage(sourceSet, item.packageName), hasPackage(currentPackages, item.packageName), options.NoDeletePackages)
		params[item.paramName] = value
		if included {
			expected = append(expected, item.packageName)
		}
	}

	return params, expected, warnings
}

func buildEmailFeatureParams(sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) (map[string]string, []string, []string) {
	params := map[string]string{}
	expected := make([]string, 0, 8)
	warnings := make([]string, 0)

	desiredMTA := ""
	if hasPackage(sourceSet, "exim") {
		desiredMTA = "exim"
	}
	currentMTA := currentMTA(currentPackages)
	mtaValue, mtaExpected, conflict := resolveExclusiveValue(desiredMTA, currentMTA, options.NoDeletePackages, "turn_off")
	if conflict != "" {
		warnings = append(warnings, conflict)
	}
	params["packagegroup_mta"] = mtaValue
	if mtaExpected != "" {
		expected = append(expected, mtaExpected)
	}

	mailEnabled := desiredMTA != "" || hasPackage(sourceSet, "dovecot") || hasPackage(sourceSet, "spamassassin") || hasPackage(sourceSet, "roundcube") || hasPackage(sourceSet, "postgrey") || hasPackage(sourceSet, "sieve")
	for _, item := range []struct {
		packageName string
		paramName   string
		forceOff    bool
	}{
		{packageName: "dovecot", paramName: "package_dovecot"},
		{packageName: "spamassassin", paramName: "package_spamassassin"},
		{packageName: "roundcube", paramName: "package_roundcube"},
		{packageName: "postgrey", paramName: "package_postgrey"},
		{packageName: "sieve", paramName: "package_sieve"},
		{packageName: "clamav", paramName: "package_clamav", forceOff: true},
		{packageName: "opendkim", paramName: "package_opendkim"},
	} {
		desired := hasPackage(sourceSet, item.packageName)
		if item.packageName == "opendkim" && mailEnabled {
			desired = true
		}
		if item.forceOff {
			desired = false
		}
		value, included := resolveToggleValue(desired, hasPackage(currentPackages, item.packageName), options.NoDeletePackages)
		params[item.paramName] = value
		if included {
			expected = append(expected, item.packageName)
		}
	}
	return params, expected, warnings
}

func buildExclusiveGroupParams(feature string, paramName string, choices []string, offValue string, sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) (map[string]string, []string, []string) {
	params := map[string]string{}
	expected := make([]string, 0, 1)
	warnings := make([]string, 0)
	desired := ""
	for _, choice := range choices {
		if hasPackage(sourceSet, choice) {
			desired = choice
			break
		}
	}
	current := ""
	for _, choice := range choices {
		if hasPackage(currentPackages, choice) {
			current = choice
			break
		}
	}
	value, expectedValue, conflict := resolveExclusiveValue(desired, current, options.NoDeletePackages, offValue)
	if conflict != "" {
		warnings = append(warnings, conflict)
	}
	params[paramName] = value
	if expectedValue != "" {
		expected = append(expected, expectedValue)
	}
	return params, expected, warnings
}

func buildMySQLFeatureParams(sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) (map[string]string, []string, []string, bool) {
	params := map[string]string{}
	warnings := make([]string, 0)
	expected := make([]string, 0, 1)
	if hasPackage(currentPackages, "mysql-server") || hasPackage(currentPackages, "mariadb-server") {
		return params, expected, warnings, false
	}
	desired := ""
	if hasPackage(sourceSet, "mariadb-server") {
		desired = "mariadb-server"
	} else if hasPackage(sourceSet, "mysql-server") {
		desired = "mysql-server"
	}
	if desired == "" {
		return params, expected, warnings, false
	}
	params["packagegroup_mysql"] = desired
	expected = append(expected, desired)
	return params, expected, warnings, true
}

func buildSingleFeatureParams(feature string, paramName string, packageName string, sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) (map[string]string, []string) {
	params := map[string]string{}
	value, included := resolveToggleValue(hasPackage(sourceSet, packageName), hasPackage(currentPackages, packageName), options.NoDeletePackages)
	if !hasPackage(sourceSet, packageName) && !hasPackage(currentPackages, packageName) && options.SkipSatisfied {
		return nil, nil
	}
	params[paramName] = value
	expected := make([]string, 0, 1)
	if included {
		expected = append(expected, packageName)
	}
	return params, expected
}

func buildPythonFeatureParams(sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) (map[string]string, []string) {
	versions := uniquePackageVersions(sourceSet, currentPackages, "isppython")
	if len(versions) == 0 {
		return nil, nil
	}
	params := map[string]string{
		"packagegroup_altpythongr": "python",
	}
	expected := make([]string, 0, len(versions))
	for _, version := range versions {
		name := "isppython" + version
		value, included := resolveToggleValue(hasPackage(sourceSet, name), hasPackage(currentPackages, name), options.NoDeletePackages)
		params["package_"+name] = value
		if included {
			expected = append(expected, name)
		}
	}
	return params, expected
}

func buildAltPHPSteps(sourceSet map[string]struct{}, currentPackages map[string]struct{}, options packagePlanOptions) []packageSyncStep {
	versions := uniquePackageVersions(sourceSet, currentPackages, "ispphp")
	steps := make([]packageSyncStep, 0, len(versions))
	openLiteSpeed := hasPackage(sourceSet, "openlitespeed") || hasPackage(sourceSet, "openlitespeed-php")
	for _, version := range versions {
		baseName := "ispphp" + version
		wantBase := hasPackage(sourceSet, baseName) || hasPackage(sourceSet, baseName+"_lsapi")
		currentBase := hasPackage(currentPackages, baseName) || hasPackage(currentPackages, baseName+"_lsapi")
		if !wantBase && !currentBase && options.SkipSatisfied {
			continue
		}

		params := map[string]string{
			"elid":                      "altphp" + version,
			"sok":                       "ok",
			"packagegroup_altphp" + version + "gr": baseName,
		}

		lsapiWanted := hasPackage(sourceSet, baseName+"_lsapi")
		if openLiteSpeed && wantBase {
			lsapiWanted = true
		}
		fpmWanted := wantBase && !openLiteSpeed
		modApacheWanted := wantBase && !openLiteSpeed

		params["package_"+baseName+"_lsapi"] = toggleString(lsapiWanted || (options.NoDeletePackages && hasPackage(currentPackages, baseName+"_lsapi")))
		params["package_"+baseName+"_fpm"] = toggleString(fpmWanted)
		params["package_"+baseName+"_mod_apache"] = toggleString(modApacheWanted)

		expected := []string{}
		if wantBase || (options.NoDeletePackages && currentBase) {
			expected = append(expected, baseName)
		}
		if lsapiWanted || (options.NoDeletePackages && hasPackage(currentPackages, baseName+"_lsapi")) {
			expected = append(expected, baseName+"_lsapi")
		}

		steps = append(steps, packageSyncStep{
			Title:            "packages (altphp" + version + ")",
			Feature:          "altphp" + version,
			Command:          buildMgrctlCommand("feature.edit", params),
			ExpectedPackages: expected,
		})
	}
	return steps
}

func packageSetFromSource(values []Package) map[string]struct{} {
	result := map[string]struct{}{}
	for _, item := range values {
		name := strings.TrimSpace(strings.ToLower(item.Name))
		if name == "" {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func packageSetHas(values []Package, name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, item := range values {
		if strings.TrimSpace(strings.ToLower(item.Name)) == name {
			return true
		}
	}
	return false
}

func hasPackage(values map[string]struct{}, name string) bool {
	_, ok := values[strings.TrimSpace(strings.ToLower(name))]
	return ok
}

func currentApachePackage(currentPackages map[string]struct{}) string {
	for _, name := range []string{"apache-itk-ubuntu", "apache-itk", "apache-prefork"} {
		if hasPackage(currentPackages, name) {
			return name
		}
	}
	return ""
}

func currentMTA(currentPackages map[string]struct{}) string {
	if hasPackage(currentPackages, "exim") {
		return "exim"
	}
	return ""
}

func desiredApachePackage(sourceSet map[string]struct{}, targetOS string) string {
	if hasPackage(sourceSet, "apache-prefork") {
		return "apache-prefork"
	}
	if hasPackage(sourceSet, "apache-itk") || hasPackage(sourceSet, "apache-itk-ubuntu") {
		return apachePackageName(targetOS)
	}
	return ""
}

func apachePackageName(targetOS string) string {
	if strings.Contains(strings.ToLower(targetOS), "ubuntu") {
		return "apache-itk-ubuntu"
	}
	return "apache-itk"
}

func resolveExclusiveValue(desired string, current string, noDelete bool, offValue string) (string, string, string) {
	if desired != "" {
		if noDelete && current != "" && current != desired {
			return current, current, fmt.Sprintf("cannot switch package group from %s to %s while --no-delete-packages is enabled; keeping %s.", current, desired, current)
		}
		return desired, desired, ""
	}
	if noDelete && current != "" {
		return current, current, ""
	}
	return offValue, "", ""
}

func resolveToggleValue(desired bool, current bool, noDelete bool) (string, bool) {
	if desired {
		return "on", true
	}
	if noDelete && current {
		return "on", true
	}
	return "off", false
}

func toggleString(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func uniquePackageVersions(sourceSet map[string]struct{}, currentPackages map[string]struct{}, prefix string) []string {
	versions := map[string]struct{}{}
	for name := range sourceSet {
		if version := packageVersion(name, prefix); version != "" {
			versions[version] = struct{}{}
		}
	}
	for name := range currentPackages {
		if version := packageVersion(name, prefix); version != "" {
			versions[version] = struct{}{}
		}
	}
	items := make([]string, 0, len(versions))
	for version := range versions {
		items = append(items, version)
	}
	sort.Slice(items, func(i, j int) bool {
		left, _ := strconv.Atoi(items[i])
		right, _ := strconv.Atoi(items[j])
		return left < right
	})
	return items
}

func packageVersion(name string, prefix string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if !strings.HasPrefix(name, prefix) {
		return ""
	}
	version := strings.TrimPrefix(name, prefix)
	version = strings.TrimSuffix(version, "_lsapi")
	version = strings.TrimSuffix(version, "_fpm")
	version = strings.TrimSuffix(version, "_mod_apache")
	if version == "" {
		return ""
	}
	for _, r := range version {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return version
}

func panelSupportsDocker(panelName string) bool {
	panelName = strings.ToLower(strings.TrimSpace(panelName))
	if panelName == "" {
		return true
	}
	return strings.Contains(panelName, "host") || strings.Contains(panelName, "pro")
}

func packageSubsetPresent(values map[string]struct{}, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	for _, item := range expected {
		if !hasPackage(values, item) {
			return false
		}
	}
	return true
}

func parseFeatureParams(command string) map[string]string {
	_, params, ok := parseMgrctlCommand(command)
	if !ok {
		return map[string]string{}
	}
	delete(params, "sok")
	delete(params, "out")
	delete(params, "elid")
	return params
}

func pythonLabel(version string) string {
	if strings.HasPrefix(version, "3") && len(version) > 1 {
		return "python " + version[:1] + "." + version[1:]
	}
	return "python " + version
}
