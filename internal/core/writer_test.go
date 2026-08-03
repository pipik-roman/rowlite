package core_test

import (
	"context"
	"os"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

func TestAppendRowDirectly(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_test_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `| id | name |
|---|---|
| 1 | Alice |`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	err = core.AppendRowDirectly(context.Background(), tmpFile.Name(), []string{"2", "Bob"})
	if err != nil {
		t.Fatalf("AppendRowDirectly() error = %v", err)
	}

	bytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	wantContent := `| id | name |
|---|---|
| 1 | Alice |
| 2 | Bob |
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
