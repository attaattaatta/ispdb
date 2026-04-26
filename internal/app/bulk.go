package app

import (
	"fmt"
	"os"
	"strings"
)

func runBulkWorkflow(ui *UI, cfg Config) error {
	var commands []string
	var err error
	switch cfg.BulkMode {
	case "create":
		switch cfg.BulkType {
		case "webdomains":
			commands, err = buildBulkWebDomains(cfg)
		case "users":
			commands, err = buildBulkUsers(cfg)
		case "databases":
			commands, err = buildBulkDatabases(cfg)
		case "emaildomain":
			commands, err = buildBulkEmailDomains(cfg)
		case "emailbox":
			commands, err = buildBulkEmailBoxes(cfg)
		case "dns":
			commands, err = buildBulkDNS(cfg)
		default:
			return unsupportedValueError("--type", cfg.BulkType, bulkTypes)
		}
	case "modify":
		switch cfg.BulkType {
		case "webdomains":
			commands, err = buildBulkModifyWebDomains(cfg)
		default:
			return fmt.Errorf("bulk mode %q is not implemented for type %q yet. Supported now: --bulk create for all listed types, --bulk modify for webdomains", cfg.BulkMode, cfg.BulkType)
		}
	default:
		return fmt.Errorf("bulk mode %q is not implemented yet. Supported now: create, modify for webdomains", cfg.BulkMode)
	}
	if err != nil {
		return err
	}

	ui.Println(formatTitle("commands to run at remote server:", true))
	ui.Println("")
	for _, command := range commands {
		ui.Println(command)
	}
	return nil
}

func buildBulkWebDomains(cfg Config) ([]string, error) {
	domains, err := loadBulkList(cfg.DomainsSource, "Please enter / paste domains to create (each domain on newline)", 0)
	if err != nil {
		return nil, err
	}
	owners, err := loadBulkList(cfg.OwnersSource, "Please enter / paste owners to use (each owner on newline)", len(domains))
	if err != nil {
		return nil, err
	}
	ips, err := loadBulkList(cfg.IPsSource, "Please enter / paste IP addresses to use (each IP on newline)", len(domains))
	if err != nil {
		return nil, err
	}
	useLE, err := askYesNo("Use Let's Encrypt certificates for created web domains?", true)
	if err != nil {
		return nil, err
	}

	commands := make([]string, 0, len(domains)*3)
	for i, domain := range domains {
		owner := owners[i]
		ip := ips[i]
		sslCert := "selfsigned"
		if useLE {
			sslCert = "letsencrypt"
		}
		if !useLE {
			commands = append(commands, buildSelfSignedCertCommand(domain, owner))
			sslCert = domain
		}
		commands = append(commands, buildMgrctlCommand("site.edit", map[string]string{
			"site_name":          domain,
			"site_aliases":       "",
			"site_owner":         owner,
			"site_autosubdomain": "off",
			"site_basedir":       "on",
			"site_ipaddrs":       ip,
			"site_hsts":          "on",
			"site_ipsrc":         "manual",
			"site_limit_ssl":     "on",
			"site_phpcomposer":   "off",
			"site_php_enable":    "on",
			"site_php_mode":      "php_mode_mod",
			"site_redirect_http": "off",
			"site_secure":        "on",
			"site_ssl_cert":      sslCert,
			"site_ssl_port":      "443",
			"site_srv_cache":     "off",
			"sok":                "ok",
		}))
		if !useLE {
			continue
		}
		certName := domain + "_le1"
		commands = append(commands,
			buildMgrctlCommand("webdomain.edit", map[string]string{
				"cgi":             "off",
				"currname":        domain,
				"ddosshield":      "off",
				"elid":            domain,
				"email":           "webmaster@" + domain,
				"handler":         "handler_not_used",
				"hsts":            "on",
				"ipaddrs":         ip,
				"ipsrc":           "manual",
				"letsencrypt":     "yes",
				"log_access":      "off",
				"log_error":       "off",
				"nodejs":          "off",
				"nodejs_use_port": "off",
				"owner":           owner,
				"pagespeed":       "off",
				"php":             "off",
				"php_enable":      "on",
				"python":          "off",
				"secure":          "on",
				"sok":             "ok",
				"srv_cache":       "off",
				"ssl_cert":        certName,
				"ssl_port":        "443",
			}),
			buildMgrctlCommand("letsencrypt.generate", map[string]string{
				"crtname":             certName,
				"domain":              domain,
				"domain_name":         domain,
				"dns_check":           "off",
				"elid":                domain,
				"email":               "webmaster@" + domain,
				"enable_cert":         "off",
				"enable_cert_email":   "off",
				"from_webdomain":      "on",
				"keylen":              "2048",
				"name":                certName,
				"skip_check_a_record": "on",
				"sok":                 "ok",
				"username":            owner,
				"wildcard":            "off",
			}),
		)
	}
	return commands, nil
}

