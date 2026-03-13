package parser

import (
	"testing"
)

func TestParseQuantity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantQty float64
		wantUOM string
		wantErr bool
	}{
		// PRD required formats
		{name: "whole number Nos",   input: "1 Nos",    wantQty: 1.0,   wantUOM: "Nos"},
		{name: "leading space Kg",   input: " 2.5 Kg",  wantQty: 2.5,   wantUOM: "Kg"},
		{name: "zero quantity",      input: "0 Nos",    wantQty: 0.0,   wantUOM: "Nos"},

		// Additional UOMs seen in Indian GST invoices
		{name: "litres",             input: "10 Ltr",   wantQty: 10.0,  wantUOM: "Ltr"},
		{name: "dozens",             input: "2.5 Doz",  wantQty: 2.5,   wantUOM: "Doz"},
		{name: "metres",             input: "100 Mtr",  wantQty: 100.0, wantUOM: "Mtr"},

		// Error cases
		{name: "empty input",        input: "",      wantErr: true},
		{name: "no space separator", input: "10Nos", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qty, uom, err := ParseQuantity(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseQuantity(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseQuantity(%q): unexpected error: %v", tc.input, err)
				return
			}
			if qty != tc.wantQty {
				t.Errorf("ParseQuantity(%q) qty = %v, want %v", tc.input, qty, tc.wantQty)
			}
			if uom != tc.wantUOM {
				t.Errorf("ParseQuantity(%q) uom = %q, want %q", tc.input, uom, tc.wantUOM)
			}
		})
	}
}
