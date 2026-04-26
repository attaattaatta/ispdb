package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestFilterEntityCommandsSkipsAltPHPFeatureResume(t *testing.T) {
	t.Parallel()

	commands := []string{
		featureUpdateCommand(),
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp72 altphp83' 'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=alice sok=ok",
	}

	filtered := filterEntityCommands(commands)
	if len(filtered) != 1 {
		t.Fatalf("expected only one entity command after filtering, got %#v", filtered)
	}
	if filtered[0] != "/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=alice sok=ok" {
		t.Fatalf("unexpected filtered commands: %#v", filtered)
	}
}

func TestRewriteCommandForRemoteIPUsesDestinationSiteForm(t *testing.T) {
	t.Parallel()

	command := "/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit sok=ok site_name=rem.biz 'site_aliases=*.rem.biz, www.rem.biz' site_ipaddrs=79.174.15.25 site_ssl_cert=rem.biz_move-2026-04-05"
	rewritten := rewriteCommandForRemoteIP(command, "188.120.249.93")

	if !strings.Contains(rewritten, "site_ipaddrs=188.120.249.93") {
		t.Fatalf("expected destination IP in rewritten command:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "site_ssl_cert=") {
		t.Fatalf("did not expect site_ssl_cert in rewritten command:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "79.174.15.25") || strings.Contains(rewritten, "rem.biz_move-2026-04-05") {
		t.Fatalf("did not expect source IP/cert in rewritten command:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "'site_aliases=*.rem.biz www.rem.biz'") {
		t.Fatalf("expected aliases to be space-separated in rewritten command:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "site_aliases=*.rem.biz,") {
		t.Fatalf("did not expect comma-separated aliases in rewritten command:\n%s", rewritten)
	}
}

func TestInactiveSSLCertsFromOutputBuildsBulkDeleteCommand(t *testing.T) {
	t.Parallel()

	output := strings.Join([]string{
		"owner=remsklad key=remsklad%#%api.remsklad.online name=api.remsklad.online info=api.remsklad.online www.api.remsklad.online type=ssl_selfsigned valid_after=2027-04-25 cert_type=ssl_selfsigned active=off",
		"owner=simple key=simple%#%interabiz.ru name=interabiz.ru info=interabiz.ru www.interabiz.ru type=ssl_selfsigned valid_after=2027-04-25 cert_type=ssl_selfsigned active=off",
		"owner=www-root key=www-root%#%active.example.com name=active.example.com info=active.example.com type=ssl_selfsigned active=on",
	}, "\n")

	certs := inactiveSSLCertsFromOutput(output)
	if len(certs) != 2 {
		t.Fatalf("expected two inactive SSL certificates, got %#v", certs)
	}

	command := buildUnusedSSLCertDeleteCommand(certs)
	want := "/usr/local/mgr5/sbin/mgrctl -m ispmgr sslcert.delete sok=ok 'elid=remsklad%#%api.remsklad.online, simple%#%interabiz.ru' elname=api.remsklad.online"
	if command != want {
		t.Fatalf("unexpected sslcert.delete command:\n got: %s\nwant: %s", command, want)
	}
}

func TestBuildUnusedSSLCertDeleteCommandDoesNotAddCommaForSingleCert(t *testing.T) {
	t.Parallel()

	command := buildUnusedSSLCertDeleteCommand([]inactiveSSLCert{{Key: "www-root%#%api.rem.biz", Name: "api.rem.biz"}})
	want := "/usr/local/mgr5/sbin/mgrctl -m ispmgr sslcert.delete sok=ok 'elid=www-root%#%api.rem.biz' elname=api.rem.biz"
	if command != want {
		t.Fatalf("unexpected single sslcert.delete command:\n got: %s\nwant: %s", command, want)
	}
}

func TestCleanupUnusedSSLCertsListsAndDeletesInactiveCerts(t *testing.T) {
	t.Parallel()

	var calls []string
	runner := &remoteRunner{
		cfg:    Config{LogLevel: "off"},
		ui:     &UI{out: &bytes.Buffer{}, err: &bytes.Buffer{}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			calls = append(calls, command)
			if command == "/usr/local/mgr5/sbin/mgrctl -m ispmgr sslcert" {
				return "owner=www-root key=www-root%#%api.rem.biz name=api.rem.biz active=off\n", nil
			}
			if strings.Contains(command, "sslcert.delete") {
				return "OK\n", nil
			}
			t.Fatalf("unexpected command %q", command)
			return "", nil
		},
	}

	if err := runner.cleanupUnusedSSLCerts(context.Background()); err != nil {
		t.Fatalf("cleanupUnusedSSLCerts() returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected sslcert list and delete commands, got %#v", calls)
	}
	if !strings.Contains(calls[1], "sslcert.delete") {
		t.Fatalf("expected second command to delete inactive certs, got %#v", calls)
	}
}

func TestValidateLicenseOnlyRejectsLiteForMoreThanTenWebDomains(t *testing.T) {
	t.Parallel()

	if err := validateLicense(map[string]string{"panel_name": "ispmanager Lite"}, 10, true); err != nil {
		t.Fatalf("expected Lite to allow 10 web domains, got %v", err)
	}
	if err := validateLicense(map[string]string{"panel_name": "ispmanager Pro"}, 11, true); err != nil {
		t.Fatalf("expected Pro to allow more than 10 web domains, got %v", err)
	}
	if err := validateLicense(map[string]string{"panel_name": "ispmanager Lite"}, 11, false); err != nil {
		t.Fatalf("expected non-webdomain scope to skip Lite webdomain limit, got %v", err)
	}
	if err := validateLicense(map[string]string{"panel_name": "ispmanager Lite"}, 11, true); err == nil {
		t.Fatalf("expected Lite to be rejected for more than 10 web domains")
	}
}

func TestVerifyAltPHPComponentsRetriesMissingModule(t *testing.T) {
	t.Parallel()

	inspectCount := 0
	retryCount := 0
	runner := &remoteRunner{
		cfg:    Config{LogLevel: "info"},
		ui:     &UI{out: &bytes.Buffer{}, err: &bytes.Buffer{}},
		logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			switch {
			case strings.Contains(command, "feature.edit") && strings.Contains(command, "elid=altphp74") && strings.Contains(command, "out=text"):
				inspectCount++
				if inspectCount == 1 {
					return "package_ispphp74_fpm=on\npackage_ispphp74_mod_apache=off\n", nil
				}
				return "package_ispphp74_fpm=on\npackage_ispphp74_mod_apache=on\n", nil
			case strings.Contains(command, "feature.edit") && strings.Contains(command, "elid=altphp74") && strings.Contains(command, "packagegroup_altphp74gr=ispphp74"):
				if !strings.Contains(command, "package_ispphp74_fpm=off") {
					t.Fatalf("expected retry command to disable absent fpm package, got %q", command)
				}
				if !strings.Contains(command, "package_ispphp74_mod_apache=on") {
					t.Fatalf("expected retry command to enable source mod_apache package, got %q", command)
				}
				if !strings.Contains(command, "package_ispphp74_lsapi=off") {
					t.Fatalf("expected retry command to disable absent lsapi package, got %q", command)
				}
				retryCount++
				return "OK\n", nil
			case strings.Contains(command, "mgrctl -m ispmgr feature"):
				return "name=altphp74 dname=altphp74 content=PHP 7.4 Apache module, PHP 7.4 PHP-FPM, PHP 7.4 common featlist=PHP 7.4 Apache module, PHP 7.4 PHP-FPM, PHP 7.4 common active=on promo=off\n", nil
			default:
				return "", nil
			}
		},
	}

	err := runner.verifyAltPHPComponents(context.Background(), "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp74' 'elname=PHP 7.4 Apache module, PHP 7.4 PHP-FPM, PHP 7.4 common'", []string{"ispphp74_mod_apache"}, "Ubuntu 24.04")
	if err != nil {
		t.Fatalf("verifyAltPHPComponents() returned error: %v", err)
	}
	if retryCount != 1 {
		t.Fatalf("expected one retry feature.edit command, got %d", retryCount)
	}
	if inspectCount < 2 {
		t.Fatalf("expected post-retry inspection, got %d inspections", inspectCount)
	}
}

func TestBuildAltPHPEditCommandSetsSourceComponentsOnAndOthersOff(t *testing.T) {
	t.Parallel()

	command := buildAltPHPEditCommand(altPHPComponentExpectation{
		Version: "72",
		Fields:  []string{"package_ispphp72_fpm", "package_ispphp72_lsapi"},
	})
	for _, want := range []string{
		"feature.edit",
		"sok=ok",
		"elid=altphp72",
		"packagegroup_altphp72gr=ispphp72",
		"package_ispphp72_fpm=on",
		"package_ispphp72_mod_apache=off",
		"package_ispphp72_lsapi=on",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected %q in command %q", want, command)
		}
	}
}

func TestAltPHPComponentExpectationsUseOnlySourcePackages(t *testing.T) {
	t.Parallel()

	expectations := altPHPComponentExpectations(
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp72, altphp74' 'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'",
		[]string{"ispphp72", "ispphp72_lsapi", "ispphp74", "ispphp74_fpm", "ispphp74_mod_apache"},
	)
	if len(expectations) != 2 {
		t.Fatalf("expected two altphp expectations, got %#v", expectations)
	}
	fields := map[string][]string{}
	for _, expectation := range expectations {
		fields[expectation.Version] = expectation.Fields
	}
	if !containsString(fields["72"], "package_ispphp72_lsapi") {
		t.Fatalf("expected package_ispphp72_lsapi to be checked, got %#v", fields["72"])
	}
	if containsString(fields["72"], "package_ispphp72_fpm") || containsString(fields["72"], "package_ispphp72_mod_apache") {
		t.Fatalf("did not expect absent source package components to be checked for 72, got %#v", fields["72"])
	}
	if !containsString(fields["74"], "package_ispphp74_fpm") || !containsString(fields["74"], "package_ispphp74_mod_apache") {
		t.Fatalf("expected fpm and mod_apache to be checked for 74, got %#v", fields["74"])
	}
	if containsString(fields["74"], "package_ispphp74_lsapi") {
		t.Fatalf("did not expect absent lsapi source package to be checked for 74, got %#v", fields["74"])
	}
}

func TestPackageStepsFromCommandGroupsKeepsAltPHPExpectedPackages(t *testing.T) {
	t.Parallel()

	groups := []CommandGroup{{
		Title: "packages (altphp)",
		Commands: []string{
			"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp72, altphp74' 'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'",
		},
	}}
	steps := packageStepsFromCommandGroups(groups, []Package{
		{Name: "ispphp72"},
		{Name: "ispphp72_lsapi"},
		{Name: "ispphp74"},
		{Name: "ispphp74_fpm"},
		{Name: "ispphp74_mod_apache"},
	})
	if len(steps) != 1 {
		t.Fatalf("expected one package step, got %#v", steps)
	}
	for _, want := range []string{"ispphp72", "ispphp72_lsapi", "ispphp74", "ispphp74_fpm", "ispphp74_mod_apache"} {
		if !containsString(steps[0].ExpectedPackages, want) {
			t.Fatalf("expected %s in ExpectedPackages, got %#v", want, steps[0].ExpectedPackages)
		}
	}
	if containsString(steps[0].ExpectedPackages, "ispphp74_lsapi") {
		t.Fatalf("did not expect absent source package ispphp74_lsapi, got %#v", steps[0].ExpectedPackages)
	}
}

func TestDestinationPackageStepsForceDoesNotBypassSatisfiedPackageFiltering(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{
		cfg: Config{
			DestScope: "all",
			Force:     true,
			LogLevel:  "info",
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			if strings.Contains(command, "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature") {
				return "name=fail2ban dname=fail2ban content=fail2ban 1.1.0 featlist=fail2ban 1.1.0 active=on promo=off type=recommended\n", nil
			}
			return "", nil
		},
	}

	steps, warnings, err := runner.destinationPackageSteps(context.Background(), SourceData{
		Packages: []Package{
			{ID: "1", Name: "fail2ban"},
			{ID: "2", Name: "nginx"},
		},
	}, map[string]string{"os": "Ubuntu 24.04", "panel_name": "ispmanager Host"})
	if err != nil {
		t.Fatalf("destinationPackageSteps() returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	commands := make([]string, 0, len(steps))
	for _, step := range steps {
		commands = append(commands, step.Command)
	}
	joined := strings.Join(commands, "\n")
	if strings.Contains(joined, "package_fail2ban=on") {
		t.Fatalf("--force must not push satisfied fail2ban package command:\n%s", joined)
	}
	if !strings.Contains(joined, "package_nginx=on") {
		t.Fatalf("expected missing nginx package command to remain:\n%s", joined)
	}
}

func TestAltPHPComponentMismatchMessageIncludesInspectCommand(t *testing.T) {
	t.Parallel()

	message := altPHPComponentMismatch{
		Version: "52",
		Fields:  []string{"package_ispphp52_fpm=on"},
	}.String()
	want := "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=altphp52 missing package_ispphp52_fpm=on"
	if message != want {
		t.Fatalf("unexpected mismatch message:\n got: %s\nwant: %s", message, want)
	}
}

func TestFilterExistingEntityCommandsRequiresOverwriteToPushExistingEntities(t *testing.T) {
	t.Parallel()

	commands := []string{
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=alice sok=ok",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=bob sok=ok",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr domain.edit name=example.com sok=ok",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit site_name=example.com sok=ok",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit site_name=new.example.com sok=ok",
	}
	inventory := &remoteInventory{
		users:    map[string]struct{}{"alice": {}},
		dnsZones: map[string]struct{}{"example.com": {}},
		webSites: map[string]struct{}{"example.com": {}},
	}

	filtered := filterExistingEntityCommands(commands, inventory, false)
	joined := strings.Join(filtered, "\n")
	for _, unexpected := range []string{"name=alice", "domain.edit name=example.com", "site_name=example.com "} {
		if strings.Contains(joined, unexpected) {
			t.Fatalf("did not expect existing entity command %q without --overwrite:\n%s", unexpected, joined)
		}
	}
	for _, expected := range []string{"name=bob", "site_name=new.example.com"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected missing entity command %q to remain:\n%s", expected, joined)
		}
	}

	overwriteFiltered := filterExistingEntityCommands(commands, inventory, true)
	if len(overwriteFiltered) != len(commands) {
		t.Fatalf("expected --overwrite to keep all commands, got %#v", overwriteFiltered)
	}
}

func TestFilterExistingEntityCommandsKeepsExistingMySQLForPasswordSync(t *testing.T) {
	t.Parallel()

	commands := []string{
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=MySQL sok=ok",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=mariadb-10.11 sok=ok",
	}
	inventory := &remoteInventory{
		dbServers: map[string]struct{}{
			"mysql":         {},
			"mariadb-10.11": {},
		},
	}

	filtered := filterExistingEntityCommands(commands, inventory, false)
	joined := strings.Join(filtered, "\n")
	if !strings.Contains(joined, "name=MySQL") {
		t.Fatalf("expected existing MySQL command to remain for password sync:\n%s", joined)
	}
	if strings.Contains(joined, "mariadb-10.11") {
		t.Fatalf("did not expect existing non-MySQL db server command without --overwrite:\n%s", joined)
	}
}

func TestEntityCommandExistsInInventoryCanVerifyExistingMySQL(t *testing.T) {
	t.Parallel()

	command := "/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=MySQL sok=ok"
	inventory := &remoteInventory{
		dbServers: map[string]struct{}{"mysql": {}},
	}

	if entityCommandExistsInInventory(command, inventory, true) {
		t.Fatalf("expected filter mode to keep existing MySQL command for password sync")
	}
	if !entityCommandExistsInInventory(command, inventory, false) {
		t.Fatalf("expected visibility mode to verify existing MySQL command")
	}
}

func TestDatabaseServerEditDoesNotUseEntityVisibilityPostCheck(t *testing.T) {
	t.Parallel()

	command := "/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=mariadb-10.11 sok=ok"
	if isInventoryBackedEntityCommand(command) {
		t.Fatalf("db.server.edit must not use post-push remote inventory visibility checks")
	}
}

func TestEntityProgressCheckOptionsUseShortTimeouts(t *testing.T) {
	t.Parallel()

	options := entityProgressCheckOptions("/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit site_name=example.com sok=ok")
	if options.DiscoveryTimeout != entityProgressDiscoveryTimeout {
		t.Fatalf("expected entity discovery timeout %s, got %s", entityProgressDiscoveryTimeout, options.DiscoveryTimeout)
	}
	if options.CompletionTimeout != entityProgressCompletionTimeout {
		t.Fatalf("expected entity completion timeout %s, got %s", entityProgressCompletionTimeout, options.CompletionTimeout)
	}
	if !options.AllowIdleCompletion {
		t.Fatalf("expected entity progress to allow idle completion")
	}
}

func TestDatabaseServerProgressCheckOptionsUseLongerCompletionTimeout(t *testing.T) {
	t.Parallel()

	options := entityProgressCheckOptions("/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=mariadb-10.11 sok=ok")
	if options.DiscoveryTimeout != dbServerProgressDiscoveryTimeout {
		t.Fatalf("expected db.server discovery timeout %s, got %s", dbServerProgressDiscoveryTimeout, options.DiscoveryTimeout)
	}
	if options.CompletionTimeout != dbServerProgressCompletionTimeout {
		t.Fatalf("expected db.server completion timeout %s, got %s", dbServerProgressCompletionTimeout, options.CompletionTimeout)
	}
	if !options.AllowIdleCompletion {
		t.Fatalf("expected db.server progress to allow idle completion")
	}
}

func TestWaitEntityCommandVisibleReloadsDestinationInventory(t *testing.T) {
	calls := 0
	waits := make([]time.Duration, 0, 1)
	runner := &remoteRunner{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		inventory: &remoteInventory{},
		loadInventoryOverride: func(ctx context.Context) (*remoteInventory, error) {
			calls++
			return &remoteInventory{
				webSites: map[string]struct{}{"telemaster.spb.ru": {}},
			}, nil
		},
	}

	err := runner.waitEntityCommandVisibleWithWait(
		context.Background(),
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit site_name=telemaster.spb.ru sok=ok",
		func(ctx context.Context, interval time.Duration) error {
			waits = append(waits, interval)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("waitEntityCommandVisible() returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one destination inventory reload, got %d", calls)
	}
	if len(waits) != 1 || waits[0] != entityVisibilityPollInterval {
		t.Fatalf("expected first inventory reload after %s, got %#v", entityVisibilityPollInterval, waits)
	}
}

func TestConfirmPanelInstallForceStillAsksUser(t *testing.T) {
	originalAsk := askYesNoWithColorHook
	t.Cleanup(func() {
		askYesNoWithColorHook = originalAsk
	})

	asked := false
	askYesNoWithColorHook = func(question string, defaultNo bool, color string) (bool, error) {
		asked = true
		if question != "ispmanager was not found on destination server. Install it?" {
			t.Fatalf("unexpected question %q", question)
		}
		if !defaultNo {
			t.Fatalf("expected default answer to be no")
		}
		return false, nil
	}

	install, err := confirmPanelInstall(Config{Force: true})
	if err != nil {
		t.Fatalf("confirmPanelInstall() returned error: %v", err)
	}
	if install {
		t.Fatalf("expected --force without -y to keep user confirmation result")
	}
	if !asked {
		t.Fatalf("expected --force without -y to ask before installing panel")
	}
}

func TestConfirmPanelInstallAutoYesSkipsUserQuestion(t *testing.T) {
	originalAsk := askYesNoWithColorHook
	t.Cleanup(func() {
		askYesNoWithColorHook = originalAsk
	})

	askYesNoWithColorHook = func(question string, defaultNo bool, color string) (bool, error) {
		t.Fatalf("did not expect prompt when -y is set")
		return false, nil
	}

	install, err := confirmPanelInstall(Config{AutoYes: true})
	if err != nil {
		t.Fatalf("confirmPanelInstall() returned error: %v", err)
	}
	if !install {
		t.Fatalf("expected -y to allow automatic panel install")
	}
}

func TestWarnOverwriteBlockAddsBlankLinesAtInfo(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnOverwriteBlock("user www-root already exists on remote side, skipped. Run again with --overwrite to modify it.")

	got := stripANSI(out.String())
	want := "\nuser www-root already exists on remote side, skipped. Run again with --overwrite to modify it.\n\n"
	if got != want {
		t.Fatalf("warnOverwriteBlock() info output = %q, want %q", got, want)
	}
}

func TestWarnOverwriteBlockDoesNotAddBlankLinesAtWarn(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "warn"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnOverwriteBlock("user www-root already exists on remote side, skipped. Run again with --overwrite to modify it.")

	got := stripANSI(out.String())
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(strings.TrimSuffix(got, "\n"), "\n") {
		t.Fatalf("warnOverwriteBlock() warn output should not have extra blank lines, got %q", got)
	}
	want := "user www-root already exists on remote side, skipped. Run again with --overwrite to modify it.\n"
	if got != want {
		t.Fatalf("warnOverwriteBlock() warn output = %q, want %q", got, want)
	}
}

func TestWarnPackageWarningAddsTrailingBlankLineAtInfoForDockerSkip(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnPackageWarning(`destination panel edition "ispmanager Lite" does not support Docker, package_docker command was skipped.`)

	got := stripANSI(out.String())
	want := "destination panel edition \"ispmanager Lite\" does not support Docker, package_docker command was skipped.\n\n"
	if got != want {
		t.Fatalf("warnPackageWarning() info output = %q, want %q", got, want)
	}
}

func TestWarnFullLineColorsDestinationMemoryMismatchAsWarning(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnFullLine("destination memory (1.91 GiB) is more than 2 GiB lower than source memory (11.60 GiB).")

	got := out.String()
	if !strings.Contains(got, colorYellow) {
		t.Fatalf("expected warning output to contain yellow color, got %q", got)
	}
	if !strings.Contains(got, "destination memory (1.91 GiB) is more than 2 GiB lower than source memory (11.60 GiB).") {
		t.Fatalf("expected warning text in output, got %q", got)
	}
}

func TestRecordSummarySuccessDeduplicates(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{}
	runner.recordSummarySuccess("adding user alice: OK")
	runner.recordSummarySuccess("adding user alice: OK")

	if len(runner.summarySuccesses) != 1 {
		t.Fatalf("expected one deduplicated summary success, got %#v", runner.summarySuccesses)
	}
}

func TestRecordFailureStoresLastPushCommand(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{}
	runner.printLaunchedCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit sok=ok")
	runner.recordPushOutput("ERROR value(site_name): invalid domain\n")
	runner.recordFailure("adding web site rem.biz", errors.New("Process exited with status 1"))

	if len(runner.failures) != 1 {
		t.Fatalf("expected one failure, got %#v", runner.failures)
	}
	if runner.failures[0].PushCommand != "/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit sok=ok" {
		t.Fatalf("expected failure push command to be stored, got %#v", runner.failures[0])
	}
	if runner.failures[0].PushOutput != "ERROR value(site_name): invalid domain" {
		t.Fatalf("expected failure push output to be stored, got %#v", runner.failures[0])
	}
}

func TestPrintLaunchedCommandMirrorsToLogFile(t *testing.T) {
	t.Parallel()

	logFile := filepath.Join(t.TempDir(), "ispdb.log")
	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info", LogFile: logFile},
		ui:  &UI{out: &out, err: &out},
	}

	runner.printLaunchedCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.update sok=ok")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read mirrored log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `level=INFO`) {
		t.Fatalf("expected mirrored info level in log, got %q", got)
	}
	if !strings.Contains(got, `msg="pushing command: /usr/local/mgr5/sbin/mgrctl -m ispmgr feature.update sok=ok"`) {
		t.Fatalf("expected mirrored pushing command in log, got %q", got)
	}
}

func TestRemoteCommandTimeoutAppliesOnlyToMgrctl(t *testing.T) {
	if remoteCommandTimeout("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature", false) != defaultRemoteMgrctlTimeout {
		t.Fatalf("expected mgrctl timeout to be %s", defaultRemoteMgrctlTimeout)
	}
	if remoteCommandTimeout("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature", true) != forceRemoteCommandTimeout {
		t.Fatalf("expected forced mgrctl timeout to be %s", forceRemoteCommandTimeout)
	}
	aptCommand := "bash -lc 'apt-get update && apt-get install -y wget'"
	if remoteCommandTimeout(aptCommand, false) != 0 {
		t.Fatalf("expected non-mgrctl command to have no forced timeout")
	}
	if remoteCommandTimeout(aptCommand, true) != forceRemoteCommandTimeout {
		t.Fatalf("expected forced non-mgrctl command timeout to be %s", forceRemoteCommandTimeout)
	}
}

func TestIsDpkgCacheLockLineDetectsAnyAptDpkgLockHolder(t *testing.T) {
	line := "Waiting for cache lock: Could not get lock /var/lib/dpkg/lock-frontend. It is held by process 1687 (unattended-upgr)..."
	for _, item := range []string{
		line,
		"Waiting for cache lock: Could not get lock /var/lib/dpkg/lock-frontend. It is held by process 42 (apt-get)...",
		"Waiting for cache lock: package manager is busy",
	} {
		if !isDpkgCacheLockLine(item) {
			t.Fatalf("expected apt/dpkg cache lock line to be detected: %q", item)
		}
	}
	if isDpkgCacheLockLine("ERROR value(packagegroup_apache): The 'Apache' field has invalid value.") {
		t.Fatalf("expected unrelated package error line to stay ignored")
	}
}

func TestFeatureStepTimeoutErrorMentionsDpkgLockWhenDetected(t *testing.T) {
	t.Parallel()

	err := featureStepTimeoutError(packageSyncStep{Title: "packages (web)"}, []string{"nginx"}, true)
	got := stripANSI(err.Error())
	if !strings.Contains(got, "feature step packages (web) did not finish in time") {
		t.Fatalf("expected feature timeout message, got %q", got)
	}
	if !strings.Contains(got, "apt/dpkg cache lock was detected in pkg.log and wait timed out") {
		t.Fatalf("expected dpkg lock timeout context, got %q", got)
	}
}

func TestNormalizeRemoteCommandErrorDetectsTimeoutAndDisconnect(t *testing.T) {
	timeoutErr := normalizeRemoteCommandError("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature", context.DeadlineExceeded)
	if !strings.Contains(timeoutErr.Error(), "timed out or stalled under load") {
		t.Fatalf("expected timeout normalization, got %v", timeoutErr)
	}

	disconnectErr := normalizeRemoteCommandError("cmd", errors.New("read tcp 1.2.3.4: broken pipe"))
	if !strings.Contains(disconnectErr.Error(), "server may have rebooted") {
		t.Fatalf("expected disconnect normalization, got %v", disconnectErr)
	}
}

func TestRunWithTraceLogsDBServerTextProbeFailureAsDebug(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	runner := &remoteRunner{
		logger: logger,
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			return "", errors.New("Process exited with status 1")
		},
	}

	_, _ = runner.run(context.Background(), "/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit elid=mysql-8.4 out=text")

	got := logs.String()
	if strings.Contains(got, `level=ERROR msg="ssh command failed"`) {
		t.Fatalf("expected no error-level ssh command failure for db.server text probe, got %q", got)
	}
	if !strings.Contains(got, `level=DEBUG msg="ssh probe command failed"`) {
		t.Fatalf("expected debug-level probe failure log, got %q", got)
	}
}

func TestSanitizeRemoteConsoleOutputSuppressesBenignProcSysGrepWarnings(t *testing.T) {
	input := strings.Join([]string{
		"grep: /proc/sys/net/ipv4/route/flush: Permission denied",
		"grep: /proc/sys/net/ipv6/conf/all/stable_secret: Input/output error",
		"normal line",
	}, "\n")

	cleaned, suppressed := sanitizeRemoteConsoleOutput(input)
	if suppressed != 2 {
		t.Fatalf("expected 2 suppressed lines, got %d", suppressed)
	}
	if strings.Contains(cleaned, "/proc/sys/") {
		t.Fatalf("expected /proc/sys grep warnings to be removed, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "normal line") {
		t.Fatalf("expected regular output to remain, got %q", cleaned)
	}
}

func TestConnectRemoteRunnerUsesHooksAndAnnouncesConnection(t *testing.T) {
	originalConnectSSHHook := connectSSHHook
	originalSFTPClientHook := newSFTPClientHook
	t.Cleanup(func() {
		connectSSHHook = originalConnectSSHHook
		newSFTPClientHook = originalSFTPClientHook
	})

	connectSSHHook = func(cfg Config) (*ssh.Client, error) {
		return nil, nil
	}
	newSFTPClientHook = func(client *ssh.Client, opts ...sftp.ClientOption) (*sftp.Client, error) {
		return nil, nil
	}

	var out bytes.Buffer
	runner, err := connectRemoteRunner(&UI{out: &out, err: &out}, slog.Default(), Config{
		DestHost: "192.0.2.10",
		LogLevel: "info",
	}, true)
	if err != nil {
		t.Fatalf("connectRemoteRunner() returned error: %v", err)
	}
	if runner == nil {
		t.Fatalf("connectRemoteRunner() returned nil runner")
	}

	got := stripANSI(out.String())
	if !strings.Contains(got, "connecting: 192.0.2.10\n") {
		t.Fatalf("expected connecting line, got %q", got)
	}
	if !strings.Contains(got, "connecting: OK\n") {
		t.Fatalf("expected success line, got %q", got)
	}
}

func TestPrepareExistingMySQLPasswordSyncBuildsChangePasswordCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
		inventory: &remoteInventory{
			dbServers: map[string]struct{}{"mysql": {}},
		},
	}

	command, skipReason, err := runner.prepareExistingMySQLPasswordSync(context.Background(), SourceData{
		DBServers: []DBServer{{Name: "MySQL", Host: "localhost", Username: "root", Password: "secret123"}},
	}, "/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=MySQL sok=ok host=localhost password=generated remote_access=off type=mysql username=root")
	if err != nil {
		t.Fatalf("prepareExistingMySQLPasswordSync() returned error: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("expected no skipReason, got %q", skipReason)
	}
	for _, want := range []string{
		"db.server.edit",
		"elid=MySQL",
		"name=MySQL",
		"host=localhost",
		"username=root",
		"password=secret123",
		"change_password=on",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected adjusted command to contain %q, got %q", want, command)
		}
	}
	got := stripANSI(out.String())
	if !strings.Contains(got, "DB server MySQL already exists on remote side, syncing password from source.") {
		t.Fatalf("expected info message, got %q", got)
	}
}

