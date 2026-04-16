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
		"feature.update sok=ok updatesystem=on upgradesystem=off upgradewarning_msg=",
		"package_nginx=on",
		"package_logrotate=on",
		"packagegroup_mysql=mariadb-server",
		"package_dovecot=on",
		"package_opendkim=on",
		"packagegroup_altphp72gr=ispphp72",
		"package_ispphp72_lsapi=on",
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

	othersIndex := -1
	altPHPIndex := -1
	for index, group := range groups {
		if group.Title == "packages (altphp)" {
			altPHPIndex = index
			if len(group.Commands) < 2 {
				t.Fatalf("expected combined altphp group to contain update and commands, got %#v", group)
			}
			if group.Commands[0] != featureUpdateCommand() {
				t.Fatalf("expected first altphp command to be feature.update, got %q", group.Commands[0])
			}
		}
		if group.Title == "packages (others)" {
			othersIndex = index
			if len(group.Commands) < 2 {
				t.Fatalf("expected combined others group to contain update and commands, got %#v", group)
			}
			if group.Commands[0] != featureUpdateCommand() {
				t.Fatalf("expected first others command to be feature.update, got %q", group.Commands[0])
			}
			break
		}
	}
	if othersIndex == -1 {
		t.Fatalf("expected packages (others) group, got %#v", groups)
	}
	if altPHPIndex == -1 {
		t.Fatalf("expected packages (altphp) group, got %#v", groups)
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
