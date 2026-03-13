package store

import "fmt"

// GetConfig returns the value for key, or "" if the key is unknown.
func (s *Store) GetConfig(key string) string {
	var val string
	row := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, key)
	if err := row.Scan(&val); err != nil {
		return ""
	}
	return val
}

// SetConfig upserts a key-value pair in the config table.
func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("store: SetConfig: %w", err)
	}
	return nil
}
