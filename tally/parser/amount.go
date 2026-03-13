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
//	"-215476.00"     → -215476.00
//	"215476.00 Cr"   →  -215476.00  (credit = negative in accounting convention)
//	"215476.00 Dr"   →  +215476.00  (debit  = positive)
//	"215476.00-"     →  -215476.00  (trailing minus)
//	""               →   0.0
func ParseAmount(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0.0
	}

	negate := false

	// Suffix " Cr" → credit, which Tally represents as a positive balance
	// but we store as negative (liability/income convention).
	if strings.HasSuffix(s, " Cr") {
		negate = true
		s = strings.TrimSuffix(s, " Cr")
	} else if strings.HasSuffix(s, " Dr") {
		// Debit is already positive; strip suffix only.
		s = strings.TrimSuffix(s, " Dr")
	} else if strings.HasSuffix(s, "-") {
		// Trailing minus sign variant.
		negate = true
		s = strings.TrimSuffix(s, "-")
	}

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
