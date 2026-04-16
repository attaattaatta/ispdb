package app

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

type CommandGroup struct {
	Title    string
	Commands []string
}

const defaultNameservers = "ns1.example.com. ns2.example.com."

type CommandBuildOptions struct {
	DefaultIP string
	DefaultNS string
	TargetOS  string
	TargetPanel string
}

func buildCommands(data SourceData, scope string, options CommandBuildOptions) ([]CommandGroup, []string) {
	groups := make([]CommandGroup, 0, 6)
	warnings := make([]string, 0)
	include := func(group string) bool {
		return scope == "" || scope == "all" || scope == "commands" || scope == group
	}
	appendGroup := func(title string, commands []string) {
		if len(commands) == 0 {
			return
		}
		groups = append(groups, CommandGroup{Title: title, Commands: commands})
	}

	if include("packages") {
		packageGroups, packageWarnings := buildPackageCommandGroups(data.Packages, options.TargetOS, options.TargetPanel)
		groups = append(groups, packageGroups...)
		warnings = append(warnings, packageWarnings...)
	}

	if include("users") {
		groupCommands := make([]string, 0)
		for _, item := range data.Users {
			if strings.EqualFold(strings.TrimSpace(item.Name), "root") {
				continue
			}
			password, err := randomPassword(16)
			if err != nil {
				return nil, []string{fmt.Sprintf("failed to generate password for user %s: %v", item.Name, err)}
			}
			params := userEditParams(item.Name, password)
			if item.FullName != "" {
				params["fullname"] = item.FullName
			}
			if item.Comment != "" {
				params["comment"] = item.Comment
			}
			if item.Backup != "" {
				params["backup"] = item.Backup
			}
			groupCommands = append(groupCommands, buildMgrctlCommand("user.edit", params))
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
			groupCommands = append(groupCommands, buildMgrctlCommand("ftp.user.edit", map[string]string{
				"confirm": password,
				"home":    firstNonEmpty(item.Home, "/"),
				"name":    item.Name,
				"owner":   item.Owner,
				"passwd":  password,
				"sok":     "ok",
			}))
		}
		appendGroup("users", groupCommands)
	}

	if include("webdomains") {
		groupCommands := make([]string, 0)
		for _, item := range data.WebDomains {
			params := map[string]string{
				"site_aliases":       item.Aliases,
				"site_analyzer":      "off",
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
				"site_ssl_cert":      normalizedSSLCert(item.SSLCert),
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
		appendGroup("web sites", groupCommands)
	}

	if include("databases") {
		groupCommands := make([]string, 0)
		usedServers := map[string]struct{}{}
		for _, item := range data.Databases {
			if strings.TrimSpace(item.Server) != "" {
				usedServers[item.Server] = struct{}{}
			}
		}
		for _, item := range data.DBUsers {
			if strings.TrimSpace(item.Server) != "" {
				usedServers[item.Server] = struct{}{}
			}
		}

		for _, item := range data.DBServers {
			if len(usedServers) > 0 {
				if _, ok := usedServers[item.Name]; !ok {
					continue
				}
			}
			params := map[string]string{
				"host":          item.Host,
				"name":          item.Name,
				"password":      item.Password,
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

		dbPasswords := make(map[string]string, len(data.DBUsers))
		for _, item := range data.DBUsers {
			dbPasswords[item.Server+"::"+item.Name] = item.Password
		}

		for _, item := range data.Databases {
			password := dbPasswords[item.Server+"::"+item.Name]
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
				"username":      item.Name,
			}))
		}

		appendGroup("databases", groupCommands)
	}

	if include("email") {
		groupCommands := make([]string, 0)
		for _, item := range data.EmailDomains {
			ip := firstNonEmpty(item.IP, options.DefaultIP)
			groupCommands = append(groupCommands, buildMgrctlCommand("emaildomain.edit", map[string]string{
				"defaction":    "ignore",
				"dkim":         "on",
				"dmarc":        "on",
				"ip":           ip,
				"ipsrc":        ip,
				"name":         item.Name,
				"owner":        item.Owner,
				"secure":       "off",
				"secure_alias": "mail." + item.Name,
				"sok":          "ok",
				"ssl_name":     "selfsigned",
			}))
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
	}

	if include("dns") {
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

	return groups, warnings
}

func flattenCommandGroups(groups []CommandGroup) []string {
	commands := make([]string, 0)
	for _, group := range groups {
		commands = append(commands, group.Commands...)
	}
	return commands
}

func userEditParams(name string, password string) map[string]string {
	return map[string]string{
		"backup":                       "on",
		"confirm":                      password,
		"limit_cgi":                    "on",
		"limit_db_enabled":             "on",
		"limit_db_users_enabled":       "on",
		"limit_dirindex":               "index.php index.html",
		"limit_emaildomains_enabled":   "on",
		"limit_ftp_users_enabled":      "on",
		"limit_php_apache_version":     "native",
		"limit_php_fpm_version":        "native",
		"limit_php_mode":               "php_mode_mod",
		"limit_php_mode_cgi":           "on",
		"limit_php_mode_fcgi_nginxfpm": "on",
		"limit_php_mode_mod":           "on",
		"limit_shell":                  "on",
		"limit_ssl":                    "on",
		"limit_webdomains_enabled":     "on",
		"name":                         name,
		"passwd":                       password,
		"php_enable":                   "on",
		"preset":                       "#custom",
		"sok":                          "ok",
	}
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
	parts := []string{"/usr/local/mgr5/sbin/mgrctl", "-m", "ispmgr", function}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == "sok" && keys[j] != "sok" {
			return true
		}
		if keys[j] == "sok" && keys[i] != "sok" {
			return false
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		parts = append(parts, shellQuote(fmt.Sprintf("%s=%s", key, params[key])))
	}
	return strings.Join(parts, " ")
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
