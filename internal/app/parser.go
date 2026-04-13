package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

type rawRow map[string]string

type rawSource struct {
	format string
	tables map[string][]rawRow
}

func loadSource(ctx context.Context, cfg Config) (rawSource, error) {
	if cfg.UseLocalMySQL {
		return loadLocalMySQLSource(ctx, cfg.MySQLPassword)
	}

	file, err := os.Open(filepath.Clean(cfg.DBFile))
	if err != nil {
		return rawSource{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return rawSource{}, err
	}
	if len(content) >= 16 && string(content[:16]) == "SQLite format 3\x00" {
		return loadSQLiteSource(ctx, cfg.DBFile)
	}
	text := string(content)
	if looksLikeSQLDump(text) {
		return parseSQLDump(text)
	}
	return rawSource{}, errors.New("unsupported database file format")
}

func looksLikeSQLDump(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "insert into") || strings.Contains(lower, "insert ignore into") || strings.Contains(lower, "create table")
}

func loadLocalMySQLSource(ctx context.Context, password string) (rawSource, error) {
	dsn := fmt.Sprintf("root:%s@tcp(%s:%d)/ispmgr?charset=utf8mb4&parseTime=true&loc=Local", password, defaultMySQLHost, defaultMySQLPort)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return rawSource{}, err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return rawSource{}, err
	}
	tables, err := queryTrackedMySQLTables(ctx, db)
	if err != nil {
		return rawSource{}, err
	}
	return rawSource{format: "MySQL", tables: tables}, nil
}

func loadSQLiteSource(ctx context.Context, path string) (rawSource, error) {
	dsn := "file:" + filepath.ToSlash(filepath.Clean(path)) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return rawSource{}, err
	}
	defer db.Close()

	tables := make(map[string][]rawRow)
	for _, table := range trackedTables() {
		rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", table))
		if err != nil {
			continue
		}
		parsed, err := scanRows(rows)
		rows.Close()
		if err != nil {
			return rawSource{}, err
		}
		tables[table] = parsed
	}
	return rawSource{format: "SQLite", tables: tables}, nil
}

func queryTrackedMySQLTables(ctx context.Context, db *sql.DB) (map[string][]rawRow, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := beginConsistentSnapshot(ctx, conn); err != nil {
		return nil, err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
	}()

	tables := make(map[string][]rawRow)
	for _, table := range trackedTables() {
		rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT * FROM `%s`", table))
		if err != nil {
			continue
		}
		parsed, err := scanRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		tables[table] = parsed
	}
	return tables, nil
}

func beginConsistentSnapshot(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY"); err == nil {
		if _, err := conn.ExecContext(ctx, "START TRANSACTION WITH CONSISTENT SNAPSHOT"); err == nil {
			return nil
		}
	}
	if _, err := conn.ExecContext(ctx, "START TRANSACTION READ ONLY WITH CONSISTENT SNAPSHOT"); err == nil {
		return nil
	}
	if _, err := conn.ExecContext(ctx, "START TRANSACTION WITH CONSISTENT SNAPSHOT"); err == nil {
		return nil
	}
	if _, err := conn.ExecContext(ctx, "START TRANSACTION READ ONLY"); err == nil {
		return nil
	}
	_, err := conn.ExecContext(ctx, "START TRANSACTION")
	return err
}

func scanRows(rows *sql.Rows) ([]rawRow, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	list := make([]rawRow, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := make(rawRow, len(columns))
		for i, column := range columns {
			item[column] = sqlValueToString(values[i])
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func sqlValueToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		return typed
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "on"
		}
		return "off"
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprint(typed)
	}
}

func parseSQLDump(content string) (rawSource, error) {
	statements := splitSQLStatements(content)
	tables := make(map[string][]rawRow)
	for _, statement := range statements {
		table, columns, rows, ok, err := parseInsertStatement(statement)
		if err != nil {
			return rawSource{}, err
		}
		if !ok {
			continue
		}
		for _, row := range rows {
			if len(row) != len(columns) {
				return rawSource{}, fmt.Errorf("malformed INSERT row for table %s", table)
			}
			item := make(rawRow, len(columns))
			for index, column := range columns {
				item[column] = row[index]
			}
			tables[table] = append(tables[table], item)
		}
	}
	if len(tables) == 0 {
		return rawSource{}, errors.New("no supported INSERT statements were found in SQL dump")
	}
	return rawSource{format: "MySQL", tables: tables}, nil
}

