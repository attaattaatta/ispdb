package app

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDBPath    = "/usr/local/mgr5/etc/ispmgr.db"
	defaultKeyPath   = "/usr/local/mgr5/etc/ispmgr.pem"
	defaultLock      = "/root/.ispdb/ispdb.lock"
	defaultMySQLHost = "localhost"
	defaultMySQLPort = 3306
)

var (
	listModes     = []string{"all", "commands", "packages", "webdomains", "databases", "users", "email", "dns"}
	exportScopes  = []string{"all", "data", "commands", "packages", "webdomains", "databases", "users", "email", "dns"}
	exportFormats = []string{"text", "csv", "json"}
	logLevels     = []string{"off", "info", "warn", "error", "crit", "debug"}
	bulkModes     = []string{"create", "modify", "delete"}
	bulkTypes     = []string{"webdomains", "databases", "users", "emaildomain", "emailbox", "dns"}
	leModes       = []string{"on", "off"}
)

type Config struct {
	DBFile          string
	DBDisplay       string
	ISPKey          string
	ListMode        string
	ListExplicit    bool
	ExportFile      string
	ExportScope     string
	ExportFormat    string
	DestHost        string
	DestAuth        string
	Force           bool
	ShowHelp        bool
	LogLevel        string
	LogFile         string
	Silent          bool
	UseLocalMySQL   bool
	MySQLPassword   string
	CSVDelimiter    rune
	Columns         []string
	BulkMode        string
	BulkType        string
	BinaryName      string
	DomainsSource   string
	OwnersSource    string
	IPsSource       string
	NamesSource     string
	PasswordsSource string
	DBServersSource string
	NSSource        string
	LEMode          string
	MemeMode        bool
	DevMode         bool
}

func ParseConfig(binaryName string, args []string) (Config, error) {
	cfg := Config{
		ListMode:     "all",
		ExportFormat: "text",
		LogLevel:     "info",
		CSVDelimiter: ',',
		BinaryName:   sanitizeBinaryName(binaryName),
		LEMode:       "off",
	}

	if len(args) == 1 && strings.EqualFold(strings.TrimSpace(args[0]), "pepe") {
		cfg.MemeMode = true
		return cfg, nil
	}

	if len(args) == 0 {
		if err := applyDefaultDataSource(&cfg); err != nil {
			cfg.ShowHelp = true
			return cfg, nil
		}
		if _, err := os.Stat(defaultKeyPath); err == nil {
			cfg.ISPKey = defaultKeyPath
		}
		return cfg, nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-f", "--file":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.DBFile = filepath.Clean(value)
			cfg.DBDisplay = cfg.DBFile
			i = next
		case "-k", "--key":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.ISPKey = filepath.Clean(value)
			i = next
		case "--list":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			value = strings.ToLower(value)
			if !contains(listModes, value) {
				return cfg, unsupportedValueError("--list", value, listModes)
			}
			cfg.ListMode = value
			cfg.ListExplicit = true
			i = next
		case "-e", "--export":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.ExportFile = filepath.Clean(value)
			i = next
		case "--export-data":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			value = strings.ToLower(value)
			if !contains(exportScopes, value) {
				return cfg, unsupportedValueError("--export-data", value, exportScopes)
			}
			cfg.ExportScope = value
			i = next
		case "--format":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			value = strings.ToLower(value)
			if !contains(exportFormats, value) {
				return cfg, unsupportedValueError("--format", value, exportFormats)
			}
			cfg.ExportFormat = value
			i = next
		case "--csv-delimiter":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			if len([]rune(value)) != 1 {
				return cfg, errors.New("unsupported --csv-delimiter value: use exactly one character")
			}
			cfg.CSVDelimiter = []rune(value)[0]
			i = next
		case "--columns":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.Columns = parseColumns(value)
			i = next
		case "-d", "--dest":
			host, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			if ip := net.ParseIP(host); ip == nil || strings.Contains(host, ":") {
				return cfg, fmt.Errorf("invalid IPv4 address for %s: %s", arg, host)
			}
			cfg.DestHost = host
			i = next
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				cfg.DestAuth = args[i+1]
				i++
			}
		case "--force":
			cfg.Force = true
		case "--log":
			level, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			level = strings.ToLower(level)
			if !contains(logLevels, level) {
				return cfg, unsupportedValueError("--log", level, logLevels)
			}
			cfg.LogLevel = level
			if level == "off" {
				cfg.Silent = true
			}
			i = next
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				cfg.LogFile = filepath.Clean(args[i+1])
				i++
			}
		case "-b", "--bulk":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			value = strings.ToLower(value)
			if !contains(bulkModes, value) {
				return cfg, unsupportedValueError("--bulk", value, bulkModes)
			}
			cfg.BulkMode = value
			i = next
		case "--type":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			value = strings.ToLower(value)
			if !contains(bulkTypes, value) {
				return cfg, unsupportedValueError("--type", value, bulkTypes)
			}
			cfg.BulkType = value
			i = next
		case "--domains":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.DomainsSource = value
			i = next
		case "--owners":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.OwnersSource = value
			i = next
		case "--ips":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.IPsSource = value
			i = next
		case "--names":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.NamesSource = value
			i = next
		case "--passwords":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.PasswordsSource = value
			i = next
		case "--dbservers":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.DBServersSource = value
			i = next
		case "--ns":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			cfg.NSSource = value
			i = next
		case "--le":
			value, next, err := nextArg(args, i, arg)
			if err != nil {
				return cfg, err
			}
			value = strings.ToLower(strings.TrimSpace(value))
			if !contains(leModes, value) {
				return cfg, unsupportedValueError("--le", value, leModes)
			}
			cfg.LEMode = value
			i = next
		case "-h", "--help":
			cfg.ShowHelp = true
		case "--x-dev":
			cfg.DevMode = true
		default:
			return cfg, fmt.Errorf("unknown option: %s", arg)
		}
	}

	if cfg.ExportFile != "" && cfg.ExportScope == "" {
		cfg.ExportScope = defaultExportScope(cfg.ListMode)
	}
	if cfg.ExportScope == "commands" && cfg.ExportFormat != "text" {
		return cfg, errors.New("commands export supports only --format text")
	}
	if cfg.ExportFile == "" && cfg.ExportScope != "" {
		return cfg, errors.New("--export-data requires --export <file>")
	}
	if cfg.ExportFormat != "text" && cfg.ExportFile == "" {
		return cfg, errors.New("--format requires --export <file>")
	}
	if cfg.Force && cfg.DestHost == "" {
		return cfg, errors.New("--force can be used only together with --dest")
	}
	if cfg.BulkMode != "" && cfg.DestHost != "" {
		return cfg, errors.New("--bulk cannot be used together with --dest")
	}
	if cfg.BulkMode != "" && cfg.BulkType == "" {
		return cfg, fmt.Errorf("--bulk %s requires --type. Supported values: %s", cfg.BulkMode, strings.Join(bulkTypes, ", "))
	}
	if cfg.BulkMode == "" && cfg.BulkType != "" {
		return cfg, errors.New("--type requires --bulk")
	}
	if cfg.LEMode != "off" && !(cfg.BulkMode == "modify" && cfg.BulkType == "webdomains") {
		return cfg, errors.New("--le can be used only with --bulk modify --type webdomains")
	}

	if cfg.BulkMode != "" || cfg.ShowHelp {
		return cfg, nil
	}

	if cfg.DBFile == "" && !cfg.UseLocalMySQL {
		if err := applyDefaultDataSource(&cfg); err != nil {
			return cfg, err
		}
		if cfg.ISPKey == "" {
			if _, err := os.Stat(defaultKeyPath); err == nil {
				cfg.ISPKey = defaultKeyPath
			}
		}
	}

	return cfg, nil
}

