package app

import (
	"strings"
	"testing"
)

func TestParseFeatureRecords(t *testing.T) {
	t.Parallel()

	output := "name=web dname=web content=Apache MPM-ITK 2.4, nginx 1.28 featlist=Apache MPM-ITK 2.4, nginx 1.28 active=on promo=off type=recommended\n" +
		"name=altphp72 dname=altphp72 content=PHP 7.2 LSAPI, PHP 7.2 common featlist=PHP 7.2 LSAPI, PHP 7.2 common active=on promo=off"

	records := parseFeatureRecords(output)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Name != "web" || !strings.Contains(records[0].Content, "Apache MPM-ITK") {
		t.Fatalf("unexpected first record: %#v", records[0])
	}
	if records[1].Name != "altphp72" || records[1].Active != "on" {
		t.Fatalf("unexpected second record: %#v", records[1])
	}
}

func TestParseFeatureRecordsKeepsStatusSeparateFromBadState(t *testing.T) {
	t.Parallel()

	output := "name=web dname=web content= featlist= active=off promo=off type=recommended badstate=install\n"

	records := parseFeatureRecords(output)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Status != "" {
		t.Fatalf("expected empty status when only badstate is present, got %#v", records[0])
	}
	if records[0].BadState != "install" {
		t.Fatalf("expected badstate=install, got %#v", records[0])
	}
}

func TestInstalledPackagesFromFeatures(t *testing.T) {
	t.Parallel()

	records := parseFeatureRecords("" +
		"name=web dname=web content=Apache MPM-ITK 2.4, PHP Composer, PHP-FPM 8.2, nginx 1.28 featlist=Apache MPM-ITK 2.4, PHP Composer, PHP-FPM 8.2, nginx 1.28 active=on promo=off type=recommended\n" +
		"name=email dname=email content=Dovecot, Exim, OpenDKIM featlist=Dovecot, Exim, OpenDKIM active=on promo=off type=recommended\n" +
		"name=python dname=python content=Python, Python 3.8, Python 3.11 featlist=Python, Python 3.8, Python 3.11 active=on promo=off\n" +
		"name=altphp72 dname=altphp72 content=PHP 7.2 LSAPI, PHP 7.2 common featlist=PHP 7.2 LSAPI, PHP 7.2 common active=on promo=off\n")

	packages := installedPackagesFromFeatures(records, "Ubuntu 24.04")
	for _, want := range []string{"apache-itk-ubuntu", "nginx", "composer", "php-fpm", "dovecot", "exim", "opendkim", "isppython38", "isppython311", "ispphp72", "ispphp72_lsapi"} {
		if !hasPackage(packages, want) {
			t.Fatalf("expected package %q in detected set", want)
		}
	}
}

func TestBuildPackageCommandGroups(t *testing.T) {
	t.Parallel()

	groups, warnings := buildPackageCommandGroups([]Package{
		{ID: "1", Name: "nginx"},
		{ID: "2", Name: "logrotate"},
		{ID: "3", Name: "mariadb-server"},
		{ID: "4", Name: "dovecot"},
		{ID: "5", Name: "exim"},
		{ID: "6", Name: "opendkim"},
		{ID: "7", Name: "ispphp72"},
		{ID: "8", Name: "ispphp72_lsapi"},
		{ID: "9", Name: "fail2ban"},
		{ID: "10", Name: "wireguard"},
		{ID: "11", Name: "isppython311"},
		{ID: "12", Name: "docker"},
	}, "Ubuntu 24.04", "ispmanager Host")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	joined := strings.Join(flattenCommandGroups(groups), "\n")
	for _, want := range []string{
		"package_nginx=on",
		"package_logrotate=on",
		"packagegroup_mysql=mariadb-server",
		"package_dovecot=on",
		"package_opendkim=on",
		"feature.resume",
		"'elid=altphp72'",
		"'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'",
		"package_fail2ban=on",
		"package_wireguard=on",
		"package_docker=on",
		"package_isppython311=on",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected generated package commands to contain %q\n%s", want, joined)
		}
	}

	for _, line := range strings.Split(joined, "\n") {
		if strings.Contains(line, "/usr/local/mgr5/sbin/mgrctl") && strings.Contains(line, " sok=ok ") {
			functionMarker := " ispmgr "
			index := strings.Index(line, functionMarker)
			if index == -1 {
				continue
			}
			afterFunction := line[index+len(functionMarker):]
			spaceIndex := strings.Index(afterFunction, " ")
			if spaceIndex == -1 {
				continue
			}
			args := afterFunction[spaceIndex+1:]
			if !strings.HasPrefix(args, "sok=ok ") && args != "sok=ok" {
				t.Fatalf("expected sok=ok to be the first argument in command: %s", line)
			}
		}
	}

	if strings.Contains(joined, "out=devel") {
		t.Fatalf("package commands must not contain out=devel anymore\n%s", joined)
	}

	altPHPIndex := -1
	seenOthers := map[string]bool{}
	for index, group := range groups {
		if group.Title == "packages (altphp)" {
			altPHPIndex = index
			if len(group.Commands) < 1 {
				t.Fatalf("expected combined altphp group to contain at least one command, got %#v", group)
			}
		}
		switch group.Title {
		case "packages (fail2ban)", "packages (wireguard)", "packages (docker)", "packages (python)":
			seenOthers[group.Title] = true
			if len(group.Commands) != 1 {
				t.Fatalf("expected standalone other package group to contain one command, got %#v", group)
			}
		}
	}
	if altPHPIndex == -1 {
		t.Fatalf("expected packages (altphp) group, got %#v", groups)
	}
	for _, title := range []string{"packages (fail2ban)", "packages (wireguard)", "packages (docker)", "packages (python)"} {
		if !seenOthers[title] {
			t.Fatalf("expected standalone group %s, got %#v", title, groups)
		}
	}
}

