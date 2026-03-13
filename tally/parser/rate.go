package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseRate splits a Tally rate string into numeric rate and UOM.
//
// Formats:
//
//	"55000.00/Nos"   → (55000.0, "Nos", nil)
//	"55000.00/ Nos"  → (55000.0, "Nos", nil)   ← space after slash trimmed
func ParseRate(s string) (rate float64, uom string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", fmt.Errorf("ParseRate: empty input")
	}

	idx := strings.Index(s, "/")
	if idx < 0 {
		return 0, "", fmt.Errorf("ParseRate: no '/' separator in %q", s)
	}

	numPart := strings.TrimSpace(s[:idx])
	uomPart := strings.TrimSpace(s[idx+1:])

	rate, err = strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, "", fmt.Errorf("ParseRate: invalid number %q in %q: %w", numPart, s, err)
	}

	if uomPart == "" {
		return 0, "", fmt.Errorf("ParseRate: empty UOM in %q", s)
	}

	return rate, uomPart, nil
}
