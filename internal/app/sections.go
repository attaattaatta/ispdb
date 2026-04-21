package app

import (
	"sort"
	"strings"
)

func (s SourceData) sections(mode string) []Section {
	return s.sectionsForScopes(dataScopesFromListMode(mode))
}

func (s SourceData) listSections(mode string) []Section {
	return prepareListSections(s.sections(mode))
}

func (s SourceData) listSectionsForScopes(scopes []string) []Section {
	return prepareListSections(s.sectionsForScopes(scopes))
}

func (s SourceData) sectionsForScopes(scopes []string) []Section {
	if len(scopes) == 0 {
		scopes = append([]string{}, dataScopeOrder...)
	}

	sections := make([]Section, 0, 10)
	seen := map[string]struct{}{}
	appendUnique := func(section Section) {
		if _, ok := seen[section.Title]; ok {
			return
		}
		seen[section.Title] = struct{}{}
		sections = append(sections, section)
	}

	for _, scope := range scopes {
		switch scope {
		case "packages":
			appendUnique(Section{
				Title:        "packages",
				Headers:      []string{"id", "name"},
				Rows:         packageRows(s.Packages),
				EmptyMessage: "No packages were found.",
			})
		case "users":
			ftpRowsData := ftpRows(s.FTPUsers)
			ftpSubtitle := ""
			if !s.PrivateKeyUsed && len(ftpRowsData) > 0 {
				ftpSubtitle = "Passwords for FTP users are not decrypted because no ispmgr.pem key was provided. See --key option."
			}
			dbUserRowsData := dbUserRows(s.DBUsers)
			dbUsersSubtitle := ""
			if !s.PrivateKeyUsed && len(dbUserRowsData) > 0 {
				dbUsersSubtitle = "Passwords for DB users are not decrypted because no ispmgr.pem key was provided. See --key option."
			}
			appendUnique(Section{
				Title:        "users",
				Headers:      []string{"id", "name", "active", "safepasswd", "level", "home", "fullname", "uid", "gid", "shell", "tag", "create_time", "comment", "backup", "backup_type", "backup_size_limit"},
				Rows:         userRows(s.Users),
				EmptyMessage: "No users were found.",
			})
			appendUnique(Section{
				Title:        "ftp users",
				Subtitle:     ftpSubtitle,
				Headers:      []string{"id", "name", "active", "enabled", "home", "password", "owner"},
				Rows:         ftpRowsData,
				EmptyMessage: "No FTP users were found.",
			})
			appendUnique(Section{
				Title:        "db users",
				Subtitle:     dbUsersSubtitle,
				Headers:      []string{"id", "name", "password", "db_server"},
				Rows:         dbUserRowsData,
				EmptyMessage: "No database users were found.",
			})
		case "webdomains":
			appendUnique(Section{
				Title:        "web domains",
				Headers:      []string{"id", "name", "name_idn", "aliases", "docroot", "secure", "ssl_cert", "autosubdomain", "php_mode", "php_version", "active", "owner", "ipaddr", "redirect_http"},
				Rows:         webRows(s.WebDomains),
				EmptyMessage: "No web domains were found.",
			})
		case "databases":
			dbServerRowsData := dbServerRows(s.DBServers)
			dbUserRowsData := dbUserRows(s.DBUsers)
			dbSubtitle := ""
			if !s.PrivateKeyUsed && (len(dbServerRowsData) > 0 || len(dbUserRowsData) > 0) {
				dbSubtitle = "Passwords are not decrypted because no ispmgr.pem key was provided. See --key option."
			}
			appendUnique(Section{
				Title:        "database servers",
				Subtitle:     dbSubtitle,
				Headers:      []string{"id", "name", "type", "host", "username", "password", "remote_access", "savedver"},
				Rows:         dbServerRowsData,
				EmptyMessage: "No database servers were found.",
			})
			appendUnique(Section{
				Title:        "databases",
				Headers:      []string{"id", "name", "unaccounted", "owner", "db_server"},
				Rows:         databaseRows(s.Databases),
				EmptyMessage: "No databases were found.",
			})
			appendUnique(Section{
				Title:        "db users",
				Subtitle:     dbSubtitle,
				Headers:      []string{"id", "name", "password", "db_server"},
				Rows:         dbUserRowsData,
				EmptyMessage: "No database users were found.",
			})
		case "email":
			emailBoxRowsData := emailBoxRows(s.EmailBoxes)
			emailSubtitle := ""
			if !s.PrivateKeyUsed && len(emailBoxRowsData) > 0 {
				emailSubtitle = "Mailbox passwords are not decrypted because no ispmgr.pem key was provided. See --key option."
			}
			appendUnique(Section{
				Title:        "email domains",
				Headers:      []string{"id", "name", "name_idn", "ip", "active", "owner"},
				Rows:         emailDomainRows(s.EmailDomains),
				EmptyMessage: "No email domains were found.",
			})
			appendUnique(Section{
				Title:        "email boxes",
				Subtitle:     emailSubtitle,
				Headers:      []string{"id", "name", "domain", "email_forward", "password", "path", "active", "maxsize", "used", "note"},
				Rows:         emailBoxRowsData,
				EmptyMessage: "No email boxes were found.",
			})
		case "dns":
			appendUnique(Section{
				Title:        "dns",
				Headers:      []string{"id", "name", "name_idn", "owner", "dtype"},
				Rows:         dnsRows(s.DNSDomains),
				EmptyMessage: "No DNS zones were found.",
			})
		}
	}

	return sections
}

