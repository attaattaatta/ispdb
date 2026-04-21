package app

import (
	"sort"
	"strconv"
	"strings"
)

func buildSourceData(raw rawSource, keyPath string) (SourceData, error) {
	key, err := loadPrivateKey(keyPath)
	if err != nil {
		return SourceData{}, err
	}

	data := SourceData{
		Format:         raw.format,
		PrivateKeyUsed: key != nil,
	}

	presetLimitsByKey := presetLimitProps(raw)
	userLimitsByID := userLimitProps(raw)

	userNames := map[string]string{}
	for _, row := range raw.tables["users"] {
		userID := rowValue(row, "id")
		presetName := rowValue(row, "preset")
		item := User{
			ID:              userID,
			Name:            rowValue(row, "name"),
			Active:          rowValue(row, "active"),
			SafePasswd:      rowValue(row, "safepasswd"),
			Level:           rowValue(row, "level"),
			Home:            rowValue(row, "home"),
			FullName:        rowValue(row, "fullname"),
			UID:             rowValue(row, "uid"),
			GID:             rowValue(row, "gid"),
			Shell:           rowValue(row, "shell"),
			Tag:             rowValue(row, "tag"),
			CreateTime:      rowValue(row, "create_time"),
			Comment:         rowValue(row, "comment"),
			Backup:          rowValue(row, "backup"),
			BackupType:      rowValue(row, "backup_type"),
			BackupSizeLimit: rowValue(row, "backup_size_limit"),
			Preset:          presetName,
			LimitProps:      mergeLimitProps(presetLimitsByKey[userID+"::"+presetName], presetLimitsByKey["::"+presetName], userLimitsByID[userID]),
		}
		data.Users = append(data.Users, item)
		userNames[item.ID] = item.Name
	}

	for _, row := range raw.tables["isppackages"] {
		data.Packages = append(data.Packages, Package{
			ID:   rowValue(row, "id"),
			Name: rowValue(row, "name"),
		})
	}

	aliasMap := map[string][]string{}
	for _, row := range raw.tables["webdomain_alias"] {
		aliasMap[rowValue(row, "webdomain")] = append(aliasMap[rowValue(row, "webdomain")], firstNonEmpty(rowValue(row, "name"), rowValue(row, "alias")))
	}

	ipMap := map[string][]string{}
	for _, row := range raw.tables["webdomain_ipaddr"] {
		ipMap[rowValue(row, "webdomain")] = append(ipMap[rowValue(row, "webdomain")], firstNonEmpty(rowValue(row, "value"), rowValue(row, "ipaddr"), rowValue(row, "ip")))
	}

	type dbMySQLServerInfo struct {
		Host    string
		Version string
	}
	dbMySQLServerMap := map[string]dbMySQLServerInfo{}
	for _, row := range raw.tables["db_mysql_servers"] {
		dbServerID := rowValue(row, "db_server")
		if dbServerID == "" {
			continue
		}
		host := strings.TrimSpace(rowValue(row, "hostname"))
		port := strings.TrimSpace(rowValue(row, "port"))
		if host == "" {
			host = strings.TrimSpace(rowValue(row, "host"))
		}
		if host == "0.0.0.0" {
			host = "localhost"
		}
		if port != "" {
			host = host + ":" + port
		}
		dbMySQLServerMap[dbServerID] = dbMySQLServerInfo{
			Host:    host,
			Version: strings.TrimSpace(rowValue(row, "version")),
		}
	}

	serverNames := map[string]string{}
	for _, row := range raw.tables["db_server"] {
		mysqlInfo := dbMySQLServerMap[rowValue(row, "id")]
		item := DBServer{
			ID:           rowValue(row, "id"),
			Name:         rowValue(row, "name"),
			Type:         rowValue(row, "type"),
			Host:         firstNonEmpty(mysqlInfo.Host, rowValue(row, "host")),
			Username:     rowValue(row, "username"),
			Password:     decryptPassword(rowValue(row, "password"), key),
			SavedVer:     firstNonEmpty(mysqlInfo.Version, rowValue(row, "savedver")),
			RemoteAccess: rowValue(row, "remote_access"),
		}
		data.DBServers = append(data.DBServers, item)
		serverNames[item.ID] = item.Name
	}

	emailDomainNames := map[string]string{}
	for _, row := range raw.tables["emaildomain"] {
		owner := userNames[rowValue(row, "users")]
		if owner == "" {
			owner = rowValue(row, "users")
		}
		item := EmailDomain{
			ID:      rowValue(row, "id"),
			Name:    rowValue(row, "name"),
			NameIDN: rowValue(row, "name_idn"),
			IP:      rowValue(row, "ip"),
			Active:  rowValue(row, "active"),
			Owner:   owner,
		}
		data.EmailDomains = append(data.EmailDomains, item)
		emailDomainNames[item.ID] = item.Name
	}

	for _, row := range raw.tables["ftp_users"] {
		owner := userNames[rowValue(row, "users")]
		if owner == "" {
			owner = rowValue(row, "users")
		}
		data.FTPUsers = append(data.FTPUsers, FTPUser{
			ID:       rowValue(row, "id"),
			Name:     rowValue(row, "name"),
			Active:   rowValue(row, "active"),
			Enabled:  rowValue(row, "enabled"),
			Home:     rowValue(row, "home"),
			Password: decryptPassword(rowValue(row, "password"), key),
			Owner:    owner,
		})
	}

	for _, row := range raw.tables["webdomain"] {
		id := rowValue(row, "id")
		owner := userNames[rowValue(row, "users")]
		if owner == "" {
			owner = rowValue(row, "users")
		}
		name := rowValue(row, "name")
		nameIDN := rowValue(row, "name_idn")
		data.WebDomains = append(data.WebDomains, WebDomain{
			ID:            id,
			Name:          name,
			NameIDN:       nameIDN,
			Aliases:       cleanAliases(aliasMap[id], name, nameIDN),
			DocRoot:       firstNonEmpty(rowValue(row, "docroot"), rowValue(row, "home")),
			Secure:        rowValue(row, "secure"),
			SSLCert:       firstNonEmpty(rowValue(row, "ssl_cert"), rowValue(row, "ssl_name")),
			Autosubdomain: rowValue(row, "autosubdomain"),
			PHPMode:       rowValue(row, "php_mode"),
			PHPVersion:    firstNonEmpty(rowValue(row, "php_cgi_version"), rowValue(row, "php_version"), rowValue(row, "php")),
			Active:        rowValue(row, "active"),
			Owner:         owner,
			IPAddr:        cleanAndJoin(ipMap[id]),
			RedirectHTTP:  rowValue(row, "redirect_http"),
		})
	}

	for _, row := range raw.tables["db_assign"] {
		owner := userNames[rowValue(row, "users")]
		if owner == "" {
			owner = rowValue(row, "users")
		}
		server := serverNames[rowValue(row, "db_server")]
		if server == "" {
			server = rowValue(row, "db_server")
		}
		data.Databases = append(data.Databases, Database{
			ID:          rowValue(row, "id"),
			Name:        rowValue(row, "name"),
			Unaccounted: rowValue(row, "unaccounted"),
			Owner:       owner,
			Server:      server,
		})
	}

	for _, row := range raw.tables["db_users_password"] {
		server := serverNames[rowValue(row, "db_server")]
		if server == "" {
			server = rowValue(row, "db_server")
		}
		data.DBUsers = append(data.DBUsers, DBUser{
			ID:       rowValue(row, "id"),
			Name:     rowValue(row, "name"),
			Password: decryptPassword(rowValue(row, "password"), key),
			Server:   server,
		})
	}

	for _, row := range raw.tables["email"] {
		domain := emailDomainNames[rowValue(row, "domain")]
		if domain == "" {
			domain = rowValue(row, "domain")
		}
		data.EmailBoxes = append(data.EmailBoxes, EmailBox{
			ID:       rowValue(row, "id"),
			Name:     rowValue(row, "name"),
			Domain:   domain,
			Password: decryptPassword(rowValue(row, "password"), key),
			MaxSize:  rowValue(row, "maxsize"),
			Used:     rowValue(row, "used"),
			Path:     rowValue(row, "path"),
			Active:   rowValue(row, "active"),
			Note:     rowValue(row, "note"),
		})
	}

	for _, row := range raw.tables["domain"] {
		owner := userNames[rowValue(row, "users")]
		if owner == "" {
			owner = rowValue(row, "users")
		}
		data.DNSDomains = append(data.DNSDomains, DNSDomain{
			ID:      rowValue(row, "id"),
			Name:    rowValue(row, "name"),
			NameIDN: rowValue(row, "name_idn"),
			Owner:   owner,
			DType:   firstNonEmpty(rowValue(row, "dtype"), rowValue(row, "type")),
		})
	}

	sort.Slice(data.Packages, func(i, j int) bool {
		return lessByNumericThenString(data.Packages[i].ID, data.Packages[j].ID, data.Packages[i].Name, data.Packages[j].Name)
	})
	sort.Slice(data.Users, func(i, j int) bool {
		return lessByNumericThenString(data.Users[i].ID, data.Users[j].ID, data.Users[i].Name, data.Users[j].Name)
	})
	sort.Slice(data.FTPUsers, func(i, j int) bool {
		return lessByNumericThenString(data.FTPUsers[i].ID, data.FTPUsers[j].ID, data.FTPUsers[i].Name, data.FTPUsers[j].Name)
	})
	sort.Slice(data.WebDomains, func(i, j int) bool {
		return lessByNumericThenString(data.WebDomains[i].ID, data.WebDomains[j].ID, data.WebDomains[i].Name, data.WebDomains[j].Name)
	})
	sort.Slice(data.DBServers, func(i, j int) bool {
		return lessByNumericThenString(data.DBServers[i].ID, data.DBServers[j].ID, data.DBServers[i].Name, data.DBServers[j].Name)
	})
	sort.Slice(data.Databases, func(i, j int) bool {
		return lessByNumericThenString(data.Databases[i].ID, data.Databases[j].ID, data.Databases[i].Name, data.Databases[j].Name)
	})
	sort.Slice(data.DBUsers, func(i, j int) bool {
		return lessByNumericThenString(data.DBUsers[i].ID, data.DBUsers[j].ID, data.DBUsers[i].Name, data.DBUsers[j].Name)
	})
	sort.Slice(data.EmailDomains, func(i, j int) bool {
		return lessByNumericThenString(data.EmailDomains[i].ID, data.EmailDomains[j].ID, data.EmailDomains[i].Name, data.EmailDomains[j].Name)
	})
	sort.Slice(data.EmailBoxes, func(i, j int) bool {
		return lessByNumericThenString(data.EmailBoxes[i].ID, data.EmailBoxes[j].ID, data.EmailBoxes[i].Name, data.EmailBoxes[j].Name)
	})
	sort.Slice(data.DNSDomains, func(i, j int) bool {
		return lessByNumericThenString(data.DNSDomains[i].ID, data.DNSDomains[j].ID, data.DNSDomains[i].Name, data.DNSDomains[j].Name)
	})

	if key == nil && hasEncryptedPasswords(raw) {
		data.Warnings = append(data.Warnings, "No ispmgr.pem key was provided, encrypted passwords remain encrypted.")
	}

	return data, nil
}