func buildBulkModifyWebDomains(cfg Config) ([]string, error) {
	domains, err := loadBulkList(cfg.DomainsSource, "Please enter / paste domains to modify (each domain on newline)", 0)
	if err != nil {
		return nil, err
	}
	owners, err := loadBulkList(cfg.OwnersSource, "Please enter / paste owners to use (each owner on newline)", len(domains))
	if err != nil {
		return nil, err
	}
	ips, err := loadBulkList(cfg.IPsSource, "Please enter / paste IP addresses to use (each IP on newline)", len(domains))
	if err != nil {
		return nil, err
	}

	useLE := strings.EqualFold(cfg.LEMode, "on")
	commands := make([]string, 0, len(domains)*2)
	for i, domain := range domains {
		owner := owners[i]
		ip := ips[i]
		sslMode := "selfsigned"
		if useLE {
			if strings.HasPrefix(domain, "*.") {
				return nil, fmt.Errorf("Let's Encrypt bulk modify supports non-wildcard domains only: %s", domain)
			}
			sslMode = "letsencrypt"
		}

		commands = append(commands, buildWebDomainModifyShellCommand(domain, owner, ip, sslMode))
		if useLE {
			commands = append(commands, buildWebDomainLEGenerateShellCommand(domain, owner))
		}
	}
	return commands, nil
}

func buildBulkUsers(cfg Config) ([]string, error) {
	names, err := loadBulkList(cfg.NamesSource, "Please enter / paste user names to create (each name on newline)", 0)
	if err != nil {
		return nil, err
	}
	commands := make([]string, 0, len(names))
	for _, name := range names {
		if strings.EqualFold(name, "root") {
			continue
		}
		password, err := randomPassword(16)
		if err != nil {
			return nil, err
		}
		commands = append(commands, buildMgrctlCommand("user.edit", userEditParams(name, password, "#custom", nil)))
	}
	return commands, nil
}

func buildBulkDatabases(cfg Config) ([]string, error) {
	names, err := loadBulkList(cfg.NamesSource, "Please enter / paste database names to create (each name on newline)", 0)
	if err != nil {
		return nil, err
	}
	passwords, err := loadBulkList(cfg.PasswordsSource, "Please enter / paste database passwords (each password on newline)", len(names))
	if err != nil {
		return nil, err
	}
	owners, err := loadBulkList(cfg.OwnersSource, "Please enter / paste database owners (each owner on newline)", len(names))
	if err != nil {
		return nil, err
	}
	servers, err := loadBulkList(cfg.DBServersSource, "Please enter / paste database server names (each server on newline)", len(names))
	if err != nil {
		return nil, err
	}

	commands := make([]string, 0, len(names))
	for i, name := range names {
		server := servers[i]
		commands = append(commands, buildMgrctlCommand("db.edit", map[string]string{
			"charset":       databaseCharsetForType(server),
			"confirm":       passwords[i],
			"name":          name,
			"owner":         owners[i],
			"password":      passwords[i],
			"remote_access": "off",
			"server":        server,
			"sok":           "ok",
			"username":      name,
		}))
	}
	return commands, nil
}

func buildBulkEmailDomains(cfg Config) ([]string, error) {
	domains, err := loadBulkList(cfg.DomainsSource, "Please enter / paste email domains to create (each domain on newline)", 0)
	if err != nil {
		return nil, err
	}
	owners, err := loadBulkList(cfg.OwnersSource, "Please enter / paste owners to use (each owner on newline)", len(domains))
	if err != nil {
		return nil, err
	}

	commands := make([]string, 0, len(domains))
	for i, domain := range domains {
		commands = append(commands, buildMgrctlCommand("emaildomain.edit", map[string]string{
			"defaction":    "ignore",
			"dkim":         "on",
			"dmarc":        "on",
			"ipsrc":        "auto",
			"name":         domain,
			"owner":        owners[i],
			"secure":       "off",
			"secure_alias": "mail." + domain,
			"sok":          "ok",
			"ssl_name":     "selfsigned",
		}))
	}
	return commands, nil
}

func buildBulkEmailBoxes(cfg Config) ([]string, error) {
	names, err := loadBulkList(cfg.NamesSource, "Please enter / paste mailbox names to create (each name on newline)", 0)
	if err != nil {
		return nil, err
	}
	domains, err := loadBulkList(cfg.DomainsSource, "Please enter / paste mailbox domains to use (each domain on newline)", len(names))
	if err != nil {
		return nil, err
	}
	passwords, err := loadBulkList(cfg.PasswordsSource, "Please enter / paste mailbox passwords (each password on newline)", len(names))
	if err != nil {
		return nil, err
	}

	commands := make([]string, 0, len(names))
	for i, name := range names {
		commands = append(commands, buildMgrctlCommand("email.edit", map[string]string{
			"confirm":    passwords[i],
			"domainname": domains[i],
			"maxsize":    "0",
			"name":       name,
			"passwd":     passwords[i],
			"sok":        "ok",
		}))
	}
	return commands, nil
}

