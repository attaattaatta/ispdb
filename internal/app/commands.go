package app

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

type CommandGroup struct {
	Title    string
	Commands []string
}

const defaultNameservers = "ns1.example.com. ns2.example.com."

type CommandBuildOptions struct {
	DefaultIP        string
	DefaultNS        string
	TargetOS         string
	TargetPanel      string
	CurrentPackages  map[string]struct{}
	NoDeletePackages bool
}

type dbCredential struct {
	Name     string
	Password string
}

type dbCredentialResolver struct {
	exact     map[string]dbCredential
	remaining map[string][]dbCredential
}

func buildCommands(data SourceData, scope string, options CommandBuildOptions) ([]CommandGroup, []string) {
	return buildCommandsForScopes(data, commandScopesFromListMode(scope), options)
}

func buildCommandsForScopes(data SourceData, scopes []string, options CommandBuildOptions) ([]CommandGroup, []string) {
	if len(scopes) == 0 {
		scopes = append([]string{}, dataScopeOrder...)
	}

	groups := make([]CommandGroup, 0, 6)
	warnings := make([]string, 0)
	appendGroup := func(title string, commands []string) {
		if len(commands) == 0 {
			return
		}
		groups = append(groups, CommandGroup{Title: title, Commands: commands})
	}

	for _, scope := range scopes {
		switch scope {
		case "packages":
			packageGroups, packageWarnings := buildPackageCommandGroupsWithCurrent(data.Packages, options.TargetOS, options.TargetPanel, options.CurrentPackages, options.NoDeletePackages)
			groups = append(groups, packageGroups...)
			warnings = append(warnings, packageWarnings...)
		case "users":
			userCommands := make([]string, 0)
			ftpCommands := make([]string, 0)
			for _, item := range data.Users {
				if strings.EqualFold(strings.TrimSpace(item.Name), "root") {
					continue
				}
				password, err := randomPassword(16)
				if err != nil {
					return nil, []string{fmt.Sprintf("failed to generate password for user %s: %v", item.Name, err)}
				}
				params := userEditParams(item.Name, password, item.Preset, item.LimitProps)
				if item.FullName != "" {
					params["fullname"] = item.FullName
				}
				if item.Comment != "" {
					params["comment"] = item.Comment
				}
				if item.Backup != "" {
					params["backup"] = item.Backup
				}
				userCommands = append(userCommands, buildMgrctlCommand("user.edit", params))
			}
			for _, item := range data.FTPUsers {
				password := item.Password
				if strings.TrimSpace(password) == "" {
					generated, err := randomPassword(16)
					if err != nil {
						return nil, []string{fmt.Sprintf("failed to generate FTP password for %s: %v", item.Name, err)}
					}
					password = generated
					warnings = append(warnings, fmt.Sprintf("FTP user %s password was not available, generated a random password for commands.", item.Name))
				}
				ftpCommands = append(ftpCommands, buildMgrctlCommand("ftp.user.edit", map[string]string{
					"confirm": password,
					"home":    firstNonEmpty(item.Home, "/"),
					"name":    item.Name,
					"owner":   item.Owner,
					"passwd":  password,
					"sok":     "ok",
				}))
			}
			appendGroup("users", userCommands)
			appendGroup("ftp users", ftpCommands)
		case "webdomains":
			certCommands := make([]string, 0)
			groupCommands := make([]string, 0)
			for _, item := range data.WebDomains {
				siteSSLCert, siteCertCommands := plannedSiteSSLCert(item.Name, item.Owner, item.SSLCert)
				certCommands = append(certCommands, siteCertCommands...)
				params := map[string]string{
					"site_aliases":       item.Aliases,
					"site_autosubdomain": firstNonEmpty(item.Autosubdomain, "off"),
					"site_basedir":       "on",
					"site_hsts":          "on",
					"site_ipaddrs":       firstNonEmpty(item.IPAddr, options.DefaultIP),
					"site_ipsrc":         "manual",
					"site_limit_ssl":     "on",
					"site_name":          item.Name,
					"site_owner":         item.Owner,
					"site_php_enable":    "on",
					"site_php_mode":      firstNonEmpty(item.PHPMode, "php_mode_mod"),
					"site_phpcomposer":   "off",
					"site_redirect_http": firstNonEmpty(item.RedirectHTTP, "off"),
					"site_secure":        firstNonEmpty(item.Secure, "on"),
					"site_ssl_cert":      siteSSLCert,
					"site_ssl_port":      "443",
					"site_srv_cache":     "off",
					"sok":                "ok",
				}
				if item.PHPVersion != "" {
					params["site_php_cgi_version"] = item.PHPVersion
					params["site_php_fpm_version"] = item.PHPVersion
					params["site_php_apache_version"] = item.PHPVersion
				}
				groupCommands = append(groupCommands, buildMgrctlCommand("site.edit", params))
			}
			appendGroup("ssl certificates", uniqueStringsPreserveOrder(certCommands))
			appendGroup("web sites", groupCommands)
		case "databases":
			groupCommands := make([]string, 0)
			for _, item := range data.DBServers {
				password := item.Password
				if strings.TrimSpace(password) == "" {
					generated, err := randomPassword(16)
					if err != nil {
						return nil, []string{fmt.Sprintf("failed to generate database server password for %s: %v", item.Name, err)}
					}
					password = generated
					warnings = append(warnings, fmt.Sprintf("Database server %s password was not available, generated a random password for commands.", item.Name))
				}
				params := map[string]string{
					"host":          item.Host,
					"name":          item.Name,
					"password":      password,
					"remote_access": firstNonEmpty(item.RemoteAccess, "off"),
					"sok":           "ok",
					"type":          item.Type,
					"username":      item.Username,
				}
				if shouldUseDBServerVersion(item.SavedVer) {
					params["version"] = item.SavedVer
				}
				groupCommands = append(groupCommands, buildMgrctlCommand("db.server.edit", params))
			}

			credentials := buildDBCredentials(data.DBUsers)

			for _, item := range data.Databases {
				username, password := credentials.resolve(item.Server, item.Name)
				if strings.TrimSpace(username) == "" {
					username = item.Name
				}
				if strings.TrimSpace(password) == "" {
					generated, err := randomPassword(16)
					if err != nil {
						return nil, []string{fmt.Sprintf("failed to generate database password for %s: %v", item.Name, err)}
					}
					password = generated
					warnings = append(warnings, fmt.Sprintf("Database %s password was not available, generated a random password for commands.", item.Name))
				}
				groupCommands = append(groupCommands, buildMgrctlCommand("db.edit", map[string]string{
					"charset":       "utf8mb4",
					"confirm":       password,
					"name":          item.Name,
					"owner":         item.Owner,
					"password":      password,
					"remote_access": "off",
					"server":        item.Server,
					"sok":           "ok",
					"username":      username,
				}))
			}

			appendGroup("databases", groupCommands)
		case "email":
			groupCommands := make([]string, 0)
			for _, item := range data.EmailDomains {
				params := map[string]string{
					"defaction":    "ignore",
					"dkim":         "on",
					"dmarc":        "on",
					"name":         item.Name,
					"owner":        item.Owner,
					"secure":       firstNonEmpty(item.Secure, "off"),
					"secure_alias": firstNonEmpty(item.SecureAlias, "mail."+item.Name),
					"sok":          "ok",
					"ssl_name":     "selfsigned",
				}
				if strings.EqualFold(strings.TrimSpace(item.Secure), "on") {
					params["email"] = "admin@" + item.Name
				}
				groupCommands = append(groupCommands, buildMgrctlCommand("emaildomain.edit", params))
			}

			for _, item := range data.EmailBoxes {
				password := item.Password
				if strings.TrimSpace(password) == "" {
					generated, err := randomPassword(16)
					if err != nil {
						return nil, []string{fmt.Sprintf("failed to generate mailbox password for %s: %v", item.Name, err)}
					}
					password = generated
					warnings = append(warnings, fmt.Sprintf("Mailbox %s password was not available, generated a random password for commands.", item.Name))
				}
				groupCommands = append(groupCommands, buildMgrctlCommand("email.edit", map[string]string{
					"confirm":    password,
					"domainname": item.Domain,
					"maxsize":    firstNonEmpty(item.MaxSize, "0"),
					"name":       item.Name,
					"note":       item.Note,
					"passwd":     password,
					"sok":        "ok",
				}))
			}
			appendGroup("email", groupCommands)
		case "dns":
			groupCommands := make([]string, 0)
			for _, item := range data.DNSDomains {
				groupCommands = append(groupCommands, buildMgrctlCommand("domain.edit", map[string]string{
					"dnssec":      "off",
					"dtype":       firstNonEmpty(item.DType, "master"),
					"ip":          options.DefaultIP,
					"ipsrc":       "manual",
					"name":        item.Name,
					"ns":          firstNonEmpty(strings.TrimSpace(options.DefaultNS), defaultNameservers),
					"owner":       item.Owner,
					"reversezone": "",
					"sok":         "ok",
				}))
			}
			appendGroup("dns", groupCommands)
		}
	}

	return groups, warnings
}