func userLimitProps(raw rawSource) map[string]map[string]string {
	result := map[string]map[string]string{}
	for _, row := range raw.tables["userprops"] {
		name := strings.TrimSpace(rowValue(row, "name"))
		if !strings.HasPrefix(name, "limit_") {
			continue
		}
		userID := rowValue(row, "users")
		if userID == "" {
			continue
		}
		if result[userID] == nil {
			result[userID] = map[string]string{}
		}
		result[userID][name] = rowValue(row, "value")
	}
	return result
}

func presetLimitProps(raw rawSource) map[string]map[string]string {
	presetIDs := map[string]string{}
	for _, row := range raw.tables["preset"] {
		name := strings.TrimSpace(rowValue(row, "name"))
		if name == "" {
			continue
		}
		key := rowValue(row, "users") + "::" + name
		presetIDs[key] = rowValue(row, "id")
		if rowValue(row, "users") == "" {
			presetIDs["::"+name] = rowValue(row, "id")
		}
	}

	propsByPresetID := map[string]map[string]string{}
	for _, row := range raw.tables["preset_props"] {
		name := strings.TrimSpace(rowValue(row, "name"))
		if !strings.HasPrefix(name, "limit_") {
			continue
		}
		presetID := rowValue(row, "preset")
		if presetID == "" {
			continue
		}
		if propsByPresetID[presetID] == nil {
			propsByPresetID[presetID] = map[string]string{}
		}
		propsByPresetID[presetID][name] = rowValue(row, "value")
	}

	result := map[string]map[string]string{}
	for key, presetID := range presetIDs {
		if props := propsByPresetID[presetID]; len(props) > 0 {
			result[key] = cloneStringMap(props)
		}
	}
	return result
}

func mergeLimitProps(values ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, value := range values {
		for key, item := range value {
			merged[key] = item
		}
	}
	return merged
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func rowValue(row rawRow, key string) string {
	return strings.TrimSpace(row[key])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cleanAndJoin(values []string) string {
	seen := map[string]struct{}{}
	list := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	sort.Strings(list)
	return strings.Join(list, ", ")
}

func cleanAliases(values []string, names ...string) string {
	skip := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			skip[name] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := skip[strings.TrimSpace(strings.ToLower(value))]; ok {
			continue
		}
		filtered = append(filtered, value)
	}
	return cleanAndJoin(filtered)
}

func lessByNumericThenString(left, right, leftName, rightName string) bool {
	leftID, leftErr := strconv.Atoi(left)
	rightID, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil && leftID != rightID {
		return leftID < rightID
	}
	if left != right {
		return left < right
	}
	return leftName < rightName
}

func hasEncryptedPasswords(raw rawSource) bool {
	for _, tableName := range []string{"ftp_users", "db_server", "db_users_password", "email"} {
		for _, row := range raw.tables[tableName] {
			if strings.TrimSpace(rowValue(row, "password")) != "" {
				return true
			}
		}
	}
	return false
}
