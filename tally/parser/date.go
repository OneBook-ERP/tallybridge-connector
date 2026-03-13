package parser

import (
	"strings"
	"time"
)

// Tally uses "1-Apr-2024" (export) and "20240401" (import format).
// Both must be handled.
var tallyDateLayouts = []string{
	"2-Jan-2006",  // "1-Apr-2024" or "01-Apr-2024"
	"20060102",    // "20240401" — Tally $$IsAlteredSince format
}

// ParseDate converts a Tally date string to a time.Time (UTC midnight).
// Returns (time.Time{}, nil) for empty input — callers treat zero value as null.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}

	for _, layout := range tallyDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}

	// Return zero + no error for graceful degradation; the caller decides
	// whether a zero time is acceptable for the field in question.
	return time.Time{}, nil
}

// FormatTallyDate converts a time.Time to the yyyymmddHHmmss string used
// in $$IsAlteredSince filter queries and in SQLite sync_state storage.
func FormatTallyDate(t time.Time) string {
	return t.UTC().Format("20060102150405")
}