func packageRows(values []Package) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name})
	}
	return rows
}

func userRows(values []User) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.Active, value.SafePasswd, value.Level, value.Home, value.FullName, value.UID, value.GID, value.Shell, value.Tag, value.CreateTime, value.Comment, value.Backup, value.BackupType, value.BackupSizeLimit})
	}
	return rows
}

func ftpRows(values []FTPUser) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.Active, value.Enabled, value.Home, value.Password, value.Owner})
	}
	return rows
}

func webRows(values []WebDomain) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.NameIDN, value.Aliases, value.DocRoot, value.Secure, value.SSLCert, value.Autosubdomain, value.PHPMode, value.PHPVersion, value.Active, value.Owner, value.IPAddr, value.RedirectHTTP})
	}
	return rows
}

func dbServerRows(values []DBServer) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.Type, value.Host, value.Username, value.Password, value.RemoteAccess, value.SavedVer})
	}
	return rows
}

func databaseRows(values []Database) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.Unaccounted, value.Owner, value.Server})
	}
	return rows
}

func dbUserRows(values []DBUser) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.Password, value.Server})
	}
	return rows
}

func emailDomainRows(values []EmailDomain) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.NameIDN, value.IP, value.Active, value.Owner})
	}
	return rows
}

func emailBoxRows(values []EmailBox) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.Domain, value.Forward, value.Password, value.Path, value.Active, value.MaxSize, value.Used, value.Note})
	}
	return rows
}

func dnsRows(values []DNSDomain) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.ID, value.Name, value.NameIDN, value.Owner, value.DType})
	}
	return rows
}

func prepareListSections(sections []Section) []Section {
	prepared := make([]Section, 0, len(sections))
	for _, section := range sections {
		prepared = append(prepared, prepareListSection(section))
	}
	return prepared
}

