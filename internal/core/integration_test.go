package core_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
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
	cols := []string{"id", "name", "priority"}
	cfg := core.TableConfig{
		Unique:      []string{"id"},
		Sort:        "id:asc",
		Audit: core.AuditConfig{
			Insert: "created_at",
			Edit:   "updated_at",
		},
		JournalLog:  journalPath,
	}

	err = core.InitTable(context.Background(), tablePath, cols, cfg)
	if err != nil {
		t.Fatalf("Failed to initialize table: %v", err)
	}

	// Verify structure is set up
	cfgParsed, rows, err := core.ParseFrontmatterAndContent(context.Background(), tablePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("Failed to parse initialized table: %v", err)
	}

	headers := cfgParsed.Headers
	alignments := cfgParsed.Alignments

	expectedHeaders := []string{"id", "name", "priority", "created_at", "updated_at"}
	if !reflect.DeepEqual(headers, expectedHeaders) {
		t.Errorf("headers mismatch: got = %v, want %v", headers, expectedHeaders)
	}
	if len(alignments) != len(expectedHeaders) {
		t.Errorf("alignments size mismatch: got = %d, want %d", len(alignments), len(expectedHeaders))
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows initially, got %d", len(rows))
	}

	// 2. Append rows in sorted order (since append doesn't auto-sort anymore, unsorted append is an error)
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "1", "name": "Charlie", "priority": "low"})
	if err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "2", "name": "Bob", "priority": "medium"})
	if err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "10", "name": "Alice", "priority": "high"})
	if err != nil {
		t.Fatalf("Append 3 failed: %v", err)
	}

	// Verify that appending an unsorted row fails
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "5", "name": "Unsorted", "priority": "low"})
	if err == nil {
		t.Error("expected error when appending unsorted row, got none")
	}

	// 3. Verify unique keys check (duplicate id should fail)
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "2", "name": "Duplicate Bob", "priority": "medium"})
	if err == nil {
		t.Error("expected duplicate key error for id=2, but got none")
	}

	// 4. Verify sorting (id:asc) and audit timestamp creation
	cfgParsedFinal, finalRows, err := core.ParseFrontmatterAndContent(context.Background(), tablePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("Failed to parse final table: %v", err)
	}

	finalHeaders := cfgParsedFinal.Headers

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
	flt1, _ := core.ParseFilter("priority==high")
	var matches1 [][]string
	for _, r := range finalRows {
		m, _ := core.EvaluateFilter(finalHeaders, r, flt1)
		if m {
			matches1 = append(matches1, r)
		}
	}
	if len(matches1) != 1 || matches1[0][1] != "Alice" {
		t.Errorf("filter priority==high failed: got = %v", matches1)
	}

	// Filter 2: name like B
	flt2, _ := core.ParseFilter("name like B")
	var matches2 [][]string
	for _, r := range finalRows {
		m, _ := core.EvaluateFilter(finalHeaders, r, flt2)
		if m {
			matches2 = append(matches2, r)
		}
	}
	if len(matches2) != 1 || matches2[0][1] != "Bob" {
		t.Errorf("filter name like B failed: got = %v", matches2)
	}

	// Filter 3: id regex ^[0-9]$
	flt3, _ := core.ParseFilter("id regex ^[0-9]$")
	var matches3 [][]string
	for _, r := range finalRows {
		m, _ := core.EvaluateFilter(finalHeaders, r, flt3)
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

	_, _, err = core.ParseFrontmatterAndContent(context.Background(), malformedFile, core.ContentModeAll)
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

	_, escapedRows, err := core.ParseFrontmatterAndContent(context.Background(), escapedFile, core.ContentModeAll)
	if err != nil {
		t.Fatalf("Failed to parse table with escaped pipe: %v", err)
	}
	if len(escapedRows) != 1 || escapedRows[0][1] != "details with | escaped pipe" {
		t.Errorf("escaped pipe cells failed: got = %v", escapedRows)
	}

	// Verify formatted line escapes pipe again
	formattedLine := core.FormatGFMLine(escapedRows[0])
	expectedFormatted := `| 1 | details with \| escaped pipe |`
	if formattedLine != expectedFormatted {
		t.Errorf("FormatGFMLine with escaped pipe got = %q, want %q", formattedLine, expectedFormatted)
	}

	// 9. Verify direct append optimization (no temp swap)
	// We disable sorting by initializing table without sort
	unsortedPath := filepath.Join(tempDir, "unsorted.md")
	cfgUnsorted := core.TableConfig{}
	err = core.InitTable(context.Background(), unsortedPath, []string{"id", "name"}, cfgUnsorted)
	if err != nil {
		t.Fatalf("Failed to init unsorted table: %v", err)
	}

	// Verify we can append directly
	err = core.AppendRow(context.Background(), unsortedPath, map[string]string{"id": "1", "name": "Alice"})
	if err != nil {
		t.Fatalf("Unsorted append failed: %v", err)
	}

	cfgParsedUnsorted, unsortedRows, err := core.ParseFrontmatterAndContent(context.Background(), unsortedPath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("Failed to parse unsorted table: %v", err)
	}
	if len(unsortedRows) != 1 || unsortedRows[0][0] != "1" {
		t.Errorf("unsorted rows got = %v", unsortedRows)
	}
	_ = cfgParsedUnsorted
}

