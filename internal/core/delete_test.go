package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteBasicWhere(t *testing.T) {
	content := `| id | name | status |
| --- | --- | --- |
| 1 | Alice | Pending |
| 2 | Bob | Active |
| 3 | Charlie | Pending |
| 4 | Dave | Completed |
`
	filePath := createTestTable(t, content)

	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"status == Pending"},
		PositionalOptions{},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deleted rows, got %d", count)
	}

	_, rows, err := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse updated table: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 remaining rows, got %d", len(rows))
	}
	if rows[0][1] != "Bob" || rows[1][1] != "Dave" {
		t.Errorf("unexpected remaining rows: %v", rows)
	}
}

func TestDeletePositional_Head_Tail_Index(t *testing.T) {
	content := `| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Bob |
| 3 | Charlie |
| 4 | Dave |
| 5 | Eve |
`
	filePath := createTestTable(t, content)

	// 1. Delete Head 2 -> deletes Alice and Bob
	h := 2
	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{Head: &h},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("head delete failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deleted rows, got %d", count)
	}

	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 3 || rows[0][1] != "Charlie" || rows[2][1] != "Eve" {
		t.Fatalf("unexpected rows after head delete: %v", rows)
	}

	// 2. Delete Tail 1 -> deletes Eve
	tail := 1
	count, err = DeleteWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{Tail: &tail},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("tail delete failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted row, got %d", count)
	}

	_, rows, _ = ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 2 || rows[0][1] != "Charlie" || rows[1][1] != "Dave" {
		t.Fatalf("unexpected rows after tail delete: %v", rows)
	}

	// 3. Delete single index "-1" (last row) -> deletes Dave
	count, err = DeleteWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{Index: "-1"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("index -1 delete failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted row, got %d", count)
	}

	_, rows, _ = ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 1 || rows[0][1] != "Charlie" {
		t.Fatalf("unexpected rows after index -1 delete: %v", rows)
	}
}

func TestDeleteWhereCombinedWithPositional(t *testing.T) {
	content := `| id | task | status |
| --- | --- | --- |
| 1 | Task 1 | Complete |
| 2 | Task 2 | Complete |
| 3 | Task 3 | Pending |
| 4 | Task 4 | Pending |
| 5 | Task 5 | Pending |
`
	filePath := createTestTable(t, content)

	// Delete first 2 matching 'Pending' tasks (Task 3 and Task 4)
	h := 2
	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"status == Pending"},
		PositionalOptions{Head: &h},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deleted rows, got %d", count)
	}

	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 3 {
		t.Fatalf("expected 3 remaining rows, got %d", len(rows))
	}
	if rows[0][1] != "Task 1" || rows[1][1] != "Task 2" || rows[2][1] != "Task 5" {
		t.Errorf("unexpected remaining rows: %v", rows)
	}
}

func TestDeleteJournalLogging(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "table.md")
	journalPath := filepath.Join(dir, "journal.log.md")

	content := `---
rowlite:
  unique: [id]
  journal_log: "` + strings.ReplaceAll(journalPath, `\`, `/`) + `"
---
| id | name |
| --- | --- |
| A1 | Item A |
| B2 | Item B |
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"id == A1"},
		PositionalOptions{},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted row, got %d", count)
	}

	journalBytes, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("failed to read journal: %v", err)
	}
	journalStr := string(journalBytes)
	if !strings.Contains(journalStr, "DELETE") || !strings.Contains(journalStr, "A1") || !strings.Contains(journalStr, "Item A") {
		t.Errorf("expected journal to contain DELETE record for A1, got:\n%s", journalStr)
	}
}

func TestDeleteOptimisticConcurrencyControl(t *testing.T) {
	content := `| id | name |
| --- | --- |
| 1 | Test |
`
	filePath := createTestTable(t, content)

	etag, err := ComputeFileETag(filePath)
	if err != nil {
		t.Fatalf("failed to compute ETag: %v", err)
	}

	// 1. Success with matching ETag
	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		etag,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error on matching ETag: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted row, got %d", count)
	}

	// Recreate table
	_ = os.WriteFile(filePath, []byte(content), 0644)

	// 2. Failure with stale/mismatched ETag
	_, err = DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		"stale_etag_value",
		false,
		false,
	)
	if err == nil {
		t.Fatalf("expected precondition error on stale ETag, got nil")
	}
	var precondErr *PreconditionFailedError
	if !errors.As(err, &precondErr) {
		t.Errorf("expected PreconditionFailedError, got %T: %v", err, err)
	}
}

func TestDeleteConditionlessRequiresAll(t *testing.T) {
	content := `| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Bob |
`
	filePath := createTestTable(t, content)

	// 1. Attempt conditionless delete without allowAll -> fails
	_, err := DeleteWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{},
		"",
		false,
		false,
	)
	if err == nil {
		t.Fatalf("expected error for conditionless delete without allowAll, got nil")
	}
	if !strings.Contains(err.Error(), "--all") {
		t.Errorf("expected error message to mention '--all', got: %v", err)
	}

	// Verify table is untouched
	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 2 {
		t.Errorf("table rows were deleted despite error: %v", rows)
	}

	// 2. Conditionless delete with allowAll=true -> succeeds
	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{},
		"",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error with allowAll=true: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deleted rows, got %d", count)
	}

	_, rows, _ = ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 0 {
		t.Errorf("expected table to have 0 data rows, got %d", len(rows))
	}
}

func TestDeleteDryRun(t *testing.T) {
	content := `| id | status |
| --- | --- |
| 1 | Open |
| 2 | Open |
| 3 | Closed |
`
	filePath := createTestTable(t, content)

	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"status == Open"},
		PositionalOptions{},
		"",
		false,
		true, // dryRun = true
	)
	if err != nil {
		t.Fatalf("unexpected error during dry run: %v", err)
	}
	if count != 2 {
		t.Errorf("expected dry run count 2, got %d", count)
	}

	// Verify table on disk was NOT modified
	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if len(rows) != 3 {
		t.Errorf("table was modified during dry run: %v", rows)
	}
}

func TestDeletePreservesSurroundingContent(t *testing.T) {
	content := `---
title: My Document
category: notes
---
# Document Header

Some introduction text before the table.

| id | item |
| --- | --- |
| 1 | First |
| 2 | Second |
| 3 | Third |

## Notes Section

This note comes after the table and must remain intact.
`
	filePath := createTestTable(t, content)

	count, err := DeleteWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 2"},
		PositionalOptions{},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted row, got %d", count)
	}

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	fileStr := string(fileBytes)

	if !strings.Contains(fileStr, "title: My Document") || !strings.Contains(fileStr, "# Document Header") {
		t.Errorf("frontmatter or preceding header was lost:\n%s", fileStr)
	}
	if !strings.Contains(fileStr, "## Notes Section") || !strings.Contains(fileStr, "This note comes after the table") {
		t.Errorf("post-table content was lost:\n%s", fileStr)
	}
	if strings.Contains(fileStr, "| 2 | Second |") {
		t.Errorf("deleted row still present in file:\n%s", fileStr)
	}
	if !strings.Contains(fileStr, "| 1 | First |") || !strings.Contains(fileStr, "| 3 | Third |") {
		t.Errorf("expected surviving rows not found:\n%s", fileStr)
	}
}