func TestPrepareExistingMySQLPasswordSyncSkipsWhenSourcePasswordUnavailable(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{
		inventory: &remoteInventory{
			dbServers: map[string]struct{}{"mysql": {}},
		},
	}

	command, skipReason, err := runner.prepareExistingMySQLPasswordSync(context.Background(), SourceData{
		DBServers: []DBServer{{Name: "MySQL", Host: "localhost", Username: "root", Password: ""}},
	}, "/usr/local/mgr5/sbin/mgrctl -m ispmgr db.server.edit name=MySQL sok=ok host=localhost password=generated remote_access=off type=mysql username=root")
	if err != nil {
		t.Fatalf("prepareExistingMySQLPasswordSync() returned error: %v", err)
	}
	if command == "" {
		t.Fatalf("expected original command to be returned")
	}
	if !strings.Contains(skipReason, "source password was not available") {
		t.Fatalf("expected source password skip reason, got %q", skipReason)
	}
}

func TestPrintMonitoringCommandAddsReadableInfoOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.printMonitoringCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")

	got := stripANSI(out.String())
	want := "monitoring command: /usr/local/mgr5/sbin/mgrctl -m ispmgr feature\n\n"
	if got != want {
		t.Fatalf("printMonitoringCommand() info output = %q, want %q", got, want)
	}
}

