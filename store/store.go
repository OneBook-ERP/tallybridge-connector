// Package store manages the connector's local SQLite database.
// It embeds the SQL schema and runs migrations automatically on first open.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

// Store wraps the SQLite connection and exposes typed accessors.
type Store struct {
	db *sql.DB
}

// InitDB opens (or creates) the SQLite database at path, applies the embedded
// schema, and returns a ready-to-use Store. Call Close() when done.
func InitDB(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}

	// Single writer, multiple reader safe for our single-process connector.
	db.SetMaxOpenConns(1)

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// runMigrations executes the embedded schema SQL.
// All CREATE TABLE statements use IF NOT EXISTS, so this is safe to call on
// every startup — it is a no-op when the schema is already present.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	return nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
