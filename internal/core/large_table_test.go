package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pipik-roman/rowlite/internal/core"
)

func get10kTablePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("testdata", "large_table_10k.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("static test fixture not found at %s", path)
	}
	return path
}

func TestLargeTable10k_LoadAndStructure(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse 10k table: %v", err)
	}

	if len(rows) != 10000 {
		t.Errorf("expected 10,000 rows, got %d", len(rows))
	}
	if len(cfg.Headers) != 7 {
		t.Errorf("expected 7 headers, got %d: %v", len(cfg.Headers), cfg.Headers)
	}
	if cfg.RowCount != 10000 {
		t.Errorf("expected cfg.RowCount 10,000, got %d", cfg.RowCount)
	}

	// Verify frontmatter parsed properly
	if len(cfg.Unique) != 1 || cfg.Unique[0] != "Task ID" {
		t.Errorf("expected unique [Task ID], got %v", cfg.Unique)
	}
	if cfg.Sort != "Task ID:asc" {
		t.Errorf("expected sort Task ID:asc, got %q", cfg.Sort)
	}
	if cfg.Audit.Insert != "Created At" {
		t.Errorf("expected audit.insert 'Created At', got %q", cfg.Audit.Insert)
	}
	if cfg.AutoIncrement.Column != "Task ID" {
		t.Errorf("expected autoincrement column 'Task ID', got %q", cfg.AutoIncrement.Column)
	}
}

func TestLargeTable10k_Count_FastPath(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	start := time.Now()
	count, err := core.Count(ctx, filePath, nil)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 10000 {
		t.Errorf("expected count 10000, got %d", count)
	}
	// Fast path should complete well under 25ms
	if duration > 250*time.Millisecond {
		t.Errorf("fast-path Count took unexpectedly long: %v", duration)
	}
}

func TestLargeTable10k_SelectRows_ExactMatchAndIndex(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// 1. Select row with Task ID == 5432
	filters, err := core.ParseFilters([]string{"Task ID == 5432"})
	if err != nil {
		t.Fatalf("ParseFilters error: %v", err)
	}

	matches, err := core.SelectRows(cfg.Headers, rows, filters)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for Task ID == 5432, got %d", len(matches))
	}
	if matches[0].Index != 5431 {
		t.Errorf("expected 0-based index 5431, got %d", matches[0].Index)
	}
	if matches[0].Row[0] != "5432" {
		t.Errorf("expected row[0] '5432', got %q", matches[0].Row[0])
	}

	// 2. First row index check
	firstFilter, _ := core.ParseFilters([]string{"Task ID == 1"})
	firstMatch, err := core.SelectRows(cfg.Headers, rows, firstFilter)
	if err != nil || len(firstMatch) != 1 || firstMatch[0].Index != 0 {
		t.Errorf("first row match failed: %v, matches: %+v", err, firstMatch)
	}

	// 3. Last row index check
	lastFilter, _ := core.ParseFilters([]string{"Task ID == 10000"})
	lastMatch, err := core.SelectRows(cfg.Headers, rows, lastFilter)
	if err != nil || len(lastMatch) != 1 || lastMatch[0].Index != 9999 {
		t.Errorf("last row match failed: %v, matches: %+v", err, lastMatch)
	}
}

func TestLargeTable10k_SpecialCharacters_EscapedPipesAndBackslashes(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// 1. Search for escaped pipe substring: "Option A | Option B"
	pipeFilters, err := core.ParseFilters([]string{"Task Description like Option A | Option B"})
	if err != nil {
		t.Fatalf("ParseFilters error: %v", err)
	}
	pipeMatches, err := core.SelectRows(cfg.Headers, rows, pipeFilters)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}
	if len(pipeMatches) == 0 {
		t.Fatal("expected matches for 'Option A | Option B', got 0")
	}
	for _, m := range pipeMatches {
		if !strings.Contains(m.Row[1], "Option A | Option B") {
			t.Errorf("row %d does not contain expected pipe text: %s", m.Index, m.Row[1])
		}
	}

	// 2. Search for escaped backslash substring: "C:\Windows\System32"
	slashFilters, err := core.ParseFilters([]string{"Task Description like C:\\Windows\\System32"})
	if err != nil {
		t.Fatalf("ParseFilters error: %v", err)
	}
	slashMatches, err := core.SelectRows(cfg.Headers, rows, slashFilters)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}
	if len(slashMatches) == 0 {
		t.Fatal("expected matches for 'C:\\Windows\\System32', got 0")
	}
	for _, m := range slashMatches {
		if !strings.Contains(m.Row[1], `C:\Windows\System32`) {
			t.Errorf("row %d does not contain expected backslash path: %s", m.Index, m.Row[1])
		}
	}
}

