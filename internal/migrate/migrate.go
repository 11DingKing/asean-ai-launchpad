package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed sql/*.sql
var files embed.FS

type migration struct {
	Version int
	Name    string
	SQL     string
}

func Apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	available, err := load()
	if err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	for version := range applied {
		found := false
		for _, item := range available {
			if item.Version == version {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("database contains unknown migration version %d", version)
		}
	}
	for _, item := range available {
		if applied[item.Version] {
			continue
		}
		if err := applyOne(ctx, db, item); err != nil {
			return err
		}
	}
	return nil
}

func load() ([]migration, error) {
	entries, err := fs.ReadDir(files, "sql")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	items := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		version, err := strconv.Atoi(parts[0])
		if err != nil || version <= 0 || len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration name %q", entry.Name())
		}
		data, err := files.ReadFile("sql/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		items = append(items, migration{Version: version, Name: entry.Name(), SQL: string(data)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Version < items[j].Version })
	for i, item := range items {
		if item.Version != i+1 {
			return nil, fmt.Errorf("migration versions must be contiguous from one, found %d", item.Version)
		}
	}
	return items, nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	result := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		result[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return result, nil
}

func applyOne(ctx context.Context, db *sql.DB, item migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.Version, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, item.SQL); err != nil {
		return fmt.Errorf("execute migration %d: %w", item.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.Version, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, item.Version, item.Name); err != nil {
		return fmt.Errorf("record migration %d after schema commit: %w", item.Version, err)
	}
	return nil
}