func TestPrintMonitoringCommandShowsFilesOnDebug(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "debug"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.printMonitoringCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")

	got := stripANSI(out.String())
	if !strings.Contains(got, "monitoring command: /usr/local/mgr5/sbin/mgrctl -m ispmgr feature\n") {
		t.Fatalf("expected monitoring command, got %q", got)
	}
	if !strings.Contains(got, "monitoring files: /usr/local/mgr5/var/ispmgr.log, /usr/local/mgr5/var/pkg.log\n") {
		t.Fatalf("expected monitoring files, got %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected trailing blank line, got %q", got)
	}
}

func TestPruneFeatureStepKeepsOpenLiteSpeedPHPWhenOpenLiteSpeedIsEnabled(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{
		logger: slog.Default(),
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			if strings.Contains(command, "feature.edit") && strings.Contains(command, "out=text") {
				return strings.Join([]string{
					"title=Web-server (WWW)",
					"package_nginx=on",
					"package_logrotate=on",
					"package_awstats=on",
					"package_php=on",
					"package_php-fpm=on",
					"elid=web",
					"package_openlitespeed=off",
					"package_phpcomposer=off",
					"package_nginx_modsecurity=off",
					"package_apache_modsecurity=off",
					"package_openlitespeed_modsecurity=off",
					"packagegroup_apache=apache-itk-ubuntu",
				}, "\n"), nil
			}
			return "", nil
		},
	}

	step := packageSyncStep{
		Title:            "packages (web)",
		Feature:          "web",
		Command:          "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=web package_apache_modsecurity=off package_awstats=on package_logrotate=on package_nginx=off package_nginx_modsecurity=off package_openlitespeed=on package_openlitespeed-php=on package_openlitespeed_modsecurity=off package_php=off package_php-fpm=off package_phpcomposer=off packagegroup_apache=turn_off",
		ExpectedPackages: []string{"openlitespeed", "openlitespeed-php"},
	}

	pruned, err := runner.pruneFeatureStep(context.Background(), step)
	if err != nil {
		t.Fatalf("pruneFeatureStep() returned error: %v", err)
	}
	if !strings.Contains(pruned.Command, "package_openlitespeed-php=on") {
		t.Fatalf("expected openlitespeed-php param to remain with package_openlitespeed=on, got %q", pruned.Command)
	}
	if !containsString(pruned.ExpectedPackages, "openlitespeed") {
		t.Fatalf("expected openlitespeed to remain in expected packages, got %#v", pruned.ExpectedPackages)
	}
	if !containsString(pruned.ExpectedPackages, "openlitespeed-php") {
		t.Fatalf("expected openlitespeed-php to remain in expected packages with package_openlitespeed=on, got %#v", pruned.ExpectedPackages)
	}
}