func TestLargeTable10k_SpecialCharacters_UnicodeAndEmojis(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// 1. Rocket emoji: 🚀
	emojiFilters, _ := core.ParseFilters([]string{"Task Description like 🚀"})
	emojiMatches, err := core.SelectRows(cfg.Headers, rows, emojiFilters)
	if err != nil || len(emojiMatches) == 0 {
		t.Fatalf("failed to match 🚀: err=%v, count=%d", err, len(emojiMatches))
	}

	// 2. Warning emoji: ⚠️
	warnFilters, _ := core.ParseFilters([]string{"Task Description like ⚠️"})
	warnMatches, err := core.SelectRows(cfg.Headers, rows, warnFilters)
	if err != nil || len(warnMatches) == 0 {
		t.Fatalf("failed to match ⚠️: err=%v, count=%d", err, len(warnMatches))
	}

	// 3. Accented Unicode category: "Štúdio & Media"
	accFilters, _ := core.ParseFilters([]string{"Category Name == Štúdio & Media"})
	accMatches, err := core.SelectRows(cfg.Headers, rows, accFilters)
	if err != nil || len(accMatches) == 0 {
		t.Fatalf("failed to match 'Štúdio & Media': err=%v, count=%d", err, len(accMatches))
	}

	// 4. Asian script category: "日本語 Cluster"
	asianFilters, _ := core.ParseFilters([]string{"Category Name == 日本語 Cluster"})
	asianMatches, err := core.SelectRows(cfg.Headers, rows, asianFilters)
	if err != nil || len(asianMatches) == 0 {
		t.Fatalf("failed to match '日本語 Cluster': err=%v, count=%d", err, len(asianMatches))
	}

	// 5. Regex with JSON brackets and special chars: {"retry": true
	regexFilters, _ := core.ParseFilters([]string{`Task Description regex "retry":\s*true`})
	regexMatches, err := core.SelectRows(cfg.Headers, rows, regexFilters)
	if err != nil || len(regexMatches) == 0 {
		t.Fatalf("failed to match regex with JSON payload: err=%v, count=%d", err, len(regexMatches))
	}
}

func TestLargeTable10k_MultiWordHeadersNormalized(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Query same column using 3 variations of header normalization
	variants := [][]string{
		{"task_description like Kubernetes"},
		{"task description like Kubernetes"},
		{"TASK-DESCRIPTION like Kubernetes"},
	}

	var firstCount int
	for i, v := range variants {
		filters, err := core.ParseFilters(v)
		if err != nil {
			t.Fatalf("variant %d ParseFilters error: %v", i, err)
		}
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil {
			t.Fatalf("variant %d SelectRows error: %v", i, err)
		}
		if i == 0 {
			firstCount = len(matches)
			if firstCount == 0 {
				t.Fatal("expected non-zero matches for Kubernetes")
			}
		} else if len(matches) != firstCount {
			t.Errorf("variant %d matched %d rows, expected %d", i, len(matches), firstCount)
		}
	}
}

func TestLargeTable10k_MultiFilter_AND(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	filters, err := core.ParseFilters([]string{
		"Category Name == DevOps & CI/CD",
		"Priority Level == high",
		"Status State == in_progress",
	})
	if err != nil {
		t.Fatalf("ParseFilters error: %v", err)
	}

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	matches, err := core.SelectRows(cfg.Headers, rows, filters)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected multi-filter matches, got 0")
	}

	for _, m := range matches {
		if m.Row[2] != "DevOps & CI/CD" || m.Row[3] != "high" || m.Row[4] != "in_progress" {
			t.Errorf("row %d did not satisfy all AND filters: %v", m.Index, m.Row)
		}
	}
}

