package parser

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantStr string    // ISO date "2006-01-02" or "" for zero
		wantErr bool
	}{
		// PRD required formats
		{name: "single digit day",    input: "1-Apr-2024",  wantStr: "2024-04-01"},
		{name: "two digit day",       input: "01-Apr-2024", wantStr: "2024-04-01"},
		{name: "import yyyymmdd",     input: "20240401",    wantStr: "2024-04-01"},
		{name: "empty returns zero",  input: "",            wantStr: ""},

		// Additional month variants
		{name: "January",            input: "1-Jan-2025",  wantStr: "2025-01-01"},
		{name: "March",              input: "31-Mar-2025", wantStr: "2025-03-31"},
		{name: "financial year end", input: "20250331",    wantStr: "2025-03-31"},
		{name: "December",           input: "31-Dec-2024", wantStr: "2024-12-31"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDate(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ParseDate(%q): expected error, got nil", tc.input)
				return
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ParseDate(%q): unexpected error: %v", tc.input, err)
				return
			}

			if tc.wantStr == "" {
				if !got.IsZero() {
					t.Errorf("ParseDate(%q) = %v, want zero time", tc.input, got)
				}
				return
			}

			want, _ := time.Parse("2006-01-02", tc.wantStr)
			want = want.UTC()
			if !got.Equal(want) {
				t.Errorf("ParseDate(%q) = %v, want %v", tc.input, got, want)
			}
		})
	}
}

func TestFormatTallyDate(t *testing.T) {
	ts := time.Date(2024, 4, 1, 15, 30, 45, 0, time.UTC)
	got := FormatTallyDate(ts)
	want := "20240401153045"
	if got != want {
		t.Errorf("FormatTallyDate = %q, want %q", got, want)
	}
}
