package store

import (
	"fmt"
	"time"
)

// validEntityTypes lists the master entity types tracked by master_hashes.
var validEntityTypes = map[string]bool{
	"ledger":      true,
	"stock_item":  true,
	"stock_group": true,
	"uom":         true,
	"godown":      true,
}

// GetMasterHash returns the stored content hash for a given entity type and GUID.
// Returns ("", nil) if no record exists yet.
func (s *Store) GetMasterHash(entityType, guid string) (string, error) {
	if !validEntityTypes[entityType] {
		return "", fmt.Errorf("store: GetMasterHash: unknown entity type %q", entityType)
	}
	var hash string
	row := s.db.QueryRow(
		`SELECT content_hash FROM master_hashes WHERE entity_type = ? AND entity_guid = ?`,
		entityType, guid,
	)
	if err := row.Scan(&hash); err != nil {
		// sql.ErrNoRows — entity not yet tracked, return empty string (not an error)
		return "", nil
	}
	return hash, nil
}

// SetMasterHash upserts the content hash for a given entity type and GUID.
// Call this only after the entity has been successfully sent to the cloud.
func (s *Store) SetMasterHash(entityType, guid, hash string) error {
	if !validEntityTypes[entityType] {
		return fmt.Errorf("store: SetMasterHash: unknown entity type %q", entityType)
	}
	_, err := s.db.Exec(
		`INSERT INTO master_hashes (entity_type, entity_guid, content_hash, synced_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(entity_type, entity_guid) DO UPDATE
		   SET content_hash = excluded.content_hash,
		       synced_at    = excluded.synced_at`,
		entityType, guid, hash, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: SetMasterHash: %w", err)
	}
	return nil
}

// MasterEntity is a (GUID, Hash) pair used for bulk changed-detection.
type MasterEntity struct {
	GUID string
	Hash string
}

// GetChangedMasters returns the GUIDs from entities whose stored hash differs
// from the supplied hash (or whose GUID has never been seen before).
// Use this before sending masters to cloud to skip unchanged entities.
func (s *Store) GetChangedMasters(entityType string, entities []MasterEntity) ([]string, error) {
	if !validEntityTypes[entityType] {
		return nil, fmt.Errorf("store: GetChangedMasters: unknown entity type %q", entityType)
	}

	var changed []string
	for _, e := range entities {
		stored, err := s.GetMasterHash(entityType, e.GUID)
		if err != nil {
			return nil, fmt.Errorf("store: GetChangedMasters: %w", err)
		}
		if stored != e.Hash {
			changed = append(changed, e.GUID)
		}
	}
	return changed, nil
}
