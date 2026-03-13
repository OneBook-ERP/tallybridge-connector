package store

import "fmt"

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
// Only call this on successful sync completion — never on partial or failed sync.
func (s *Store) SetLastSyncAt(collection, timestamp string) error {
	res, err := s.db.Exec(
		`UPDATE sync_state SET last_sync_at = ? WHERE collection = ?`,
		timestamp, collection,
	)
	if err != nil {
		return fmt.Errorf("store: SetLastSyncAt: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: SetLastSyncAt: unknown collection %q", collection)
	}
	return nil
}