func TestPruneFeatureStepKeepsSupportedWebOffParamsAgainstDestinationForm(t *testing.T) {
	t.Parallel()

	runner := &remoteRunner{
		logger: slog.Default(),
		runOverride: func(ctx context.Context, command string, trace bool) (string, error) {
			if strings.Contains(command, "feature.edit") && strings.Contains(command, "out=text") {
				return strings.Join([]string{
					"title=Web-server (WWW)",
					"package_nginx=on",
					"package_logrotate=on",
					"package_awstats=on",
					"package_php=on",
					"package_php-fpm=on",
					"package_pagespeed=off",
					"hide_low_ram_banner=on",
					"elid=web",
					"package_openlitespeed=off",
					"hide_pagespeed=",
					"package_phpcomposer=off",
					"package_nginx_modsecurity=off",
					"package_apache_modsecurity=off",
					"package_openlitespeed_modsecurity=off",
					"packagegroup_apache=apache-itk-ubuntu",
					"saved_filters=",
				}, "\n"), nil
			}
			return "", nil
		},
	}

	step := packageSyncStep{
		Title:   "packages (web)",
		Feature: "web",
		Command: "/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.edit sok=ok elid=web package_apache_modsecurity=off package_awstats=on package_logrotate=on package_nginx=off package_nginx_modsecurity=off package_nginx_pagespeed=off package_openlitespeed=on package_openlitespeed-php=on package_openlitespeed_modsecurity=off package_pagespeed=off package_php=off package_php-fpm=off package_phpcomposer=off packagegroup_apache=turn_off",
	}

	pruned, err := runner.pruneFeatureStep(context.Background(), step)
	if err != nil {
		t.Fatalf("pruneFeatureStep() returned error: %v", err)
	}

	for _, want := range []string{
		"package_awstats=on",
		"package_logrotate=on",
		"package_nginx=off",
		"package_openlitespeed=on",
		"package_openlitespeed-php=on",
		"package_php=off",
		"package_php-fpm=off",
		"package_phpcomposer=off",
		"packagegroup_apache=turn_off",
	} {
		if !strings.Contains(pruned.Command, want) {
			t.Fatalf("expected pruned web command to contain %q, got %q", want, pruned.Command)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestParseRemoteSwapStateIgnoresZRAMAndDetectsSwapfile(t *testing.T) {
	t.Parallel()

	state := parseRemoteSwapState(strings.Join([]string{
		"Filename\t\t\tType\t\tSize\t\tUsed\t\tPriority",
		"/dev/zram0                               partition\t524284\t0\t100",
		"/swapfile                                file\t2097148\t0\t10",
	}, "\n"), "/swapfile                                 none                    swap    sw,pri=10              0 0\n", true)

	if state.HasOtherNonZRAMSwap {
		t.Fatalf("expected zram to be ignored, got %+v", state)
	}
	if !state.SwapfileActive {
		t.Fatalf("expected /swapfile to be active, got %+v", state)
	}
	if !state.FstabHasSwapfile {
		t.Fatalf("expected /swapfile fstab entry, got %+v", state)
	}
	if !state.SwapfileExists {
		t.Fatalf("expected /swapfile existence flag, got %+v", state)
	}
}

func TestHasSwapfileFstabEntryAcceptsExistingSwapfileLineWithDifferentOptions(t *testing.T) {
	t.Parallel()

	line := "/swapfile none swap defaults,pri=5 0 0"
	if !hasSwapfileFstabEntry(line) {
		t.Fatalf("expected swapfile fstab line to be detected: %q", line)
	}
}

func TestAppendSwapfileFstabLineDoesNotDuplicateExistingEntry(t *testing.T) {
	t.Parallel()

	original := "/swapfile none swap defaults,pri=5 0 0\n"
	updated, changed := appendSwapfileFstabLine(original)
	if changed {
		t.Fatalf("expected existing swapfile line not to be duplicated")
	}
	if updated != original {
		t.Fatalf("expected unchanged fstab content, got %q", updated)
	}
}

func TestAppendSwapfileFstabLineAppendsManagedLineOnce(t *testing.T) {
	t.Parallel()

	updated, changed := appendSwapfileFstabLine("UUID=abc / ext4 defaults 0 1\n")
	if !changed {
		t.Fatalf("expected swapfile line to be appended")
	}
	if !strings.Contains(updated, swapfileFstabLine+"\n") {
		t.Fatalf("expected managed swapfile line in output, got %q", updated)
	}
}

func TestWarnOnMemoryMismatchCreatesSwapfileWhenMemoryIsLowAndNoSwapExists(t *testing.T) {
	originalLocalMemoryTotalBytesHook := localMemoryTotalBytesHook
	originalRemoteMemoryTotalBytesHook := remoteMemoryTotalBytesHook
	originalRemoteSwapStateHook := remoteSwapStateHook
	originalEnsureSwapfileHook := ensureSwapfileHook
	t.Cleanup(func() {
		localMemoryTotalBytesHook = originalLocalMemoryTotalBytesHook
		remoteMemoryTotalBytesHook = originalRemoteMemoryTotalBytesHook
		remoteSwapStateHook = originalRemoteSwapStateHook
		ensureSwapfileHook = originalEnsureSwapfileHook
	})

	const gib = int64(1024 * 1024 * 1024)
	localMemoryTotalBytesHook = func() (int64, error) { return 4 * gib, nil }
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) { return int64(1900 * 1024 * 1024), nil }
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return remoteSwapState{}, nil
	}

	called := false
	ensureSwapfileHook = func(r *remoteRunner, ctx context.Context, state remoteSwapState) error {
		called = true
		return nil
	}

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	if err := runner.warnOnMemoryMismatch(context.Background()); err != nil {
		t.Fatalf("warnOnMemoryMismatch() returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected ensureSwapfileHook to be called")
	}
	got := stripANSI(out.String())
	if !strings.Contains(got, "destination memory is low:") {
		t.Fatalf("expected low-memory warning, got %q", got)
	}
	if !strings.Contains(got, "creating swapfile and enabling it: OK") {
		t.Fatalf("expected successful swapfile action, got %q", got)
	}
}

func TestWarnOnMemoryMismatchSkipsSwapCreationWhenOtherSwapExists(t *testing.T) {
	originalLocalMemoryTotalBytesHook := localMemoryTotalBytesHook
	originalRemoteMemoryTotalBytesHook := remoteMemoryTotalBytesHook
	originalRemoteSwapStateHook := remoteSwapStateHook
	originalEnsureSwapfileHook := ensureSwapfileHook
	t.Cleanup(func() {
		localMemoryTotalBytesHook = originalLocalMemoryTotalBytesHook
		remoteMemoryTotalBytesHook = originalRemoteMemoryTotalBytesHook
		remoteSwapStateHook = originalRemoteSwapStateHook
		ensureSwapfileHook = originalEnsureSwapfileHook
	})

	const gib = int64(1024 * 1024 * 1024)
	localMemoryTotalBytesHook = func() (int64, error) { return 4 * gib, nil }
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) { return int64(1900 * 1024 * 1024), nil }
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return remoteSwapState{HasOtherNonZRAMSwap: true}, nil
	}

	called := false
	ensureSwapfileHook = func(r *remoteRunner, ctx context.Context, state remoteSwapState) error {
		called = true
		return nil
	}

	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &bytes.Buffer{}, err: &bytes.Buffer{}},
	}

	if err := runner.warnOnMemoryMismatch(context.Background()); err != nil {
		t.Fatalf("warnOnMemoryMismatch() returned error: %v", err)
	}
	if called {
		t.Fatalf("did not expect ensureSwapfileHook to be called when other swap exists")
	}
}

