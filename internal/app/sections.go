package app

func (s SourceData) sections(mode string) []Section {
	sections := make([]Section, 0, 10)
	include := func(group string) bool {
		return mode == "all" || mode == group
	}

	if include("packages") {
		sections = append(sections, Section{
			Title:        "packages",
			Headers:      []string{"id", "name"},
			Rows:         packageRows(s.Packages),
			EmptyMessage: "No packages were found.",
		})
	}

	if include("users") {
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
		sections = append(sections,
			Section{
				Title:        "users",
				Headers:      []string{"id", "name", "active", "safepasswd", "level", "home", "fullname", "uid", "gid", "shell", "tag", "create_time", "comment", "backup", "backup_type", "backup_size_limit"},
				Rows:         userRows(s.Users),
				EmptyMessage: "No users were found.",
			},
			Section{
				Title:        "ftp users",
				Subtitle:     ftpSubtitle,
				Headers:      []string{"id", "name", "active", "enabled", "home", "password", "owner"},
				Rows:         ftpRowsData,
				EmptyMessage: "No FTP users were found.",
			},
			Section{
				Title:        "db users",
				Subtitle:     dbUsersSubtitle,
				Headers:      []string{"id", "name", "password", "db_server"},
				Rows:         dbUserRowsData,
				EmptyMessage: "No database users were found.",
			},
		)
	}

	if include("webdomains") {
		sections = append(sections, Section{
			Title:        "web domains",
			Headers:      []string{"id", "name", "name_idn", "aliases", "docroot", "secure", "ssl_cert", "autosubdomain", "php_mode", "php_version", "active", "owner", "ipaddr", "redirect_http"},
			Rows:         webRows(s.WebDomains),
			EmptyMessage: "No web domains were found.",
		})
	}

	if include("databases") {
		dbServerRowsData := dbServerRows(s.DBServers)
		dbUserRowsData := dbUserRows(s.DBUsers)
		dbSubtitle := ""
		if !s.PrivateKeyUsed && (len(dbServerRowsData) > 0 || len(dbUserRowsData) > 0) {
			dbSubtitle = "Passwords are not decrypted because no ispmgr.pem key was provided. See --key option."
		}
		sections = append(sections,
			Section{
				Title:        "database servers",
				Subtitle:     dbSubtitle,
				Headers:      []string{"id", "name", "type", "host", "username", "password", "remote_access", "savedver"},
				Rows:         dbServerRowsData,
				EmptyMessage: "No database servers were found.",
			},
			Section{
				Title:        "databases",
				Headers:      []string{"id", "name", "unaccounted", "owner", "db_server"},
				Rows:         databaseRows(s.Databases),
				EmptyMessage: "No databases were found.",
			},
		)
		if mode == "databases" {
			sections = append(sections, Section{
				Title:        "db users",
				Subtitle:     dbSubtitle,
				Headers:      []string{"id", "name", "password", "db_server"},
				Rows:         dbUserRowsData,
				EmptyMessage: "No database users were found.",
			})
		}
	}

	if include("email") {
		emailBoxRowsData := emailBoxRows(s.EmailBoxes)
		emailSubtitle := ""
		if !s.PrivateKeyUsed && len(emailBoxRowsData) > 0 {
			emailSubtitle = "Mailbox passwords are not decrypted because no ispmgr.pem key was provided. See --key option."
		}
		sections = append(sections,
			Section{
				Title:        "email domains",
				Headers:      []string{"id", "name", "name_idn", "ip", "active", "owner"},
				Rows:         emailDomainRows(s.EmailDomains),
				EmptyMessage: "No email domains were found.",
			},
			Section{
				Title:        "email boxes",
				Subtitle:     emailSubtitle,
				Headers:      []string{"id", "name", "domain", "password", "path", "active", "maxsize", "used", "note"},
				Rows:         emailBoxRowsData,
				EmptyMessage: "No email boxes were found.",
			},
		)
	}

	if include("dns") {
		sections = append(sections, Section{
			Title:        "dns",
			Headers:      []string{"id", "name", "name_idn", "owner", "dtype"},
			Rows:         dnsRows(s.DNSDomains),
			EmptyMessage: "No DNS zones were found.",
		})
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
		rows = append(rows, []string{value.ID, value.Name, value.Domain, value.Password, value.Path, value.Active, value.MaxSize, value.Used, value.Note})
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