func applyDefaultDataSource(cfg *Config) error {
	if _, err := os.Stat(defaultDBPath); err == nil {
		cfg.DBFile = defaultDBPath
		cfg.DBDisplay = defaultDBPath
		return nil
	}

	password, err := readMyCNFPassword("/root/.my.cnf")
	if err == nil && password != "" {
		cfg.UseLocalMySQL = true
		cfg.MySQLPassword = password
		cfg.DBDisplay = fmt.Sprintf("%s:%d", defaultMySQLHost, defaultMySQLPort)
		return nil
	}

	return errors.New("database source was not provided. Default SQLite file /usr/local/mgr5/etc/ispmgr.db was not found and MySQL password was not found in /root/.my.cnf")
}

func readMyCNFPassword(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	inClientSection := false
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inClientSection = strings.EqualFold(strings.Trim(line, "[]"), "client")
			continue
		}
		if !inClientSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "password") {
			return strings.TrimSpace(value), nil
		}
	}

	return "", errors.New("password was not found in /root/.my.cnf [client] section")
}

func defaultExportScope(listMode string) string {
	if contains(listModes, listMode) && listMode != "all" {
		return listMode
	}
	return "data"
}

func nextArg(args []string, index int, name string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("missing value for %s", name)
	}
	return args[index+1], index + 1, nil
}

func unsupportedValueError(flag string, value string, supported []string) error {
	return fmt.Errorf("unsupported %s value: %s. Supported values: %s", flag, value, strings.Join(supported, ", "))
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func parseColumns(value string) []string {
	parts := strings.Split(value, ",")
	columns := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		column := strings.ToLower(strings.TrimSpace(part))
		if column == "" {
			continue
		}
		if _, ok := seen[column]; ok {
			continue
		}
		seen[column] = struct{}{}
		columns = append(columns, column)
	}
	return columns
}

func sanitizeBinaryName(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "ispdb"
	}
	return name
}