func TestWarnOnMemoryMismatchMentionsEnabledSwapWhenAlreadyPresent(t *testing.T) {
	originalLocalMemoryTotalBytesHook := localMemoryTotalBytesHook
	originalRemoteMemoryTotalBytesHook := remoteMemoryTotalBytesHook
	originalRemoteSwapStateHook := remoteSwapStateHook
	t.Cleanup(func() {
		localMemoryTotalBytesHook = originalLocalMemoryTotalBytesHook
		remoteMemoryTotalBytesHook = originalRemoteMemoryTotalBytesHook
		remoteSwapStateHook = originalRemoteSwapStateHook
	})

	const gib = int64(1024 * 1024 * 1024)
	localMemoryTotalBytesHook = func() (int64, error) { return 4 * gib, nil }
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) { return int64(1900 * 1024 * 1024), nil }
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return remoteSwapState{SwapfileActive: true, FstabHasSwapfile: true}, nil
	}

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	if err := runner.warnOnMemoryMismatch(context.Background()); err != nil {
		t.Fatalf("warnOnMemoryMismatch() returned error: %v", err)
	}
	got := stripANSI(out.String())
	if !strings.Contains(got, "destination memory is low:") || !strings.Contains(got, "(swap already enabled)") {
		t.Fatalf("expected low-memory message to mention enabled swap, got %q", got)
	}
}

