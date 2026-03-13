package parser

import (
	"testing"
)

func TestParseBool(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// PRD required true values
		{name: "Yes",          input: "Yes",   want: true},
		{name: "TRUE upper",   input: "TRUE",  want: true},
		{name: "true lower",   input: "true",  want: true},

		// PRD required false values
		{name: "No",           input: "No",    want: false},
		{name: "FALSE upper",  input: "FALSE", want: false},
		{name: "false lower",  input: "false", want: false},
		{name: "empty string", input: "",      want: false},

		// Edge cases — unrecognised values default to false (safe default)
		{name: "yes lowercase",  input: "yes",  want: false},
		{name: "no lowercase",   input: "no",   want: false},
		{name: "random string",  input: "maybe",want: false},
		{name: "1",              input: "1",    want: false},
		{name: "0",              input: "0",    want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseBool(tc.input)
			if got != tc.want {
				t.Errorf("ParseBool(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
