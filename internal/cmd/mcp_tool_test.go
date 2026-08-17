package cmd

import (
	"testing"
)

func TestParseIndexArg(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "string input",
			input:    "2..10",
			expected: "2..10",
		},
		{
			name:     "int input",
			input:    5,
			expected: "5",
		},
		{
			name:     "float64 input (json unmarshaling default for number)",
			input:    float64(3),
			expected: "3",
		},
		{
			name:     "slice of any",
			input:    []any{0, 2, 4},
			expected: "0,2,4",
		},
		{
			name:     "slice of int",
			input:    []int{1, 3, 5},
			expected: "1,3,5",
		},
		{
			name:     "slice of string",
			input:    []string{"0", "2", "4"},
			expected: "0,2,4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIndexArg(tt.input)
			if got != tt.expected {
				t.Errorf("parseIndexArg(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