func buildBulkDNS(cfg Config) ([]string, error) {
	domains, err := loadBulkList(cfg.DomainsSource, "Please enter / paste DNS zones to create (each domain on newline)", 0)
	if err != nil {
		return nil, err
	}
	owners, err := loadBulkList(cfg.OwnersSource, "Please enter / paste DNS owners (each owner on newline)", len(domains))
	if err != nil {
		return nil, err
	}
	ips, err := loadBulkList(cfg.IPsSource, "Please enter / paste DNS IP addresses (each IP on newline)", len(domains))
	if err != nil {
		return nil, err
	}
	nameservers, err := loadBulkList(cfg.NSSource, "Please enter / paste DNS nameserver values (each ns list on newline)", len(domains))
	if err != nil {
		return nil, err
	}

	commands := make([]string, 0, len(domains))
	for i, domain := range domains {
		commands = append(commands, buildMgrctlCommand("domain.edit", map[string]string{
			"dnssec": "off",
			"dtype":  "master",
			"ip":     ips[i],
			"ipsrc":  "manual",
			"name":   domain,
			"ns":     nameservers[i],
			"owner":  owners[i],
			"sok":    "ok",
		}))
	}
	return commands, nil
}

func loadBulkList(source string, question string, expected int) ([]string, error) {
	for {
		values, interactive, err := readBulkValues(source, question)
		if err != nil {
			return nil, err
		}
		values = cleanBulkValues(values)
		if len(values) == 0 {
			return nil, fmt.Errorf("required list is empty")
		}
		normalized, ok := normalizeBulkValues(values, expected)
		if ok {
			return normalized, nil
		}
		if !interactive {
			return nil, fmt.Errorf("input count does not match required item count %d", expected)
		}
		message := fmt.Sprintf("%sInput count does not match required item count %d. Please enter this list again.%s\n", colorRed, expected, colorReset)
		fmt.Print(message)
		mirrorProgramOutput(message)
	}
}

func readBulkValues(source string, question string) ([]string, bool, error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "stdin":
		values, err := askLines(question)
		return values, true, err
	default:
		content, err := os.ReadFile(source)
		if err != nil {
			return nil, false, err
		}
		return strings.Split(string(content), "\n"), false, nil
	}
}

func cleanBulkValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func normalizeBulkValues(values []string, expected int) ([]string, bool) {
	if expected <= 0 || len(values) == expected {
		return values, true
	}
	if len(values) == 1 {
		result := make([]string, expected)
		for i := range result {
			result[i] = values[0]
		}
		return result, true
	}
	return nil, false
}

func buildWebDomainModifyShellCommand(domain string, owner string, ip string, sslMode string) string {
	elidLookup := webDomainELIDLookup(domain)
	prelude := ""
	siteSSLCert := shellQuote(sslMode)
	if strings.EqualFold(sslMode, "selfsigned") {
		prelude = buildSelfSignedCertCommand(domain, owner) + " && "
		siteSSLCert = shellQuote(domain)
	}
	args := []string{
		"/bin/sh",
		"-lc",
		shellQuote(prelude + "elid=\"$(" + elidLookup + ")\"; [ -n \"$elid\" ] && /usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit elid=\"$elid\" site_name=" + shellQuote(domain) + " site_aliases= site_owner=" + shellQuote(owner) + " site_autosubdomain=off site_basedir=on site_ipaddrs=" + shellQuote(ip) + " site_hsts=on site_ipsrc=manual site_limit_ssl=on site_ssl_port=443 site_ssl_cert=" + siteSSLCert + " site_srv_cache=off site_secure=on site_phpcomposer=off site_php_enable=on site_php_mode=php_mode_mod site_redirect_http=off sok=ok"),
	}
	return strings.Join(args, " ")
}

func buildWebDomainLEGenerateShellCommand(domain string, owner string) string {
	elidLookup := webDomainELIDLookup(domain)
	script := "elid=\"$(" + elidLookup + ")\"; " +
		"[ -n \"$elid\" ] && " +
		"crtname=\"$(/usr/local/mgr5/sbin/mgrctl -m ispmgr site.edit elid=\"$elid\" out=text | awk -F= '/^site_ssl_cert=/{print $2; exit}')\"; " +
		"[ -n \"$crtname\" ] && " +
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr letsencrypt.generate " +
		"elid=" + shellQuote(domain) + " " +
		"crtname=\"$crtname\" " +
		"username=" + shellQuote(owner) + " " +
		"wildcard=off dns_check=off " +
		"domain=" + shellQuote(domain) + " " +
		"domain_name=" + shellQuote(domain) + " " +
		"domain_type= " +
		"email=" + shellQuote("webmaster@"+domain) + " " +
		"enable_cert=off enable_cert_email=off keylen=2048 " +
		"name=\"$crtname\" skip_check_a_record=on from_webdomain=on sok=ok"
	return "/bin/sh -lc " + shellQuote(script)
}

func webDomainELIDLookup(domain string) string {
	return "/usr/local/mgr5/sbin/mgrctl -m ispmgr webdomain out=text | awk -v domain=" +
		shellQuote(domain) + " " +
		shellQuote(`$0 ~ (" name=" domain " ") || $0 ~ (" name=" domain "$") {for (i = 1; i <= NF; i++) if ($i ~ /^id=/) {sub(/^id=/, "", $i); print $i; exit}}`)
}
