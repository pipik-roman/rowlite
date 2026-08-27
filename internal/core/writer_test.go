package core_test

import (
	"context"
	"os"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

func TestStreamTransformTable_AppendAndTransform(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_test_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `# Title
Some notes.

| id | name |
| --- | --- |
| 1 | Alice |

## Footer
Footer note.`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	cfg, _, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeNone)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}

	transformer := func(physicalRowIdx int, cells []string) ([]core.RowAction, error) {
		if cells != nil {
			return []core.RowAction{{Type: core.ActionKeep}}, nil
		}
		// Boundary event: append new row
		return []core.RowAction{
			{
				Type:  core.ActionInsert,
				Cells: []string{"2", "Bob"},
			},
		}, nil
	}

	_, err = core.StreamTransformTable(context.Background(), tmpFile.Name(), cfg, transformer)
	if err != nil {
		t.Fatalf("StreamTransformTable() error = %v", err)
	}

	bytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	wantContent := `# Title
Some notes.

| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Bob |

## Footer
Footer note.
`
	if string(bytes) != wantContent {
		t.Errorf("content got = %q, want %q", string(bytes), wantContent)
	}
}

func TestAtomicWriteTable(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_test_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	headers := []string{"id", "name"}
	alignments := []string{"---", "---"}
	rows := [][]string{
		{"1", "Alice"},
		{"2", "Bob"},
	}

	err = core.AtomicWriteTable(context.Background(), tmpFile.Name(), core.TableConfig{Headers: headers, Alignments: alignments}, rows)
	if err != nil {
		t.Fatalf("AtomicWriteTable() error = %v", err)
	}

	bytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	wantContent := `| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Bob |
`
	if string(bytes) != wantContent {
		t.Errorf("content got = %q, want %q", string(bytes), wantContent)
	}
}