func prepareListSection(section Section) Section {
	headers := make([]string, 0, len(section.Headers))
	indexes := make([]int, 0, len(section.Headers))
	for index, header := range section.Headers {
		if shouldHideListColumn(header) {
			continue
		}
		headers = append(headers, header)
		indexes = append(indexes, index)
	}

	rows := make([][]string, 0, len(section.Rows))
	for _, row := range section.Rows {
		item := make([]string, 0, len(indexes))
		for _, index := range indexes {
			if index < len(row) {
				item = append(item, row[index])
			} else {
				item = append(item, "")
			}
		}
		rows = append(rows, item)
	}

	headers, rows = reorderListColumns(section.Title, headers, rows)
	headers = renameListHeaders(section.Title, headers)
	rows = normalizeListRows(section.Title, headers, rows)

	nameIndex := indexOfHeader(headers, "name")
	if nameIndex >= 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			left := strings.ToLower(strings.TrimSpace(rows[i][nameIndex]))
			right := strings.ToLower(strings.TrimSpace(rows[j][nameIndex]))
			if left == right {
				return strings.Join(rows[i], "\x00") < strings.Join(rows[j], "\x00")
			}
			return left < right
		})
	}

	section.Headers = headers
	section.Rows = rows
	return section
}

func reorderListColumns(title string, headers []string, rows [][]string) ([]string, [][]string) {
	order := listColumnOrder(title)
	if len(order) == 0 {
		return headers, rows
	}

	targetIndexes := make([]int, 0, len(headers))
	used := make([]bool, len(headers))
	for _, want := range order {
		index := indexOfHeader(headers, want)
		if index < 0 || used[index] {
			continue
		}
		targetIndexes = append(targetIndexes, index)
		used[index] = true
	}
	for index := range headers {
		if used[index] {
			continue
		}
		targetIndexes = append(targetIndexes, index)
	}

	reorderedHeaders := make([]string, 0, len(targetIndexes))
	for _, index := range targetIndexes {
		reorderedHeaders = append(reorderedHeaders, headers[index])
	}

	reorderedRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		item := make([]string, 0, len(targetIndexes))
		for _, index := range targetIndexes {
			if index < len(row) {
				item = append(item, row[index])
			} else {
				item = append(item, "")
			}
		}
		reorderedRows = append(reorderedRows, item)
	}

	return reorderedHeaders, reorderedRows
}

func listColumnOrder(title string) []string {
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "users":
		return []string{"name", "home", "active", "level", "uid", "gid", "shell", "backup"}
	case "ftp users":
		return []string{"name", "password", "home", "active", "enabled", "owner"}
	case "web domains":
		return []string{"name", "aliases", "docroot", "php_version", "php_mode", "owner", "ssl_cert", "autosubdomain", "active", "ipaddr", "redirect_http"}
	case "databases":
		return []string{"name", "owner", "db_server", "unaccounted"}
	case "email boxes":
		return []string{"name", "domain", "password", "email_forward", "path", "active", "maxsize", "used", "note"}
	default:
		return nil
	}
}

func renameListHeaders(title string, headers []string) []string {
	renamed := append([]string{}, headers...)
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "email boxes":
		for index, header := range renamed {
			if strings.EqualFold(header, "used") {
				renamed[index] = "used_mb"
			}
		}
	}
	return renamed
}

func normalizeListRows(title string, headers []string, rows [][]string) [][]string {
	normalized := make([][]string, 0, len(rows))
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "database servers":
		savedverIndex := indexOfHeader(headers, "savedver")
		if savedverIndex < 0 {
			return rows
		}
		for _, row := range rows {
			item := append([]string{}, row...)
			if savedverIndex < len(item) {
				item[savedverIndex] = trimDatabaseServerSavedVer(item[savedverIndex])
			}
			normalized = append(normalized, item)
		}
		return normalized
	default:
		return rows
	}
}

func trimDatabaseServerSavedVer(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "PostgreSQL ") {
		return value
	}
	if index := strings.Index(value, ") on "); index >= 0 {
		return strings.TrimSpace(value[:index+1])
	}
	if index := strings.Index(value, ", compiled by "); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}

func shouldHideListColumn(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "id",
		"secure",
		"name_idn",
		"safepasswd",
		"fullname",
		"tag",
		"create_time",
		"comment",
		"backup_type",
		"backup_size_limit":
		return true
	default:
		return false
	}
}

func indexOfHeader(headers []string, target string) int {
	for index, header := range headers {
		if strings.EqualFold(header, target) {
			return index
		}
	}
	return -1
}
