package store

import "fmt"

// GetMappingConfig returns the JSON string for a config section.
// Returns "{}" if not found.
func (s *Store) GetMappingConfig(section string) string {
	var val string
	row := s.db.QueryRow(`SELECT config FROM mapping_config WHERE section = ?`, section)
	if err := row.Scan(&val); err != nil {
		return "{}"
	}
	return val
}

// SetMappingConfig upserts the JSON config for a section.
func (s *Store) SetMappingConfig(section, jsonConfig string) error {
	_, err := s.db.Exec(
		`INSERT INTO mapping_config (section, config) VALUES (?, ?)
		 ON CONFLICT(section) DO UPDATE SET config = excluded.config`,
		section, jsonConfig,
	)
	if err != nil {
		return fmt.Errorf("store: SetMappingConfig[%s]: %w", section, err)
	}
	return nil
}

// GetAllMappingConfigs returns a map of section → JSON string for all sections.
func (s *Store) GetAllMappingConfigs() map[string]string {
	rows, err := s.db.Query(`SELECT section, config FROM mapping_config`)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var section, config string
		if err := rows.Scan(&section, &config); err == nil {
			result[section] = config
		}
	}
	return result
}