func TestIntegration_PreserveContent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_preserve_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "preserved.md")
	content := `---
title: Keep Me
rowlite:
  unique: [id]
  sort: id:asc
---
# Header
Some intro text.

| id | name |
|---|---|
| 1 | Alice |

Footer text.
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Read config and rows
	cfg, rows, err := core.ParseFrontmatterAndContent(context.Background(), filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}

	// Update rows
	rows = append(rows, []string{"2", "Bob"})

	// Write atomically (e.g., simulating sort or edit)
	err = core.AtomicWriteTable(context.Background(), filePath, cfg, rows)
	if err != nil {
		t.Fatalf("AtomicWriteTable failed: %v", err)
	}

	// Read file contents back and check if it preserved non-table content
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	wantContent := `---
title: Keep Me
rowlite:
  unique: [id]
  sort: id:asc
---
# Header
Some intro text.

| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Bob |

Footer text.
`
	// Standardize line endings just in case
	gotStr := strings.ReplaceAll(string(bytes), "\r\n", "\n")
	wantStr := strings.ReplaceAll(wantContent, "\r\n", "\n")

	if gotStr != wantStr {
		t.Errorf("content preservation failed:\ngot:\n%s\nwant:\n%s", gotStr, wantStr)
	}
}

func TestIntegration_CountFacade(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_count_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "tasks.md")
	content := `| id | name | status |
|---|---|---|
| 1 | Task One | todo |
| 2 | Task Two | doing |
| 3 | Task Three | todo |
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// 1. Count without filters (total count - should run with ContentModeNone internally)
	totalCount, err := core.Count(context.Background(), filePath, nil)
	if err != nil {
		t.Fatalf("Count total failed: %v", err)
	}
	if totalCount != 3 {
		t.Errorf("expected total count 3, got = %d", totalCount)
	}

	// 2. Count with filters (filtered count - should run with ContentModeAll internally)
	todoCount, err := core.Count(context.Background(), filePath, []string{"status==todo"})
	if err != nil {
		t.Fatalf("Count filtered failed: %v", err)
	}
	if todoCount != 2 {
		t.Errorf("expected filtered count 2, got = %d", todoCount)
	}
}

func TestJournal4ColumnsAndRowKey(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_journal_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tablePath := filepath.Join(tempDir, "table.md")
	journalPath := filepath.Join(tempDir, "journal.md")

	cfg := core.TableConfig{
		Unique:     []string{"id"},
		JournalLog: journalPath,
	}

	err = core.InitTable(context.Background(), tablePath, []string{"id", "title"}, cfg)
	if err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "100", "title": "First"})
	if err != nil {
		t.Fatalf("AppendRow failed: %v", err)
	}

	journalBytes, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("ReadFile journal failed: %v", err)
	}

	journalStr := string(journalBytes)
	if !strings.Contains(journalStr, "| timestamp | operation | row_key | details |") {
		t.Errorf("journal header missing 4-column layout: %s", journalStr)
	}
	if !strings.Contains(journalStr, "| 100 |") {
		t.Errorf("journal record missing expected row_key '100': %s", journalStr)
	}
}

func TestAutoIncrementColumn(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_autoinc_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tablePath := filepath.Join(tempDir, "tasks.md")
	cfg := core.TableConfig{
		AutoIncrement: core.AutoIncrementConfig{
			Column: "id",
			Step:   10,
		},
	}

	err = core.InitTable(context.Background(), tablePath, []string{"id", "name"}, cfg)
	if err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	// 1st append: omit id -> auto-increments to 10
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"name": "Task 1"})
	if err != nil {
		t.Fatalf("AppendRow 1 failed: %v", err)
	}

	// 2nd append: omit id -> auto-increments to 20
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"name": "Task 2"})
	if err != nil {
		t.Fatalf("AppendRow 2 failed: %v", err)
	}

	// 3rd append: explicit id = 50 -> preserves 50
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"id": "50", "name": "Task 3"})
	if err != nil {
		t.Fatalf("AppendRow 3 failed: %v", err)
	}

	// 4th append: omit id -> auto-increments to 60 (50 + 10)
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"name": "Task 4"})
	if err != nil {
		t.Fatalf("AppendRow 4 failed: %v", err)
	}

	cfgParsed, rows, err := core.ParseFrontmatterAndContent(context.Background(), tablePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}

	if cfgParsed.AutoIncrement.Column != "id" || cfgParsed.AutoIncrement.Step != 10 {
		t.Errorf("AutoIncrement config mismatch: got %+v", cfgParsed.AutoIncrement)
	}

	expectedIds := []string{"10", "20", "50", "60"}
	if len(rows) != len(expectedIds) {
		t.Fatalf("expected %d rows, got %d", len(expectedIds), len(rows))
	}
	for i, r := range rows {
		if r[0] != expectedIds[i] {
			t.Errorf("row %d id mismatch: got %s, want %s", i, r[0], expectedIds[i])
		}
	}
}

