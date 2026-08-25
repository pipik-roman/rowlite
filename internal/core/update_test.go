package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestTable(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "table.md")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	return filePath
}

func TestUpdateBasicWhere(t *testing.T) {
	content := `| id | name | status |
| --- | --- | --- |
| 1 | Alice | Pending |
| 2 | Bob | Pending |
| 3 | Charlie | Completed |
`
	filePath := createTestTable(t, content)

	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"status == Pending"},
		PositionalOptions{},
		map[string]string{"status": "Active"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 updated rows, got %d", count)
	}

	cfg, rows, err := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse updated table: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0][2] != "Active" || rows[1][2] != "Active" || rows[2][2] != "Completed" {
		t.Errorf("unexpected row statuses: %v, %v, %v", rows[0][2], rows[1][2], rows[2][2])
	}
	_ = cfg
}

func TestUpdateMultiColumnWithSnapshotIsolation(t *testing.T) {
	content := `| id | score | prev_score |
| --- | --- | --- |
| 1 | 100 | 0 |
| 2 | 200 | 0 |
`
	filePath := createTestTable(t, content)

	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		map[string]string{"score": "150", "prev_score": "100"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 updated row, got %d", count)
	}

	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[0][1] != "150" || rows[0][2] != "100" {
		t.Errorf("expected score=150, prev_score=100, got score=%s, prev_score=%s", rows[0][1], rows[0][2])
	}
}

func TestUpdatePositionalSlicing(t *testing.T) {
	content := `| id | name | status |
| --- | --- | --- |
| 1 | Alice | Pending |
| 2 | Bob | Pending |
| 3 | Charlie | Pending |
| 4 | Dave | Pending |
| 5 | Eve | Pending |
`
	filePath := createTestTable(t, content)

	// Update first 2 matching rows (--head 2)
	h := 2
	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"status == Pending"},
		PositionalOptions{Head: &h},
		map[string]string{"status": "Processed"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 updated rows, got %d", count)
	}

	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[0][2] != "Processed" || rows[1][2] != "Processed" || rows[2][2] != "Pending" {
		t.Errorf("unexpected row statuses after head update: %v", rows)
	}

	// Update last row (--index -1)
	count, err = UpdateWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{Index: "-1"},
		map[string]string{"status": "LastRow"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 updated row, got %d", count)
	}

	_, rows, _ = ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[4][2] != "LastRow" {
		t.Errorf("expected last row status=LastRow, got %s", rows[4][2])
	}
}

func TestUpdateEditAuditAutoTimestamp(t *testing.T) {
	content := `---
rowlite:
  audit:
    edit: "modified_at"
---
| id | name | modified_at |
| --- | --- | --- |
| 1 | Task A | 2026-01-01 00:00:00 |
| 2 | Task B | 2026-01-01 00:00:00 |
`
	filePath := createTestTable(t, content)

	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		map[string]string{"name": "Task A (Done)"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 updated row, got %d", count)
	}

	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[0][2] == "2026-01-01 00:00:00" || rows[0][2] == "" {
		t.Errorf("expected modified_at to be automatically updated, got %q", rows[0][2])
	}
	if rows[1][2] != "2026-01-01 00:00:00" {
		t.Errorf("expected row 2 modified_at to remain unchanged, got %q", rows[1][2])
	}
}

func TestUpdateUniqueConstraintCollision(t *testing.T) {
	content := `---
rowlite:
  unique: [email]
---
| id | email |
| --- | --- |
| 1 | alice@example.com |
| 2 | bob@example.com |
| 3 | charlie@example.com |
`
	filePath := createTestTable(t, content)

	// Attempt to update Bob's email to Alice's email (collision with existing row)
	_, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 2"},
		PositionalOptions{},
		map[string]string{"email": "alice@example.com"},
		"",
		false,
		false,
	)
	if err == nil {
		t.Fatalf("expected duplicate key error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate key error") {
		t.Errorf("expected error message to contain 'duplicate key error', got %v", err)
	}

	// Verify table content remains untouched
	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[1][1] != "bob@example.com" {
		t.Errorf("expected Bob's email to remain untouched, got %s", rows[1][1])
	}
}

func TestUpdateUniqueEmptyKeyRejection(t *testing.T) {
	content := `---
rowlite:
  unique: [code]
---
| id | code |
| --- | --- |
| 1 | ABC |
`
	filePath := createTestTable(t, content)

	_, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		map[string]string{"code": "   "},
		"",
		false,
		false,
	)
	if err == nil {
		t.Fatalf("expected empty unique key error, got nil")
	}
}

