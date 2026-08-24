package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrationAndLedgerCommitAtomically(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "migration-atomicity.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create migration ledger: %v", err)
	}
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER reject_migration_ledger BEFORE INSERT ON schema_migrations BEGIN SELECT RAISE(ABORT, 'ledger writes blocked'); END`); err != nil {
		t.Fatalf("install ledger policy: %v", err)
	}
	item := migration{Version: 1, Name: "001_accelerator_allocations.sql", SQL: `CREATE TABLE accelerator_allocations (id TEXT PRIMARY KEY, site_id TEXT NOT NULL)`}
	firstErr := applyOne(ctx, database, item)

	var schemaTables int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='accelerator_allocations'`).Scan(&schemaTables); err != nil {
		t.Fatalf("inspect schema after failed ledger write: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DROP TRIGGER reject_migration_ledger`); err != nil {
		t.Fatalf("remove ledger policy: %v", err)
	}
	retryErr := applyOne(ctx, database, item)
	var ledgerRows int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, item.Version).Scan(&ledgerRows); err != nil {
		t.Fatalf("inspect migration ledger: %v", err)
	}
	if firstErr == nil || schemaTables != 0 || retryErr != nil || ledgerRows != 1 {
		t.Fatalf("migration and ledger diverged: first_err=%v schema_tables=%d retry_err=%v ledger_rows=%d", firstErr, schemaTables, retryErr, ledgerRows)
	}
}