func splitSQLStatements(content string) []string {
	var statements []string
	var builder strings.Builder
	inString := false
	escape := false
	for _, r := range content {
		builder.WriteRune(r)
		if escape {
			escape = false
			continue
		}
		switch r {
		case '\\':
			if inString {
				escape = true
			}
		case '\'':
			inString = !inString
		case ';':
			if !inString {
				statements = append(statements, builder.String())
				builder.Reset()
			}
		}
	}
	if strings.TrimSpace(builder.String()) != "" {
		statements = append(statements, builder.String())
	}
	return statements
}

func parseInsertStatement(statement string) (string, []string, [][]string, bool, error) {
	trimmed := strings.TrimSpace(statement)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "insert") {
		return "", nil, nil, false, nil
	}

	intoIndex := strings.Index(lower, "into")
	if intoIndex < 0 {
		return "", nil, nil, false, nil
	}

	afterInto := strings.TrimSpace(trimmed[intoIndex+len("into"):])
	table, remainder, err := parseTableName(afterInto)
	if err != nil {
		return "", nil, nil, false, err
	}
	remainder = strings.TrimSpace(remainder)
	if !strings.HasPrefix(remainder, "(") {
		return "", nil, nil, false, fmt.Errorf("INSERT statement for table %s does not include explicit column list", table)
	}
	columnEnd := findMatchingParen(remainder)
	if columnEnd < 0 {
		return "", nil, nil, false, fmt.Errorf("failed to parse column list for table %s", table)
	}
	columns := parseIdentifierList(remainder[1:columnEnd])
	valuesPart := strings.TrimSpace(remainder[columnEnd+1:])
	if !strings.HasPrefix(strings.ToLower(valuesPart), "values") {
		return "", nil, nil, false, nil
	}
	valuesPart = strings.TrimSpace(valuesPart[len("values"):])
	rows, err := parseValueRows(valuesPart)
	if err != nil {
		return "", nil, nil, false, err
	}
	return table, columns, rows, true, nil
}

func parseTableName(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errors.New("empty table name")
	}
	if input[0] == '`' || input[0] == '"' {
		quote := input[0]
		end := strings.IndexByte(input[1:], quote)
		if end < 0 {
			return "", "", errors.New("unterminated table name")
		}
		return input[1 : end+1], input[end+2:], nil
	}
	for index, r := range input {
		if r == ' ' || r == '\t' || r == '\n' || r == '(' {
			return strings.Trim(input[:index], "`\""), input[index:], nil
		}
	}
	return strings.Trim(input, "`\""), "", nil
}

func findMatchingParen(value string) int {
	depth := 0
	inString := false
	escape := false
	for index, r := range value {
		if escape {
			escape = false
			continue
		}
		switch r {
		case '\\':
			if inString {
				escape = true
			}
		case '\'':
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString {
				depth--
				if depth == 0 {
					return index
				}
			}
		}
	}
	return -1
}

func parseIdentifierList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.Trim(strings.TrimSpace(part), "`\""))
	}
	return result
}

func parseValueRows(value string) ([][]string, error) {
	var rows [][]string
	for {
		value = strings.TrimSpace(value)
		if value == "" {
			return rows, nil
		}
		if value[0] != '(' {
			return nil, errors.New("VALUES section does not start with '('")
		}
		end := findMatchingParen(value)
		if end < 0 {
			return nil, errors.New("unterminated VALUES row")
		}
		row, err := parseValueList(value[1:end])
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
		value = strings.TrimSpace(value[end+1:])
		if value == "" {
			return rows, nil
		}
		if value[0] == ',' {
			value = value[1:]
			continue
		}
		return rows, nil
	}
}

func parseValueList(value string) ([]string, error) {
	var result []string
	var builder strings.Builder
	inString := false
	escape := false

	flush := func() {
		item := strings.TrimSpace(builder.String())
		if strings.EqualFold(item, "null") {
			item = ""
		}
		result = append(result, item)
		builder.Reset()
	}

	for _, r := range value {
		if escape {
			builder.WriteRune(unescapeRune(r))
			escape = false
			continue
		}
		switch r {
		case '\\':
			if inString {
				escape = true
			} else {
				builder.WriteRune(r)
			}
		case '\'':
			inString = !inString
		case ',':
			if inString {
				builder.WriteRune(r)
			} else {
				flush()
			}
		default:
			builder.WriteRune(r)
		}
	}
	if inString {
		return nil, errors.New("unterminated SQL string literal")
	}
	flush()
	return result, nil
}

func unescapeRune(r rune) rune {
	switch r {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case '0':
		return '\x00'
	default:
		return r
	}
}

func trackedTables() []string {
	return []string{
		"isppackages",
		"users",
		"ftp_users",
		"webdomain",
		"webdomain_alias",
		"webdomain_ipaddr",
		"db_server",
		"db_mysql_servers",
		"db_assign",
		"db_users_password",
		"emaildomain",
		"email",
		"domain",
	}
}
