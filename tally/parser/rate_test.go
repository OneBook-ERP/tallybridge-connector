package parser

import (
	"testing"
)

func TestParseRate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantRate float64
		wantUOM  string
		wantErr  bool
	}{
		// PRD required formats
		{name: "no space after slash",   input: "55000.00/Nos",   wantRate: 55000.0, wantUOM: "Nos"},
		{name: "space after slash",      input: "55000.00/ Nos",  wantRate: 55000.0, wantUOM: "Nos"},

		// Additional real-world Tally rate variants
		{name: "per Kg",                 input: "150.50/Kg",      wantRate: 150.50,  wantUOM: "Kg"},
		{name: "integer rate",           input: "500/Nos",        wantRate: 500.0,   wantUOM: "Nos"},
		{name: "space before slash",     input: "250.00 /Mtr",    wantRate: 250.0,   wantUOM: "Mtr"},
		{name: "spaces around slash",    input: "1000.00 / Ltr",  wantRate: 1000.0,  wantUOM: "Ltr"},

		// Error cases
		{name: "empty input",            input: "",      wantErr: true},
		{name: "no slash separator",     input: "500Nos", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rate, uom, err := ParseRate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseRate(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRate(%q): unexpected error: %v", tc.input, err)
				return
			}
			if rate != tc.wantRate {
				t.Errorf("ParseRate(%q) rate = %v, want %v", tc.input, rate, tc.wantRate)
			}
			if uom != tc.wantUOM {
				t.Errorf("ParseRate(%q) uom = %q, want %q", tc.input, uom, tc.wantUOM)
			}
		})
	}
}
