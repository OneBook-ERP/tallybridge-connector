package store

import "fmt"

// GetMappingConfig returns the JSON string for a config section scoped to the given company.
// Returns "{}" if not found.
func (s *Store) GetMappingConfig(companyID, section string) string {
	var val string
	row := s.db.QueryRow(
		`SELECT config FROM mapping_config WHERE company_id = ? AND section = ?`,
		companyID, section,
	)
	if err := row.Scan(&val); err != nil {
		return "{}"
	}
	return val
}

// SetMappingConfig upserts the JSON config for a section scoped to the given company.
func (s *Store) SetMappingConfig(companyID, section, jsonConfig string) error {
	_, err := s.db.Exec(
		`INSERT INTO mapping_config (company_id, section, config) VALUES (?, ?, ?)
		 ON CONFLICT(company_id, section) DO UPDATE SET config = excluded.config`,
		companyID, section, jsonConfig,
	)
	if err != nil {
		return fmt.Errorf("store: SetMappingConfig[%s/%s]: %w", companyID, section, err)
	}
	return nil
}

// GetAllMappingConfigs returns a map of section → JSON string for the given company.
func (s *Store) GetAllMappingConfigs(companyID string) map[string]string {
	rows, err := s.db.Query(
		`SELECT section, config FROM mapping_config WHERE company_id = ?`,
		companyID,
	)
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

// ClearMappingConfig removes all cached mapping rows for a company.
// Called when a company is deregistered or switches to a fresh config.
func (s *Store) ClearMappingConfig(companyID string) error {
	_, err := s.db.Exec(`DELETE FROM mapping_config WHERE company_id = ?`, companyID)
	return err
}
