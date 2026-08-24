package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "migrate.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("ping sqlite: %v", err)
	}
	return db
}

// TestApplyRollsBackWhenLedgerWriteRejected reproduces the regional node upgrade
// scenario where the migration ledger INSERT is rejected by a database policy
// after the schema change has run. The schema change must roll back together
// with the rejected ledger write, so lifting the policy and retrying succeeds
// instead of failing on leftover "table already exists" objects.
func TestApplyRollsBackWhenLedgerWriteRejected(t *testing.T) {
	ctx := context.Background()
	db := openMigrationDB(t)

	// Pre-create the migration ledger with an extra NOT NULL column so the
	// version-registration INSERT (which omits that column) is rejected,
	// simulating a policy that denies ledger writes during an upgrade.
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL, policy_token TEXT NOT NULL)`); err != nil {
		t.Fatalf("seed policy ledger: %v", err)
	}

	// Apply must fail because the ledger write is rejected.
	if err := Apply(ctx, db); err == nil {
		t.Fatal("expected migration to fail when ledger write is rejected")
	}

	// No schema objects from the migration may remain: the DDL must have been
	// rolled back together with the rejected ledger write.
	var leftover int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&leftover); err != nil {
		t.Fatalf("count leftover users table: %v", err)
	}
	if leftover != 0 {
		t.Fatalf("migration DDL survived a rejected ledger write; users table count=%d", leftover)
	}

	// Lift the policy by restoring the standard ledger schema, then retry.
	if _, err := db.ExecContext(ctx, `DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("drop policy ledger: %v", err)
	}
	if err := Apply(ctx, db); err != nil {
		t.Fatalf("retry after lifting policy: %v", err)
	}

	var applied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count applied after retry: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied migration count=%d, want 2", applied)
	}
	var users int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&users); err != nil {
		t.Fatalf("count users table after retry: %v", err)
	}
	if users != 1 {
		t.Fatalf("users table count=%d, want 1 after retry", users)
	}
}