func flattenCommandGroups(groups []CommandGroup) []string {
	commands := make([]string, 0)
	for _, group := range groups {
		commands = append(commands, group.Commands...)
	}
	return commands
}

func buildDBCredentials(values []DBUser) dbCredentialResolver {
	resolver := dbCredentialResolver{
		exact:     make(map[string]dbCredential, len(values)),
		remaining: map[string][]dbCredential{},
	}
	for _, item := range values {
		server := strings.TrimSpace(item.Server)
		name := strings.TrimSpace(item.Name)
		if server == "" || name == "" {
			continue
		}
		key := server + "::" + name
		credential := dbCredential{Name: name, Password: item.Password}
		resolver.exact[key] = credential
		resolver.remaining[server] = append(resolver.remaining[server], credential)
	}
	for server, credentials := range resolver.remaining {
		sort.SliceStable(credentials, func(i, j int) bool {
			return strings.ToLower(credentials[i].Name) < strings.ToLower(credentials[j].Name)
		})
		resolver.remaining[server] = credentials
	}
	return resolver
}

func (r *dbCredentialResolver) resolve(server string, databaseName string) (string, string) {
	server = strings.TrimSpace(server)
	databaseName = strings.TrimSpace(databaseName)
	if credential, ok := r.exact[server+"::"+databaseName]; ok {
		r.consume(server, credential.Name)
		return credential.Name, credential.Password
	}
	return "", ""
}