func TestJournal3To4ColumnMigration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_journal_mig_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tablePath := filepath.Join(tempDir, "table.md")
	journalPath := filepath.Join(tempDir, "journal.md")

	// Create legacy 3-column journal file
	legacyContent := `| timestamp | operation | details |
| --- | --- | --- |
| 2026-08-08 06:17:21 | APPEND | {"tag":"COAR","shortcut":"cr","next_seq":" 0003"} |
| 2026-08-08 06:17:21 | APPEND | {"tag":"DOMAIL","shortcut":"dm","next_seq":" 0038"} |
`
	if err := os.WriteFile(journalPath, []byte(legacyContent), 0644); err != nil {
		t.Fatalf("WriteFile journal failed: %v", err)
	}

	cfg := core.TableConfig{
		Unique:     []string{"tag"},
		JournalLog: journalPath,
	}
	if err := core.InitTable(context.Background(), tablePath, []string{"tag", "shortcut", "next_seq"}, cfg); err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	// Trigger new append
	if err := core.AppendRow(context.Background(), tablePath, map[string]string{"tag": "ROWLITE", "shortcut": "rl", "next_seq": "0004"}); err != nil {
		t.Fatalf("AppendRow failed: %v", err)
	}

	migratedBytes, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	migratedStr := string(migratedBytes)

	if !strings.Contains(migratedStr, "| timestamp | operation | row_key | details |") {
		t.Errorf("expected 4-column header, got:\n%s", migratedStr)
	}
	if !strings.Contains(migratedStr, "| 2026-08-08 06:17:21 | APPEND | COAR |") {
		t.Errorf("expected legacy entry to be migrated with row_key 'COAR', got:\n%s", migratedStr)
	}
	if !strings.Contains(migratedStr, "| APPEND | ROWLITE |") {
		t.Errorf("expected new entry with row_key 'ROWLITE', got:\n%s", migratedStr)
	}
}

func TestAutoIncrementWithSortLast(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_autoinc_sort_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tablePath := filepath.Join(tempDir, "tasks.md")
	cfg := core.TableConfig{
		Unique: []string{"id"},
		Sort:   "id:asc",
		AutoIncrement: core.AutoIncrementConfig{
			Column: "id",
			Step:   10,
		},
	}

	err = core.InitTable(context.Background(), tablePath, []string{"id", "title"}, cfg)
	if err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	err = core.AppendRow(context.Background(), tablePath, map[string]string{"title": "First"})
	if err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"title": "Second"})
	if err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}

	cfgParsed, rows, err := core.ParseFrontmatterAndContent(context.Background(), tablePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(rows) != 2 || rows[0][0] != "10" || rows[1][0] != "20" {
		t.Errorf("unexpected rows content: %v", rows)
	}
	if cfgParsed.MaxAutoIncVal != 20 {
		t.Errorf("expected MaxAutoIncVal = 20, got = %d", cfgParsed.MaxAutoIncVal)
	}
}

func TestAutoIncrementEmptyTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rowlite_autoinc_empty_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tablePath := filepath.Join(tempDir, "tasks.md")
	cfg := core.TableConfig{
		AutoIncrement: core.AutoIncrementConfig{
			Column: "id",
			Step:   10,
		},
	}

	err = core.InitTable(context.Background(), tablePath, []string{"id", "title"}, cfg)
	if err != nil {
		t.Fatalf("InitTable failed: %v", err)
	}

	// 1. Verify ParseFrontmatterAndContent on empty table
	cfgParsed, rows, err := core.ParseFrontmatterAndContent(context.Background(), tablePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}

	if cfgParsed.RowCount != 0 {
		t.Errorf("expected RowCount = 0, got = %d", cfgParsed.RowCount)
	}
	if cfgParsed.MaxAutoIncVal != 0 {
		t.Errorf("expected MaxAutoIncVal = 0 for empty table, got = %d", cfgParsed.MaxAutoIncVal)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got = %d", len(rows))
	}

	// 2. Append first row -> should get id = 0 + 10 = 10
	err = core.AppendRow(context.Background(), tablePath, map[string]string{"title": "First Item"})
	if err != nil {
		t.Fatalf("AppendRow failed: %v", err)
	}

	cfgAfter, rowsAfter, err := core.ParseFrontmatterAndContent(context.Background(), tablePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent after append failed: %v", err)
	}

	if cfgAfter.RowCount != 1 {
		t.Errorf("expected RowCount = 1, got = %d", cfgAfter.RowCount)
	}
	if cfgAfter.MaxAutoIncVal != 10 {
		t.Errorf("expected MaxAutoIncVal = 10 after 1st append, got = %d", cfgAfter.MaxAutoIncVal)
	}
	if len(rowsAfter) != 1 || rowsAfter[0][0] != "10" {
		t.Errorf("expected first row id to be 10, got = %v", rowsAfter)
	}
}
