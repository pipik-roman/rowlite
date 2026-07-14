package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIntegration_FullFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_integration_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tablePath := filepath.Join(tempDir, "backlog.md")
	journalPath := filepath.Join(tempDir, "journal.md")

	// 1. Initialize table
	cols := []string{"id", "name", "priority", "created_at", "modified_at"}
	cfg := TableConfig{
		Rowlite: RowliteConfig{
			Unique:      []string{"id"},
			Sort:        "id:asc",
			AuditInsert: true,
			AuditEdit:   true,
			JournalLog:  journalPath,
		},
	}

	err = InitTable(tablePath, cols, cfg)
	if err != nil {
		t.Fatalf("Failed to initialize table: %v", err)
	}

	// Verify structure is set up
	_, rows, headers, alignments, err := ParseFrontmatterAndContent(tablePath)
	if err != nil {
		t.Fatalf("Failed to parse initialized table: %v", err)
	}

	if !reflect.DeepEqual(headers, cols) {
		t.Errorf("headers mismatch: got = %v, want %v", headers, cols)
	}
	if len(alignments) != len(cols) {
		t.Errorf("alignments size mismatch: got = %d, want %d", len(alignments), len(cols))
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows initially, got %d", len(rows))
	}

	// 2. Append rows (unsorted input order, expect auto-sorting on write because sort is configured)
	err = AppendRow(tablePath, map[string]string{"id": "10", "name": "Alice", "priority": "high"})
	if err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}
	err = AppendRow(tablePath, map[string]string{"id": "2", "name": "Bob", "priority": "medium"})
	if err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}
	err = AppendRow(tablePath, map[string]string{"id": "1", "name": "Charlie", "priority": "low"})
	if err != nil {
		t.Fatalf("Append 3 failed: %v", err)
	}

	// 3. Verify unique keys check (duplicate id should fail)
	err = AppendRow(tablePath, map[string]string{"id": "2", "name": "Duplicate Bob", "priority": "medium"})
	if err == nil {
		t.Error("expected duplicate key error for id=2, but got none")
	}

	// 4. Verify sorting (id:asc) and audit timestamp creation
	_, finalRows, finalHeaders, _, err := ParseFrontmatterAndContent(tablePath)
	if err != nil {
		t.Fatalf("Failed to parse final table: %v", err)
	}

	if len(finalRows) != 3 {
		t.Fatalf("expected 3 rows, got = %d", len(finalRows))
	}

	// Expect order to be id=1, 2, 10
	expectedIds := []string{"1", "2", "10"}
	expectedNames := []string{"Charlie", "Bob", "Alice"}
	for idx, row := range finalRows {
		if row[0] != expectedIds[idx] {
			t.Errorf("row %d id mismatch: got = %s, want %s", idx, row[0], expectedIds[idx])
		}
		if row[1] != expectedNames[idx] {
			t.Errorf("row %d name mismatch: got = %s, want %s", idx, row[1], expectedNames[idx])
		}
		// Verify audit timestamps are stamped (not empty)
		if row[3] == "" || row[4] == "" {
			t.Errorf("row %d auditing timestamps (created_at/modified_at) are empty", idx)
		}
	}

	// 5. Verify journal log was populated
	journalBytes, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("Failed to read journal log: %v", err)
	}
	journalStr := string(journalBytes)
	if !strings.Contains(journalStr, "APPEND") || !strings.Contains(journalStr, "Alice") {
		t.Errorf("journal log does not contain expected operations: %q", journalStr)
	}

	// 6. Verify query filtering
	// Filter 1: priority==high
	flt1, _ := ParseFilter("priority==high")
	var matches1 [][]string
	for _, r := range finalRows {
		m, _ := EvaluateFilter(finalHeaders, r, flt1)
		if m {
			matches1 = append(matches1, r)
		}
	}
	if len(matches1) != 1 || matches1[0][1] != "Alice" {
		t.Errorf("filter priority==high failed: got = %v", matches1)
	}

	// Filter 2: name like B
	flt2, _ := ParseFilter("name like B")
	var matches2 [][]string
	for _, r := range finalRows {
		m, _ := EvaluateFilter(finalHeaders, r, flt2)
		if m {
			matches2 = append(matches2, r)
		}
	}
	if len(matches2) != 1 || matches2[0][1] != "Bob" {
		t.Errorf("filter name like B failed: got = %v", matches2)
	}

	// Filter 3: id regex ^[0-9]$
	flt3, _ := ParseFilter("id regex ^[0-9]$")
	var matches3 [][]string
	for _, r := range finalRows {
		m, _ := EvaluateFilter(finalHeaders, r, flt3)
		if m {
			matches3 = append(matches3, r)
		}
	}
	// Matches id=1 and id=2, but not id=10
	if len(matches3) != 2 {
		t.Errorf("filter id regex ^[0-9]$ failed: got = %v", matches3)
	}

	// 7. Verify inconsistent column size returns parsing error
	malformedFile := filepath.Join(tempDir, "malformed.md")
	malformedContent := `| id | name |
|---|---|
| 1 | Alice |
| 2 | Bob | Extra |
`
	if err := os.WriteFile(malformedFile, []byte(malformedContent), 0644); err != nil {
		t.Fatalf("Failed to write malformed table: %v", err)
	}

	_, _, _, _, err = ParseFrontmatterAndContent(malformedFile)
	if err == nil {
		t.Error("expected error due to inconsistent row size, got none")
	}

	// 8. Verify escaped pipe cells parsing/serialization
	escapedFile := filepath.Join(tempDir, "escaped.md")
	escapedContent := `| id | details |
|---|---|
| 1 | details with \| escaped pipe |
`
	if err := os.WriteFile(escapedFile, []byte(escapedContent), 0644); err != nil {
		t.Fatalf("Failed to write escaped table: %v", err)
	}

	_, escapedRows, _, _, err := ParseFrontmatterAndContent(escapedFile)
	if err != nil {
		t.Fatalf("Failed to parse table with escaped pipe: %v", err)
	}
	if len(escapedRows) != 1 || escapedRows[0][1] != "details with | escaped pipe" {
		t.Errorf("escaped pipe cells failed: got = %v", escapedRows)
	}

	// Verify formatted line escapes pipe again
	formattedLine := FormatGFMLine(escapedRows[0])
	expectedFormatted := `| 1 | details with \| escaped pipe |`
	if formattedLine != expectedFormatted {
		t.Errorf("FormatGFMLine with escaped pipe got = %q, want %q", formattedLine, expectedFormatted)
	}

	// 9. Verify direct append optimization (no temp swap)
	// We disable sorting by initializing table without sort
	unsortedPath := filepath.Join(tempDir, "unsorted.md")
	cfgUnsorted := TableConfig{}
	err = InitTable(unsortedPath, []string{"id", "name"}, cfgUnsorted)
	if err != nil {
		t.Fatalf("Failed to init unsorted table: %v", err)
	}

	// Verify we can append directly
	err = AppendRow(unsortedPath, map[string]string{"id": "1", "name": "Alice"})
	if err != nil {
		t.Fatalf("Unsorted append failed: %v", err)
	}

	_, unsortedRows, _, _, err := ParseFrontmatterAndContent(unsortedPath)
	if err != nil {
		t.Fatalf("Failed to parse unsorted table: %v", err)
	}
	if len(unsortedRows) != 1 || unsortedRows[0][0] != "1" {
		t.Errorf("unsorted rows got = %v", unsortedRows)
	}
}