func TestUpdateSortColumnReSort(t *testing.T) {
	content := `---
rowlite:
  sort: "priority:asc"
---
| id | name | priority |
| --- | --- | --- |
| 1 | Low | 10 |
| 2 | Med | 20 |
| 3 | High | 30 |
`
	filePath := createTestTable(t, content)

	// Update High to priority 5 -> should shift to row 0
	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 3"},
		PositionalOptions{},
		map[string]string{"priority": "5"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 updated row, got %d", count)
	}

	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[0][0] != "3" || rows[0][2] != "5" {
		t.Errorf("expected row with id 3 to be first after re-sort, got %v", rows[0])
	}
	if rows[1][0] != "1" || rows[2][0] != "2" {
		t.Errorf("unexpected row order after sort update: %v", rows)
	}
}

func TestUpdateJournalLogging(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "table.md")
	journalPath := filepath.Join(dir, "journal.log.md")

	content := `---
rowlite:
  unique: [id]
  journal_log: "` + strings.ReplaceAll(journalPath, `\`, `/`) + `"
---
| id | status |
| --- | --- |
| A1 | Pending |
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == A1"},
		PositionalOptions{},
		map[string]string{"status": "Done"},
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 updated row, got %d", count)
	}

	journalBytes, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("failed to read journal: %v", err)
	}
	journalStr := string(journalBytes)
	if !strings.Contains(journalStr, "UPDATE") || !strings.Contains(journalStr, "A1") || !strings.Contains(journalStr, "Done") {
		t.Errorf("expected journal to contain UPDATE record for A1 with status Done, got:\n%s", journalStr)
	}
}

func TestUpdateOptimisticConcurrencyControl(t *testing.T) {
	content := `| id | status |
| --- | --- |
| 1 | Open |
`
	filePath := createTestTable(t, content)

	etag, err := ComputeFileETag(filePath)
	if err != nil {
		t.Fatalf("failed to compute ETag: %v", err)
	}

	// 1. Success with matching ETag
	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		map[string]string{"status": "In Progress"},
		etag,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error on matching ETag: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 updated row, got %d", count)
	}

	// 2. Failure with stale/mismatched ETag
	_, err = UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"id == 1"},
		PositionalOptions{},
		map[string]string{"status": "Closed"},
		etag, // Old stale ETag
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

func TestUpdateConditionlessRequiresAll(t *testing.T) {
	content := `| id | status |
| --- | --- |
| 1 | Open |
| 2 | Open |
`
	filePath := createTestTable(t, content)

	// 1. Attempt conditionless update without allowAll -> should fail
	_, err := UpdateWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{},
		map[string]string{"status": "Closed"},
		"",
		false,
		false,
	)
	if err == nil {
		t.Fatalf("expected error for conditionless update without allowAll, got nil")
	}
	if !strings.Contains(err.Error(), "--all") {
		t.Errorf("expected error message to mention '--all', got: %v", err)
	}

	// Verify table is untouched
	_, rows, _ := ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[0][1] != "Open" || rows[1][1] != "Open" {
		t.Errorf("table rows were modified despite error: %v", rows)
	}

	// 2. Conditionless update with allowAll=true -> should succeed
	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		nil,
		PositionalOptions{},
		map[string]string{"status": "Closed"},
		"",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error with allowAll=true: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows updated, got %d", count)
	}

	_, rows, _ = ParseFrontmatterAndContent(context.Background(), filePath, ContentModeAll)
	if rows[0][1] != "Closed" || rows[1][1] != "Closed" {
		t.Errorf("expected all rows to be updated to Closed: %v", rows)
	}
}

func TestUpdateDryRun(t *testing.T) {
	content := `| id | status |
| --- | --- |
| 1 | Open |
| 2 | Open |
| 3 | In Progress |
`
	filePath := createTestTable(t, content)

	count, err := UpdateWithOptions(
		context.Background(),
		filePath,
		[]string{"status == Open"},
		PositionalOptions{},
		map[string]string{"status": "Closed"},
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
	if rows[0][1] != "Open" || rows[1][1] != "Open" {
		t.Errorf("table was modified during dry run: %v", rows)
	}
}

func TestParseSetExpressions(t *testing.T) {
	res, err := ParseSetExpressions([]string{"status=active", "priority=high, count=10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["status"] != "active" || res["priority"] != "high" || res["count"] != "10" {
		t.Errorf("unexpected parsed assignments: %v", res)
	}

	_, err = ParseSetExpressions([]string{"invalid_no_equals"})
	if err == nil {
		t.Errorf("expected error for invalid expression without equals")
	}
}