func (r *dbCredentialResolver) consume(server string, name string) {
	remaining := r.remaining[server]
	if len(remaining) == 0 {
		return
	}
	filtered := remaining[:0]
	for _, credential := range remaining {
		if strings.EqualFold(credential.Name, name) {
			continue
		}
		filtered = append(filtered, credential)
	}
	if len(filtered) == 0 {
		delete(r.remaining, server)
		return
	}
	r.remaining[server] = filtered
}

func userEditParams(name string, password string, preset string, limitProps map[string]string) map[string]string {
	params := map[string]string{
		"backup":     "on",
		"confirm":    password,
		"name":       name,
		"passwd":     password,
		"php_enable": "on",
		"preset":     firstNonEmpty(strings.TrimSpace(preset), "#custom"),
		"sok":        "ok",
	}
	for _, key := range orderedUserLimitKeys(limitProps) {
		params[key] = limitProps[key]
	}
	return params
}

func orderedUserLimitKeys(limitProps map[string]string) []string {
	if len(limitProps) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(limitProps))
	for _, key := range knownUserLimitKeys {
		if _, ok := limitProps[key]; !ok {
			continue
		}
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	extra := make([]string, 0)
	for key := range limitProps {
		if !strings.HasPrefix(key, "limit_") {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		extra = append(extra, key)
	}
	sort.Strings(extra)
	return append(keys, extra...)
}

var knownUserLimitKeys = []string{
	"limit_cgi",
	"limit_cgi_inaccess",
	"limit_charset",
	"limit_db",
	"limit_db_enabled",
	"limit_db_inaccess",
	"limit_db_users",
	"limit_db_users_inaccess",
	"limit_emaildomains",
	"limit_emaildomains_enabled",
	"limit_emaildomains_inaccess",
	"limit_emails",
	"limit_emails_inaccess",
	"limit_ftp_users",
	"limit_ftp_users_inaccess",
	"limit_mailrate",
	"limit_mailrate_inaccess",
	"limit_php_apache_version",
	"limit_php_cgi_enable",
	"limit_php_cgi_version",
	"limit_php_fpm_version",
	"limit_php_mode",
	"limit_php_mode_cgi",
	"limit_php_mode_cgi_inaccess",
	"limit_php_mode_fcgi_nginxfpm",
	"limit_php_mode_fcgi_nginxfpm_inaccess",
	"limit_php_mode_mod",
	"limit_php_mode_mod_inaccess",
	"limit_python",
	"limit_python_inaccess",
	"limit_python_version",
	"limit_shell",
	"limit_shell_inaccess",
	"limit_ssl",
	"limit_ssl_inaccess",
	"limit_webdomains",
	"limit_webdomains_enabled",
	"limit_webdomains_inaccess",
}

func normalizedSSLCert(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "selfsigned"
	}
	switch strings.ToLower(value) {
	case "selfsigned", "default":
		return "selfsigned"
	default:
		return value
	}
}

