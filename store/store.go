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

// runMigrations executes the embedded schema SQL and any incremental
// migrations needed for existing databases.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	if err := migrateAddCompanyIDToMappingConfig(db); err != nil {
		return fmt.Errorf("migrate mapping_config: %w", err)
	}
	return nil
}

// migrateAddCompanyIDToMappingConfig upgrades the old single-column
// mapping_config table (PRIMARY KEY section) to the new composite schema
// (PRIMARY KEY company_id, section). Safe to call on every startup —
// exits immediately if the new schema is already in place.
func migrateAddCompanyIDToMappingConfig(db *sql.DB) error {
	// Check whether the company_id column already exists.
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('mapping_config') WHERE name = 'company_id'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // already migrated
	}

	// Recreate the table with the new schema, dropping all old rows
	// (they were CPT-specific and must not bleed into new companies).
	_, err := db.Exec(`
		DROP TABLE IF EXISTS mapping_config;
		CREATE TABLE mapping_config (
			company_id  TEXT NOT NULL,
			section     TEXT NOT NULL,
			config      TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (company_id, section)
		);
	`)
	return err
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