func TestAltPHPFeatureIDsFromStepCommandParsesResumeElids(t *testing.T) {
	t.Parallel()

	got := altPHPFeatureIDsFromStepCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp72, altphp83, altphp85' 'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'")
	for _, want := range []string{"altphp72", "altphp83", "altphp85"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected %q in parsed altphp ids, got %#v", want, got)
		}
	}
}

func TestAltPHPFeaturesBusyDetectsInstallStatusForRequestedVersions(t *testing.T) {
	t.Parallel()

	records := []featureRecord{
		{Name: "altphp72", Status: ""},
		{Name: "altphp83", Status: "install"},
	}
	if !altPHPFeaturesBusy(records, map[string]struct{}{"altphp83": {}}) {
		t.Fatalf("expected requested altphp feature in install state to be detected as busy")
	}
	if altPHPFeaturesBusy(records, map[string]struct{}{"altphp72": {}}) {
		t.Fatalf("did not expect completed altphp feature to be detected as busy")
	}
}

func TestFeatureStepBadStateDetectsRegularFeatureBadState(t *testing.T) {
	t.Parallel()

	bad, ok := featureStepBadState([]featureRecord{
		{Name: "web", BadState: "install"},
	}, "web", nil)
	if !ok {
		t.Fatalf("expected badstate to be detected for regular feature")
	}
	if bad.Name != "web" || bad.BadState != "install" {
		t.Fatalf("unexpected badstate info: %#v", bad)
	}
}

func TestFeatureStepBadStateDetectsAltPHPBadState(t *testing.T) {
	t.Parallel()

	bad, ok := featureStepBadState([]featureRecord{
		{Name: "altphp83", BadState: "install"},
		{Name: "altphp84", BadState: ""},
	}, "altphp", map[string]struct{}{"altphp83": {}})
	if !ok {
		t.Fatalf("expected badstate to be detected for requested altphp feature")
	}
	if bad.Name != "altphp83" || bad.BadState != "install" {
		t.Fatalf("unexpected altphp badstate info: %#v", bad)
	}
}

func TestFirstBadFeatureStateReturnsFirstBadRecord(t *testing.T) {
	t.Parallel()

	bad, ok := firstBadFeatureState([]featureRecord{
		{Name: "web", BadState: "install"},
		{Name: "mysql", BadState: "broken"},
	})
	if !ok {
		t.Fatalf("expected first bad feature state to be found")
	}
	if bad.Name != "web" || bad.BadState != "install" {
		t.Fatalf("unexpected first bad feature state: %#v", bad)
	}
}