func plannedSiteSSLCert(domain string, owner string, sourceCert string) (string, []string) {
	normalized := normalizedSSLCert(sourceCert)
	if normalized == "selfsigned" {
		return domain, []string{buildSelfSignedCertCommand(domain, owner)}
	}
	return normalized, nil
}

func buildSelfSignedCertCommand(domain string, owner string) string {
	now := time.Now().UTC()
	params := map[string]string{
		"city":           "XX",
		"clicked_button": "ok",
		"code":           "XX",
		"department":     "XX",
		"domain":         domain,
		"email":          "webmaster@" + domain,
		"end_date":       now.AddDate(1, 0, 0).Format("2006-01-02"),
		"keylen":         "2048",
		"org":            "XX",
		"sok":            "ok",
		"start_date":     now.Format("2006-01-02"),
		"state":          "XX",
		"username":       firstNonEmpty(owner, "root"),
	}
	return buildMgrctlCommand("sslcert.selfsigned", params)
}

func databaseExists(values []Database, server string, name string) bool {
	for _, value := range values {
		if value.Server == server && value.Name == name {
			return true
		}
	}
	return false
}

func shouldUseDBServerVersion(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, ":")
}

func randomPassword(length int) (string, error) {
	const lowers = "abcdefghijklmnopqrstuvwxyz"
	const uppers = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const symbols = "!@#$%^&*()-_=+[]{}<>?"
	const all = lowers + uppers + digits + symbols

	if length < 4 {
		length = 4
	}

	picks := []string{lowers, uppers, digits, symbols}
	chars := make([]byte, 0, length)
	for _, group := range picks {
		ch, err := randomChar(group)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	for len(chars) < length {
		ch, err := randomChar(all)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	if err := shuffleBytes(chars); err != nil {
		return "", err
	}
	return string(chars), nil
}

func randomChar(group string) (byte, error) {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(group))))
	if err != nil {
		return 0, err
	}
	return group[index.Int64()], nil
}

func shuffleBytes(values []byte) error {
	for i := len(values) - 1; i > 0; i-- {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(index.Int64())
		values[i], values[j] = values[j], values[i]
	}
	return nil
}

func buildMgrctlCommand(function string, params map[string]string) string {
	return buildMgrctlCommandWithForcedQuotes(function, params, nil)
}

func buildMgrctlCommandWithForcedQuotes(function string, params map[string]string, forceQuoteKeys map[string]struct{}) string {
	parts := []string{"/usr/local/mgr5/sbin/mgrctl", "-m", "ispmgr", function}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == "name" && keys[j] != "name" {
			return true
		}
		if keys[j] == "name" && keys[i] != "name" {
			return false
		}
		if keys[i] == "sok" && keys[j] != "sok" {
			return true
		}
		if keys[j] == "sok" && keys[i] != "sok" {
			return false
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		value := fmt.Sprintf("%s=%s", key, params[key])
		if _, ok := forceQuoteKeys[key]; ok {
			parts = append(parts, forceShellQuote(value))
			continue
		}
		parts = append(parts, shellQuote(value))
	}
	return strings.Join(parts, " ")
}

func forceShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	safe := true
	for _, r := range value {
		if !(r == '.' || r == '_' || r == '-' || r == '/' || r == ':' || r == '=' || r == '%' || r == '*' || r == '@' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			safe = false
			break
		}
	}
	if safe {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
