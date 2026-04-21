package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	backupMarkerTTL    = 6 * time.Hour
	mysqlBackupName    = "ispmgr-mysql-backup.sql"
	backupTimeFormat   = "02-Jan-2006-15-04-MST"
	backupMarkerPrefix = "db-backup-"
)

func ensureSourceBackup(ctx context.Context, cfg Config, raw rawSource) (string, error) {
	supportDir, stateDir, err := localBackupDirs()
	if err != nil {
		return "", err
	}

	if err := cleanupExpiredBackupMarkers(stateDir); err != nil {
		return "", err
	}

	sourceKey := sourceBackupKey(cfg)
	if sourceKey == "" {
		return "", nil
	}

	if backupPath, ok := existingBackupPath(stateDir, sourceKey); ok {
		return backupPath, nil
	}

	stamp := time.Now().UTC().Format(backupTimeFormat)
	backupDir := filepath.Join(supportDir, stamp)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}

	targetPath := filepath.Join(backupDir, backupFileName(cfg))
	if cfg.UseLocalMySQL {
		if err := createMySQLBackup(ctx, cfg.MySQLPassword, targetPath); err != nil {
			return "", err
		}
	} else {
		if err := copyLocalFile(cfg.DBFile, targetPath); err != nil {
			return "", err
		}
	}

	if err := writeBackupMarker(stateDir, sourceKey, targetPath); err != nil {
		return "", err
	}

	_ = raw
	return targetPath, nil
}

func localBackupDirs() (string, string, error) {
	home, err := userHomeDirHook()
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve user home directory for backup files: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", "", fmt.Errorf("failed to resolve user home directory for backup files: empty home path")
	}
	return filepath.Join(home, "support"), filepath.Join(home, ".ispdb"), nil
}

func sourceBackupKey(cfg Config) string {
	if cfg.UseLocalMySQL {
		return fmt.Sprintf("mysql:%s:%d/ispmgr", defaultMySQLHost, defaultMySQLPort)
	}
	if strings.TrimSpace(cfg.DBFile) == "" {
		return ""
	}
	return "file:" + filepath.Clean(cfg.DBFile)
}

func backupFileName(cfg Config) string {
	if cfg.UseLocalMySQL {
		return mysqlBackupName
	}
	return filepath.Base(cfg.DBFile)
}

func cleanupExpiredBackupMarkers(stateDir string) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), backupMarkerPrefix) || !strings.HasSuffix(entry.Name(), ".marker") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) <= backupMarkerTTL {
			continue
		}
		_ = os.Remove(filepath.Join(stateDir, entry.Name()))
	}
	return nil
}

func existingBackupPath(stateDir string, sourceKey string) (string, bool) {
	markerPath := backupMarkerPath(stateDir, sourceKey)
	info, err := os.Stat(markerPath)
	if err != nil {
		return "", false
	}
	if time.Since(info.ModTime()) > backupMarkerTTL {
		_ = os.Remove(markerPath)
		return "", false
	}

	content, err := os.ReadFile(markerPath)
	if err != nil {
		return "", false
	}
	backupPath := strings.TrimSpace(string(content))
	if backupPath == "" {
		return "", false
	}
	if _, err := os.Stat(backupPath); err != nil {
		return "", false
	}
	return backupPath, true
}

func writeBackupMarker(stateDir string, sourceKey string, backupPath string) error {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(backupMarkerPath(stateDir, sourceKey), []byte(backupPath+"\n"), 0600)
}

func backupMarkerPath(stateDir string, sourceKey string) string {
	sum := sha256.Sum256([]byte(sourceKey))
	return filepath.Join(stateDir, backupMarkerPrefix+hex.EncodeToString(sum[:])+".marker")
}

func copyLocalFile(sourcePath string, targetPath string) error {
	source, err := os.Open(filepath.Clean(sourcePath))
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer target.Close()

	if _, err := target.ReadFrom(source); err != nil {
		return err
	}
	return target.Close()
}

func createMySQLBackup(ctx context.Context, password string, targetPath string) error {
	dsn := fmt.Sprintf("root:%s@tcp(%s:%d)/ispmgr?charset=utf8mb4&parseTime=true&loc=Local", password, defaultMySQLHost, defaultMySQLPort)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := beginConsistentSnapshot(ctx, conn); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
	}()

	file, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := fmt.Fprintf(file, "-- ispdb backup of ispmgr from %s:%d at %s UTC\n\n", defaultMySQLHost, defaultMySQLPort, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := file.WriteString("SET FOREIGN_KEY_CHECKS=0;\n\n"); err != nil {
		return err
	}

	tables, err := allMySQLTables(ctx, conn)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if err := writeTableBackup(ctx, conn, file, table); err != nil {
			return err
		}
	}

	if _, err := file.WriteString("SET FOREIGN_KEY_CHECKS=1;\n"); err != nil {
		return err
	}

	return file.Close()
}

func allMySQLTables(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list MySQL tables for backup: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0, 32)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("failed to scan MySQL table name for backup: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed while reading MySQL table list for backup: %w", err)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no MySQL tables were found in ispmgr for backup")
	}
	return tables, nil
}

func writeTableBackup(ctx context.Context, conn *sql.Conn, file *os.File, table string) error {
	var tableName string
	var createStmt string
	if err := conn.QueryRowContext(ctx, fmt.Sprintf("SHOW CREATE TABLE `%s`", table)).Scan(&tableName, &createStmt); err != nil {
		return fmt.Errorf("failed to read CREATE TABLE for %s: %w", table, err)
	}

	if _, err := fmt.Fprintf(file, "DROP TABLE IF EXISTS `%s`;\n%s;\n", tableName, createStmt); err != nil {
		return err
	}

	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT * FROM `%s`", table))
	if err != nil {
		return fmt.Errorf("failed to read rows from %s: %w", table, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return err
		}
		rowValues := make([]string, len(columns))
		for i, value := range values {
			rowValues[i] = sqlValueLiteral(value)
		}
		if _, err := fmt.Fprintf(file, "INSERT INTO `%s` (`%s`) VALUES (%s);\n", tableName, strings.Join(columns, "`, `"), strings.Join(rowValues, ", ")); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = file.WriteString("\n")
	return err
}

func sqlValueLiteral(value any) string {
	if value == nil {
		return "NULL"
	}
	return quoteSQLString(sqlValueToString(value))
}

func quoteSQLString(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`'`, `\'`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return "'" + replacer.Replace(value) + "'"
}
