// Package store is the local-first persistence layer: SQLite (modernc, no cgo)
// in WAL mode with an FTS5 index for instant search. It is the source of truth;
// the sync engine reconciles it against providers. Goose runs the embedded
// migrations at Open; typed queries are the sqlc-generated package in ./db.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	migfs "github.com/atterpac/email/db"
	"github.com/atterpac/email/internal/events"
	gen "github.com/atterpac/email/internal/store/db"
)

// Store wraps the SQLite database and the generated query set.
type Store struct {
	db  *sql.DB
	q   *gen.Queries
	bus *events.Bus
}

// dsn builds a modernc connection string with the pragmas we always want:
// WAL for concurrent readers, a busy timeout, and foreign keys on.
func dsn(path string) string {
	return "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(on)"
}

// Open opens (or creates) the database at path and migrates it to the latest
// schema version.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, q: gen.New(db), bus: events.NewBus()}, nil
}

// Events returns the store's changefeed bus for subscribing to mutations.
func (s *Store) Events() *events.Bus { return s.bus }

func migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migfs.Migrations)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Queries returns the generated query set bound to the database.
func (s *Store) Queries() *gen.Queries { return s.q }

// Tx runs fn inside a transaction with a transaction-scoped query set.
func (s *Store) Tx(ctx context.Context, fn func(*gen.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(s.q.WithTx(tx)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DB exposes the underlying handle for raw queries (e.g. FTS search).
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }
