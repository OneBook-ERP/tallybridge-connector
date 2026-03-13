package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseQuantity splits a Tally quantity string into numeric value and UOM.
//
// Formats:
//
//	"1 Nos"    → (1.0,   "Nos", nil)
//	" 2.5 Kg"  → (2.5,   "Kg",  nil)   ← leading space is trimmed first
//	"0 Nos"    → (0.0,   "Nos", nil)
func ParseQuantity(s string) (qty float64, uom string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", fmt.Errorf("ParseQuantity: empty input")
	}

	// Find the first space that separates numeric part from UOM.
	idx := strings.Index(s, " ")
	if idx < 0 {
		return 0, "", fmt.Errorf("ParseQuantity: no space separator in %q", s)
	}

	numPart := s[:idx]
	uomPart := strings.TrimSpace(s[idx+1:])

	qty, err = strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, "", fmt.Errorf("ParseQuantity: invalid number %q in %q: %w", numPart, s, err)
	}

	if uomPart == "" {
		return 0, "", fmt.Errorf("ParseQuantity: empty UOM in %q", s)
	}

	return qty, uomPart, nil
}
