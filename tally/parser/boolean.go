package parser

// ParseBool converts a Tally boolean string to a Go bool.
//
// Tally uses:
//
//	"Yes" | "TRUE" | "true"          → true
//	"No"  | "FALSE" | "false" | ""   → false
//
// Any unrecognised value returns false (safe default).
func ParseBool(s string) bool {
	switch s {
	case "Yes", "TRUE", "true":
		return true
	default:
		return false
	}
}
