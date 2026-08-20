package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
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

func TestMCPConfigCache_WithETag(t *testing.T) {
	cache := NewTableConfigCache()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "mcp_cache_test.md")
	content := "| id | title |\n| --- | --- |\n| 1 | Test |\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	ctx := context.Background()

	// 1. Initial parse and cache
	cfg1, rows1, err := cache.GetOrParse(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("GetOrParse failed: %v", err)
	}
	if cfg1.ETag == "" {
		t.Errorf("expected non-empty ETag in TableConfig")
	}
	if len(rows1) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows1))
	}

	// 2. Cache hit returns same ETag
	cfg2, _, err := cache.GetOrParse(ctx, filePath, core.ContentModeNone)
	if err != nil {
		t.Fatalf("GetOrParse cache hit failed: %v", err)
	}
	if cfg2.ETag != cfg1.ETag {
		t.Errorf("expected cached ETag %q, got %q", cfg1.ETag, cfg2.ETag)
	}
}