func TestBuildPackageCommandGroupsSkipsEmptyDNSGroup(t *testing.T) {
	t.Parallel()

	groups, warnings := buildPackageCommandGroups([]Package{
		{ID: "1", Name: "nginx"},
	}, "Ubuntu 24.04", "ispmanager Host")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	for _, group := range groups {
		if group.Title == "packages (dns)" {
			t.Fatalf("did not expect dns package group when source has no dns packages: %#v", groups)
		}
	}
}

func TestBuildPackageSyncStepsCombinesAltPHPVersionsIntoSingleStep(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{
			{ID: "1", Name: "ispphp72"},
			{ID: "2", Name: "ispphp72_lsapi"},
			{ID: "3", Name: "ispphp83"},
			{ID: "4", Name: "ispphp83_lsapi"},
		},
		map[string]struct{}{},
		packagePlanOptions{
			TargetOS:      "Ubuntu 24.04",
			TargetPanel:   "ispmanager Host",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	altPHPCount := 0
	var altPHPStep packageSyncStep
	for _, step := range steps {
		if step.Title == "packages (altphp)" {
			altPHPCount++
			altPHPStep = step
		}
	}
	if altPHPCount != 1 {
		t.Fatalf("expected exactly one combined altphp step, got %d: %#v", altPHPCount, steps)
	}
	if !strings.Contains(altPHPStep.Command, "feature.resume") {
		t.Fatalf("expected combined altphp command to use feature.resume, got %q", altPHPStep.Command)
	}
	if !strings.Contains(altPHPStep.Command, "'elid=altphp72, altphp83'") {
		t.Fatalf("expected combined altphp command to contain both versions, got %q", altPHPStep.Command)
	}
	if !strings.Contains(altPHPStep.Command, "'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'") {
		t.Fatalf("expected combined altphp command to contain quoted sample elname, got %q", altPHPStep.Command)
	}
}

func TestBuildPackageSyncStepsUsesOnlyDifferingAltPHPVersions(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{
			{ID: "1", Name: "ispphp72"},
			{ID: "2", Name: "ispphp72_lsapi"},
			{ID: "3", Name: "ispphp83"},
			{ID: "4", Name: "ispphp83_lsapi"},
		},
		map[string]struct{}{
			"ispphp72":       {},
			"ispphp72_lsapi": {},
			"ispphp84":       {},
		},
		packagePlanOptions{
			TargetOS:      "Ubuntu 24.04",
			TargetPanel:   "ispmanager Host",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var altPHPStep *packageSyncStep
	for i := range steps {
		if steps[i].Feature == "altphp" {
			altPHPStep = &steps[i]
			break
		}
	}
	if altPHPStep == nil {
		t.Fatalf("expected altphp step, got %#v", steps)
	}
	if strings.Contains(altPHPStep.Command, "altphp72") {
		t.Fatalf("did not expect already matching altphp72 in diff command, got %q", altPHPStep.Command)
	}
	if !strings.Contains(altPHPStep.Command, "'elid=altphp83'") {
		t.Fatalf("expected only missing altphp83 in diff command, got %q", altPHPStep.Command)
	}
	if strings.Contains(altPHPStep.Command, "altphp84") {
		t.Fatalf("did not expect extra current altphp84 to appear in resume command, got %q", altPHPStep.Command)
	}
}

func TestBuildPackageSyncStepsSkipsDNSGroupWhenAbsentOnSourceAndDestination(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{{ID: "1", Name: "nginx"}},
		map[string]struct{}{"nginx": {}},
		packagePlanOptions{
			TargetOS:      "AlmaLinux 10",
			TargetPanel:   "ispmanager Lite",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	for _, step := range steps {
		if step.Feature == "dns" {
			t.Fatalf("did not expect dns sync step when dns packages are absent on both sides: %#v", steps)
		}
	}
}

func TestBuildPackageSyncStepsDoesNotSkipWebGroupWhenDestinationHasExtraPackages(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{
			{ID: "1", Name: "nginx"},
			{ID: "2", Name: "php"},
		},
		map[string]struct{}{
			"nginx":      {},
			"php":        {},
			"logrotate":  {},
			"apache-itk": {},
		},
		packagePlanOptions{
			TargetOS:      "AlmaLinux 9",
			TargetPanel:   "ispmanager Lite",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	for _, step := range steps {
		if step.Feature == "web" {
			return
		}
	}
	t.Fatalf("expected web step to remain when destination has extra web packages, got %#v", steps)
}

func TestBuildPackageSyncStepsUsesOnlyDifferingArgsForWebGroup(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{
			{ID: "1", Name: "nginx"},
			{ID: "2", Name: "php"},
		},
		map[string]struct{}{
			"nginx":      {},
			"php":        {},
			"logrotate":  {},
			"apache-itk": {},
		},
		packagePlanOptions{
			TargetOS:      "AlmaLinux 9",
			TargetPanel:   "ispmanager Lite",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var webStep *packageSyncStep
	for i := range steps {
		if steps[i].Feature == "web" {
			webStep = &steps[i]
			break
		}
	}
	if webStep == nil {
		t.Fatalf("expected web step, got %#v", steps)
	}

	command := webStep.Command
	for _, want := range []string{
		"packagegroup_apache=turn_off",
		"package_logrotate=off",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected web diff command to contain %q, got %q", want, command)
		}
	}
	for _, dontWant := range []string{
		"package_nginx=on",
		"package_php=on",
		"package_php-fpm=",
		"package_awstats=",
		"package_phpcomposer=",
	} {
		if strings.Contains(command, dontWant) {
			t.Fatalf("did not expect unchanged web arg %q in diff command %q", dontWant, command)
		}
	}
}

func TestBuildPackageSyncStepsAlwaysKeepsClamAVOffInEmailGroup(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{
			{ID: "1", Name: "dovecot"},
			{ID: "2", Name: "exim"},
		},
		map[string]struct{}{
			"dovecot": {},
		},
		packagePlanOptions{
			TargetOS:      "Ubuntu 24.04",
			TargetPanel:   "ispmanager Lite",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var emailStep *packageSyncStep
	for i := range steps {
		if steps[i].Feature == "email" {
			emailStep = &steps[i]
			break
		}
	}
	if emailStep == nil {
		t.Fatalf("expected email step, got %#v", steps)
	}
	if !strings.Contains(emailStep.Command, "package_clamav=off") {
		t.Fatalf("expected email command to always contain package_clamav=off, got %q", emailStep.Command)
	}
}

func TestBuildPackageCommandGroupsIncludeEmailPackagesWhenPresentOnSource(t *testing.T) {
	t.Parallel()

	groups, warnings := buildPackageCommandGroupsWithCurrent(
		[]Package{
			{ID: "1", Name: "exim"},
			{ID: "2", Name: "dovecot"},
			{ID: "3", Name: "opendkim"},
			{ID: "4", Name: "sieve"},
		},
		"Ubuntu 24.04",
		"ispmanager Lite",
		nil,
		false,
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var emailGroup CommandGroup
	found := false
	for _, group := range groups {
		if group.Title == "packages (email)" {
			emailGroup = group
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected packages (email) group, got %#v", groups)
	}

	joined := strings.Join(emailGroup.Commands, "\n")
	for _, want := range []string{
		"packagegroup_mta=exim",
		"package_dovecot=on",
		"package_opendkim=on",
		"package_sieve=on",
		"package_clamav=off",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected email package command to contain %q, got %q", want, joined)
		}
	}
}

func TestBuildPackageSyncStepsWebOpenLiteSpeedReplacesApacheNginxAndPHPOnUbuntu(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{
			{ID: "1", Name: "awstats"},
			{ID: "2", Name: "logrotate"},
			{ID: "3", Name: "openlitespeed"},
			{ID: "4", Name: "openlitespeed-php"},
		},
		map[string]struct{}{
			"apache-itk-ubuntu": {},
			"nginx":             {},
			"php":               {},
			"php-fpm":           {},
			"awstats":           {},
			"logrotate":         {},
		},
		packagePlanOptions{
			TargetOS:      "Ubuntu 24.04.3 (x86_64)",
			TargetPanel:   "ispmanager Host",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	var webStep *packageSyncStep
	for i := range steps {
		if steps[i].Feature == "web" {
			webStep = &steps[i]
			break
		}
	}
	if webStep == nil {
		t.Fatalf("expected web step, got %#v", steps)
	}

	command := webStep.Command
	for _, want := range []string{
		"package_nginx=off",
		"package_openlitespeed=on",
		"package_openlitespeed-php=on",
		"package_php=off",
		"package_php-fpm=off",
		"packagegroup_apache=turn_off",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected web diff command to contain %q, got %q", want, command)
		}
	}
}

func TestBuildPackageSyncStepsSkipsStandaloneFeatureWhenAlreadyMatching(t *testing.T) {
	t.Parallel()

	steps, warnings := buildPackageSyncSteps(
		[]Package{{ID: "1", Name: "fail2ban"}},
		map[string]struct{}{"fail2ban": {}},
		packagePlanOptions{
			TargetOS:      "Ubuntu 24.04",
			TargetPanel:   "ispmanager Lite",
			SkipSatisfied: true,
		},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	for _, step := range steps {
		if step.Feature == "fail2ban" {
			t.Fatalf("did not expect fail2ban step when destination already matches source: %#v", steps)
		}
	}
}