func TestLargeTable10k_Sort(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// Sort 10,000 rows by Score Metric:asc
	err = core.SortRows(cfg.Headers, rows, "Score Metric:asc")
	if err != nil {
		t.Fatalf("SortRows error: %v", err)
	}

	// Verify rows are in ascending score order
	scoreIdx, _, _ := core.ResolveColumn(cfg.Headers, "Score Metric")
	for i := 0; i < len(rows)-1; i++ {
		valA, _ := strconv.Atoi(rows[i][scoreIdx])
		valB, _ := strconv.Atoi(rows[i+1][scoreIdx])
		if valA > valB {
			t.Fatalf("rows not sorted properly at index %d: %d > %d", i, valA, valB)
		}
	}
}

func TestLargeTable10k_Append(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	// Create temp copy of 10k table
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "table_10k_append.md")

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("failed to write temp copy: %v", err)
	}

	// Append row with auto-increment ID omitted and special characters
	newRow := map[string]string{
		"Task Description": "New Appended Task with \\| and 🚀",
		"Category Name":    "Backend & Database",
		"Priority Level":   "critical",
		"Status State":     "active",
		"Score Metric":     "999",
	}

	err = core.AppendRow(ctx, tmpFile, newRow)
	if err != nil {
		t.Fatalf("AppendRow failed: %v", err)
	}

	// Verify count is now 10,001
	count, err := core.Count(ctx, tmpFile, nil)
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if count != 10001 {
		t.Errorf("expected count 10,001, got %d", count)
	}

	// Verify last row has ID 10001
	cfgParsed, lastRows, err := core.ParseFrontmatterAndContent(ctx, tmpFile, core.ContentModeLast)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent ContentModeLast error: %v", err)
	}
	if len(lastRows) != 1 {
		t.Fatalf("expected 1 row from ContentModeLast, got %d", len(lastRows))
	}
	idIdx, _, _ := core.ResolveColumn(cfgParsed.Headers, "Task ID")
	if lastRows[0][idIdx] != "10001" {
		t.Errorf("expected auto-increment ID '10001', got %q", lastRows[0][idIdx])
	}
}

func TestLargeTable10k_PositionalQueries(t *testing.T) {
	filePath := get10kTablePath(t)
	ctx := context.Background()

	// 1. Count with Head 50
	headCount := 50
	count, err := core.CountWithOptions(ctx, filePath, nil, core.PositionalOptions{Head: &headCount})
	if err != nil {
		t.Fatalf("CountWithOptions head error: %v", err)
	}
	if count != 50 {
		t.Errorf("expected count 50, got %d", count)
	}

	// 2. Count with Tail 25
	tailCount := 25
	count, err = core.CountWithOptions(ctx, filePath, nil, core.PositionalOptions{Tail: &tailCount})
	if err != nil {
		t.Fatalf("CountWithOptions tail error: %v", err)
	}
	if count != 25 {
		t.Errorf("expected count 25, got %d", count)
	}

	// 3. Positional query with stepped slice on 10k rows: "0..9999..1000" (10 rows)
	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	matched, err := core.SelectRows(cfg.Headers, rows, nil)
	if err != nil {
		t.Fatalf("SelectRows error: %v", err)
	}

	sliced, err := matched.ApplyPositional(core.PositionalOptions{Index: "0..9999..1000"})
	if err != nil {
		t.Fatalf("ApplyPositional error: %v", err)
	}
	if len(sliced) != 10 {
		t.Fatalf("expected 10 stepped rows, got %d", len(sliced))
	}
	if sliced[0].Index != 0 || sliced[9].Index != 9000 {
		t.Errorf("expected indices 0 and 9000 at ends, got %d and %d", sliced[0].Index, sliced[9].Index)
	}
}


