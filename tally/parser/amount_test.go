package parser

import (
	"testing"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  float64
	}{
		// PRD normalisation rules — all required edge cases
		{name: "plain negative",          input: "-215476.00",    want: -215476.00},
		{name: "Cr suffix negates",       input: "215476.00 Cr",  want: -215476.00},
		{name: "Dr suffix positive",      input: "215476.00 Dr",  want: 215476.00},
		{name: "trailing minus negates",  input: "215476.00-",    want: -215476.00},
		{name: "empty string zero",       input: "",              want: 0.0},

		// Additional real-world variants
		{name: "zero value",              input: "0.00",          want: 0.0},
		{name: "positive no suffix",      input: "100000.50",     want: 100000.50},
		{name: "whitespace only",         input: "   ",           want: 0.0},
		{name: "small credit",            input: "1.50 Cr",       want: -1.50},
		{name: "small debit",             input: "1.50 Dr",       want: 1.50},
		{name: "GST amount Cr",           input: "38476.27 Cr",   want: -38476.27},

		// Negative already signed + Cr (Tally sometimes emits this on returns)
		{name: "already signed Cr amount",input: "-1000.00 Cr",  want: 1000.00},

		// Stock qty formats: "(-) qty UOM" and "qty UOM" (opening balance in item master)
		{name: "stock qty negative UOM",  input: "(-) 17600.000 KGS", want: -17600.0},
		{name: "stock qty positive UOM",  input: "25000.000 KGS",     want: 25000.0},
		{name: "stock qty with commas",   input: "(-) 17,600.000 KGS",want: -17600.0},
		{name: "stock qty commas pos",    input: "1,250.500 KGS",     want: 1250.5},
		{name: "stock qty zero UOM",      input: "0 KGS",             want: 0.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAmount(tc.input)
			if got != tc.want {
				t.Errorf("ParseAmount(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
