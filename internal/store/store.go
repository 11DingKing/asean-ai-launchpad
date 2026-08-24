package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
	"github.com/11DingKing/asean-ai-launchpad/internal/migrate"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: database path", domain.ErrInvalid)
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file:launchpad-memory?mode=memory&cache=shared"
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate.Apply(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database readiness: %w", err)
	}
	var one int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("database query readiness: %w", err)
	}
	return nil
}

func (s *Store) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func timestamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse database time %q: %w", value, err)
	}
	return parsed.UTC(), nil
}

func optionalTimestamp(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func classify(err error, action string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", action, domain.ErrNotFound)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") {
		return fmt.Errorf("%s: %w", action, domain.ErrAlreadyExists)
	}
	if strings.Contains(message, "foreign key constraint") || strings.Contains(message, "check constraint") || strings.Contains(message, "must be positive") {
		return fmt.Errorf("%s: %w", action, domain.ErrInvalid)
	}
	if strings.Contains(message, "database is locked") || strings.Contains(message, "busy") {
		return fmt.Errorf("%s: %w", action, domain.ErrConflict)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func requireChanged(result sql.Result, conflict error) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if count != 1 {
		return conflict
	}
	return nil
}
