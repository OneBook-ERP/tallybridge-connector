package store

import (
	"fmt"
)

var validCollections = map[string]bool{
	"vouchers":    true,
	"ledgers":     true,
	"stock_items": true,
	"masters":     true,
}

// GetLastSyncAt returns the yyyymmddHHmmss timestamp for the given collection,
// or an empty string if the collection has never been synced.
// collection: "vouchers" | "ledgers" | "stock_items"
func (s *Store) GetLastSyncAt(collection string) string {
	var ts string
	row := s.db.QueryRow(
		`SELECT last_sync_at FROM sync_state WHERE collection = ?`, collection,
	)
	if err := row.Scan(&ts); err != nil {
		return ""
	}
	return ts
}

// SetLastSyncAt updates the last_sync_at timestamp for the given collection.
// Uses upsert so it works even if the seed row was accidentally deleted.
// Only call this on successful sync completion — never on partial or failed sync.
func (s *Store) SetLastSyncAt(collection, timestamp string) error {
	if !validCollections[collection] {
		return fmt.Errorf("store: SetLastSyncAt: unknown collection %q", collection)
	}
	_, err := s.db.Exec(
		`INSERT INTO sync_state (collection, last_sync_at) VALUES (?, ?)
		 ON CONFLICT(collection) DO UPDATE SET last_sync_at = excluded.last_sync_at`,
		collection, timestamp,
	)
	if err != nil {
		return fmt.Errorf("store: SetLastSyncAt: %w", err)
	}
	return nil
}
