// Package database opens the local SQLite database and runs the embedded
// migrations on every start (back_spec.md §4).
package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens the database file. modernc.org/sqlite is a pure-Go driver, so the
// binary builds with CGO_ENABLED=0 on every target.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection keeps writers serialised and removes SQLITE_BUSY noise.
	// The workload (a single-user local app) is nowhere near this limit.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Migrate applies all pending migrations.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations)
	goose.SetLogger(log.New(io.Discard, "", 0))
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
