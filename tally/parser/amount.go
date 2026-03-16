// Package parser contains pure normalisation functions for Tally XML output.
// No I/O, no global state — every function is safe to call concurrently.
package parser

import (
	"strconv"
	"strings"
)

// ParseAmount normalises a Tally amount string to a float64.
//
// Tally amount formats encountered in the wild:
//
//	"-215476.00"           → -215476.00
//	"215476.00 Cr"         →  -215476.00  (credit = negative in accounting convention)
//	"215476.00 Dr"         →  +215476.00  (debit  = positive)
//	"215476.00-"           →  -215476.00  (trailing minus)
//	"(-) 17,600.000 KGS"   →  -17600.0   (stock qty negative with UOM suffix)
//	"17,600.000 KGS"       →  +17600.0   (stock qty with UOM suffix)
//	""                     →   0.0
func ParseAmount(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0.0
	}

	negate := false

	// Tally stock qty negative format: "(-) 17,600.000 KGS"
	if strings.HasPrefix(s, "(-)") {
		negate = true
		s = strings.TrimSpace(strings.TrimPrefix(s, "(-)"))
	}

	// Suffix " Cr" → credit, which Tally represents as a positive balance
	// but we store as negative (liability/income convention).
	switch {
	case strings.HasSuffix(s, " Cr"):
		negate = true
		s = strings.TrimSuffix(s, " Cr")
	case strings.HasSuffix(s, " Dr"):
		// Debit is already positive; strip suffix only.
		s = strings.TrimSuffix(s, " Dr")
	case strings.HasSuffix(s, "-"):
		// Trailing minus sign variant.
		negate = true
		s = strings.TrimSuffix(s, "-")
	}

	// Strip UOM suffix (e.g., "17600.000 KGS" → "17600.000").
	// Tally appends the unit of measure after a space for stock quantities.
	if idx := strings.IndexByte(s, ' '); idx != -1 {
		s = s[:idx]
	}

	// Strip thousands-separator commas (e.g., "17,600.000" → "17600.000").
	s = strings.ReplaceAll(s, ",", "")

	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0
	}

	if negate {
		f = -f
	}
	return f
}
