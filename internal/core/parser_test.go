package core_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/roman-pipik/rowlite/internal/core"
)

func TestSplitGFMLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    []string
		wantErr bool
	}{
		{
			name:    "simple row",
			line:    "| col1 | col2 |",
			want:    []string{"col1", "col2"},
			wantErr: false,
		},
		{
			name:    "escaped pipe",
			line:    `| col1 \| details | col2 |`,
			want:    []string{"col1 | details", "col2"},
			wantErr: false,
		},
		{
			name:    "escaped backslash",
			line:    `| col1 \\ details | col2 |`,
			want:    []string{"col1 \\ details", "col2"},
			wantErr: false,
		},
		{
			name:    "missing leading pipe",
			line:    "col1 | col2 |",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing trailing pipe",
			line:    "| col1 | col2",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.SplitGFMLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("SplitGFMLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitGFMLine() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatGFMLine(t *testing.T) {
	tests := []struct {
		name  string
		cells []string
		want  string
	}{
		{
			name:  "simple",
			cells: []string{"col1", "col2"},
			want:  "| col1 | col2 |",
		},
		{
			name:  "with pipe",
			cells: []string{"col1 | details", "col2"},
			want:  `| col1 \| details | col2 |`,
		},
		{
			name:  "with backslash",
			cells: []string{"col1 \\ details", "col2"},
			want:  `| col1 \\ details | col2 |`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := core.FormatGFMLine(tt.cells)
			if got != tt.want {
				t.Errorf("FormatGFMLine() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSeparatorLine(t *testing.T) {
	tests := []struct {
		name  string
		cells []string
		want  bool
	}{
		{
			name:  "dashes only",
			cells: []string{"---", "---"},
			want:  true,
		},
		{
			name:  "alignment markers",
			cells: []string{":---", "---:"},
			want:  true,
		},
		{
			name:  "not a separator",
			cells: []string{"hello", "world"},
			want:  false,
		},
		{
			name:  "empty cell",
			cells: []string{"", "---"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := core.IsSeparatorLine(tt.cells)
			if got != tt.want {
				t.Errorf("IsSeparatorLine() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateSeparatorLine(t *testing.T) {
	tests := []struct {
		name    string
		cells   []string
		wantErr string
	}{
		{
			name:    "plain hyphens",
			cells:   []string{"---", "------"},
			wantErr: "",
		},
		{
			name:    "alignment colons",
			cells:   []string{":---", ":---:", "---:"},
			wantErr: "",
		},
		{
			name:    "empty cell on column 2",
			cells:   []string{"---", ""},
			wantErr: "column 2 is empty and missing hyphens ('-')",
		},
		{
			name:    "invalid character",
			cells:   []string{"---", "---abc---"},
			wantErr: "column 2 contains invalid character \"a\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := core.ValidateSeparatorLine(tt.cells)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateSeparatorLine() unexpected error = %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateSeparatorLine() error = %v, want substring %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestScanTableHeaders(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_test_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `Some preamble text.
| id | item_name | priority |
|---|---|---|
| 1 | Test | high |
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	headers, alignments, lineNum, err := core.ScanTableHeaders(context.Background(), tmpFile.Name())
	if err != nil {
		t.Fatalf("ScanTableHeaders() error = %v", err)
	}

	wantHeaders := []string{"id", "item_name", "priority"}
	wantAlignments := []string{"---", "---", "---"}
	if !reflect.DeepEqual(headers, wantHeaders) {
		t.Errorf("headers got = %v, want %v", headers, wantHeaders)
	}
	if !reflect.DeepEqual(alignments, wantAlignments) {
		t.Errorf("alignments got = %v, want %v", alignments, wantAlignments)
	}
	if lineNum != 2 {
		t.Errorf("lineNum got = %d, want %d", lineNum, 2)
	}
}

func TestScanTableHeaders_MalformedSeparator(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_test_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `Header text
| Id | Tags | Idea | Resources |
|----|------|------| |
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	_, _, _, err = core.ScanTableHeaders(context.Background(), tmpFile.Name())
	if err == nil {
		t.Fatalf("Expected error for malformed separator row, got nil")
	}
	if !strings.Contains(err.Error(), "invalid separator row at line 3") || !strings.Contains(err.Error(), "column 4 is empty") {
		t.Errorf("ScanTableHeaders() error = %v, want detailed line 3 error", err)
	}
}
