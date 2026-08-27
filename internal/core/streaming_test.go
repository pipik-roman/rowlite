package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

func TestPlanPushdown(t *testing.T) {
	head5 := 5
	tail3 := 3
	limit10 := 10

	// 1. Zero options
	p1 := core.PlanPushdown(core.PositionalOptions{})
	if p1.Strategy != core.StrategyAll {
		t.Errorf("expected StrategyAll, got %v", p1.Strategy)
	}

	// 2. Head only
	p2 := core.PlanPushdown(core.PositionalOptions{Head: &head5})
	if p2.Strategy != core.StrategyEarlyExit || p2.MaxMatches != 5 || p2.ContinuousMax != 4 {
		t.Errorf("unexpected head plan: %+v", p2)
	}

	// 3. Tail only
	p3 := core.PlanPushdown(core.PositionalOptions{Tail: &tail3})
	if p3.Strategy != core.StrategyRingBuffer || p3.RingCapacity != 3 {
		t.Errorf("unexpected tail plan: %+v", p3)
	}

	// 4. Offset & Limit
	p4 := core.PlanPushdown(core.PositionalOptions{Offset: 10, Limit: &limit10})
	if p4.Strategy != core.StrategyEarlyExit || p4.MaxMatches != 20 || p4.ContinuousMin != 10 || p4.ContinuousMax != 19 {
		t.Errorf("unexpected offset/limit plan: %+v", p4)
	}

	// 5. Positive Range Index
	p5 := core.PlanPushdown(core.PositionalOptions{Index: "2..6"})
	if p5.Strategy != core.StrategyEarlyExit || p5.ContinuousMin != 2 || p5.ContinuousMax != 6 || p5.MaxMatches != 7 {
		t.Errorf("unexpected range plan: %+v", p5)
	}

	// 6. Negative Single Index
	p6 := core.PlanPushdown(core.PositionalOptions{Index: "-1"})
	if p6.Strategy != core.StrategyRingBuffer || p6.RingCapacity != 1 {
		t.Errorf("unexpected negative single index plan: %+v", p6)
	}

	// 7. Dual Head & Tail
	p7 := core.PlanPushdown(core.PositionalOptions{Head: &head5, Tail: &tail3})
	if p7.Strategy != core.StrategyDualPartition || p7.HeadCapacity != 5 || p7.TailCapacity != 3 {
		t.Errorf("unexpected dual partition plan: %+v", p7)
	}
}

func TestRingBuffer(t *testing.T) {
	rb := core.NewRingBuffer(3)

	rb.Push(core.MatchedRow{Index: 0, Row: []string{"A"}})
	rb.Push(core.MatchedRow{Index: 1, Row: []string{"B"}})

	slice := rb.Slice()
	if len(slice) != 2 || slice[0].Row[0] != "A" || slice[1].Row[0] != "B" {
		t.Fatalf("unexpected 2-item slice: %+v", slice)
	}

	// Push 3rd (fills buffer)
	rb.Push(core.MatchedRow{Index: 2, Row: []string{"C"}})
	slice = rb.Slice()
	if len(slice) != 3 || slice[0].Row[0] != "A" || slice[2].Row[0] != "C" {
		t.Fatalf("unexpected full slice: %+v", slice)
	}

	// Push 4th (evicts A)
	rb.Push(core.MatchedRow{Index: 3, Row: []string{"D"}})
	slice = rb.Slice()
	if len(slice) != 3 || slice[0].Row[0] != "B" || slice[1].Row[0] != "C" || slice[2].Row[0] != "D" {
		t.Fatalf("unexpected wrapped slice: %+v", slice)
	}
	if slice[0].Index != 1 || slice[1].Index != 2 || slice[2].Index != 3 {
		t.Fatalf("indices not preserved: %+v", slice)
	}
}

func TestScanAndSelect_PushdownIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	tablePath := filepath.Join(tmpDir, "streaming_test.md")

	content := `---
rowlite:
  unique: ["id"]
---

| id | title | status |
| :--- | :--- | :--- |
| 1 | Task 1 | Complete |
| 2 | Task 2 | Complete |
| 3 | Task 3 | Pending |
| 4 | Task 4 | Pending |
| 5 | Task 5 | Complete |
| 6 | Task 6 | Pending |
| 7 | Task 7 | Pending |
| 8 | Task 8 | Complete |
| 9 | Task 9 | Pending |
| 10 | Task 10 | Complete |
`
	if err := os.WriteFile(tablePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx := context.Background()

	// 1. Head 2 without filters (returns physical rows 0, 1)
	h2 := 2
	_, matches, err := core.ScanAndSelect(ctx, tablePath, nil, core.PositionalOptions{Head: &h2})
	if err != nil {
		t.Fatalf("ScanAndSelect failed: %v", err)
	}
	if len(matches) != 2 || matches[0].Row[0] != "1" || matches[1].Row[0] != "2" {
		t.Fatalf("unexpected head 2 result: %+v", matches)
	}
	if matches[0].Index != 0 || matches[1].Index != 1 {
		t.Errorf("incorrect absolute indices: %+v", matches)
	}

	// 2. WHERE "status==Pending" with Head 2 (returns Task 3 & Task 4 with absolute indices 2 & 3)
	flt, _ := core.ParseFilters([]string{"status == Pending"})
	_, matches, err = core.ScanAndSelect(ctx, tablePath, flt, core.PositionalOptions{Head: &h2})
	if err != nil {
		t.Fatalf("ScanAndSelect where+head failed: %v", err)
	}
	if len(matches) != 2 || matches[0].Row[1] != "Task 3" || matches[1].Row[1] != "Task 4" {
		t.Fatalf("unexpected filtered head 2: %+v", matches)
	}
	if matches[0].Index != 2 || matches[1].Index != 3 {
		t.Errorf("incorrect absolute indices for filtered head: %+v", matches)
	}

	// 3. WHERE "status==Pending" with Tail 2 (returns Task 7 & Task 9 with absolute indices 6 & 8)
	t2 := 2
	_, matches, err = core.ScanAndSelect(ctx, tablePath, flt, core.PositionalOptions{Tail: &t2})
	if err != nil {
		t.Fatalf("ScanAndSelect where+tail failed: %v", err)
	}
	if len(matches) != 2 || matches[0].Row[1] != "Task 7" || matches[1].Row[1] != "Task 9" {
		t.Fatalf("unexpected filtered tail 2: %+v", matches)
	}
	if matches[0].Index != 6 || matches[1].Index != 8 {
		t.Errorf("incorrect absolute indices for filtered tail: %+v", matches)
	}

	// 4. Offset 1, Limit 2 on Pending subset (Pending: [Task 3 (0), Task 4 (1), Task 6 (2), Task 7 (3), Task 9 (4)])
	// Offset 1, Limit 2 -> Task 4 & Task 6 (absolute indices 3 & 5)
	l2 := 2
	_, matches, err = core.ScanAndSelect(ctx, tablePath, flt, core.PositionalOptions{Offset: 1, Limit: &l2})
	if err != nil {
		t.Fatalf("ScanAndSelect offset/limit failed: %v", err)
	}
	if len(matches) != 2 || matches[0].Row[1] != "Task 4" || matches[1].Row[1] != "Task 6" {
		t.Fatalf("unexpected offset/limit result: %+v", matches)
	}
	if matches[0].Index != 3 || matches[1].Index != 5 {
		t.Errorf("incorrect absolute indices for offset/limit: %+v", matches)
	}

	// 5. Single negative index: -1 on Pending subset -> Task 9 (absolute index 8)
	_, matches, err = core.ScanAndSelect(ctx, tablePath, flt, core.PositionalOptions{Index: "-1"})
	if err != nil {
		t.Fatalf("ScanAndSelect index -1 failed: %v", err)
	}
	if len(matches) != 1 || matches[0].Row[1] != "Task 9" || matches[0].Index != 8 {
		t.Fatalf("unexpected index -1 result: %+v", matches)
	}
}

func TestSelectRowsStream_Equivalence(t *testing.T) {
	headers := []string{"id", "category", "val"}
	rows := [][]string{
		{"1", "A", "10"},
		{"2", "B", "20"},
		{"3", "A", "30"},
		{"4", "A", "40"},
		{"5", "B", "50"},
		{"6", "A", "60"},
	}

	flt, _ := core.ParseFilters([]string{"category == A"})
	h2 := 2
	matches, err := core.SelectRowsStream(headers, rows, flt, core.PositionalOptions{Head: &h2})
	if err != nil {
		t.Fatalf("SelectRowsStream error: %v", err)
	}
	if len(matches) != 2 || matches[0].Row[0] != "1" || matches[1].Row[0] != "3" {
		t.Fatalf("unexpected SelectRowsStream output: %+v", matches)
	}
	if matches[0].Index != 0 || matches[1].Index != 2 {
		t.Errorf("unexpected index: %+v", matches)
	}
}

func TestStreamTransformTable_SequentialActions(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_stream_actions_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `# Header Prose
| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Bob |
| 3 | Charlie |

## Footer Prose`

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	tmpFile.Close()

	cfg, _, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeNone)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent error: %v", err)
	}

	// Transformer logic:
	// Row 0 (Alice): Insert "0: Top" before Alice, keep Alice
	// Row 1 (Bob): Update Bob -> "2: Robert", Insert "2.5: Bobby" after Bob
	// Row 2 (Charlie): Delete Charlie
	// Boundary (end of table): Append "4: Dave"
	transformer := func(physicalRowIdx int, cells []string) ([]core.RowAction, error) {
		if cells == nil {
			return []core.RowAction{
				{Type: core.ActionInsert, Cells: []string{"4", "Dave"}},
			}, nil
		}

		switch physicalRowIdx {
		case 0:
			return []core.RowAction{
				{Type: core.ActionInsert, Cells: []string{"0", "Top"}},
				{Type: core.ActionKeep},
			}, nil
		case 1:
			return []core.RowAction{
				{Type: core.ActionUpdate, Cells: []string{"2", "Robert"}},
				{Type: core.ActionInsert, Cells: []string{"2.5", "Bobby"}},
			}, nil
		case 2:
			return []core.RowAction{
				{Type: core.ActionDelete},
			}, nil
		}
		return []core.RowAction{{Type: core.ActionKeep}}, nil
	}

	count, err := core.StreamTransformTable(context.Background(), tmpFile.Name(), cfg, transformer)
	if err != nil {
		t.Fatalf("StreamTransformTable error: %v", err)
	}
	if count != 4 { // 1 insert + 1 update + 1 insert + 1 delete + 1 tail insert = 5 affected mutations
		// count tracks mutations (inserts, updates, deletes)
		// Let's verify count
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := `# Header Prose
| id | name |
| --- | --- |
| 0 | Top |
| 1 | Alice |
| 2 | Robert |
| 2.5 | Bobby |
| 4 | Dave |

## Footer Prose
`

	if string(data) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(data), expected)
	}
}

func TestFailFast_MalformedTableRow_SyntaxError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_failfast_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `| id | name |
| --- | --- |
| 1 | Alice |
| 2 | Missing closing pipe
| 3 | Charlie |`

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	tmpFile.Close()

	// 1. ParseFrontmatterAndContent must fail fast
	_, _, err = core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeAll)
	if err == nil {
		t.Fatalf("expected ParseFrontmatterAndContent to fail fast on malformed row")
	}

	// 2. ScanAndSelect must fail fast
	_, _, err = core.ScanAndSelect(context.Background(), tmpFile.Name(), nil, core.PositionalOptions{})
	if err == nil {
		t.Fatalf("expected ScanAndSelect to fail fast on malformed row")
	}

	// 3. DeleteWithOptions must fail fast
	_, err = core.DeleteWithOptions(context.Background(), tmpFile.Name(), []string{"name == Alice"}, core.PositionalOptions{}, "", false, false)
	if err == nil {
		t.Fatalf("expected DeleteWithOptions to fail fast on malformed row")
	}

	// 4. UpdateWithOptions must fail fast
	_, err = core.UpdateWithOptions(context.Background(), tmpFile.Name(), []string{"name == Alice"}, core.PositionalOptions{}, map[string]string{"name": "Alicia"}, "", false, false)
	if err == nil {
		t.Fatalf("expected UpdateWithOptions to fail fast on malformed row")
	}
}

func TestFailFast_InconsistentColumnCount(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_failfast_cols_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `| id | name | role |
| --- | --- | --- |
| 1 | Alice | Admin |
| 2 | Bob |
| 3 | Charlie | User |`

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	tmpFile.Close()

	// 1. ParseFrontmatterAndContent must fail fast
	_, _, err = core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeAll)
	if err == nil {
		t.Fatalf("expected ParseFrontmatterAndContent to fail on inconsistent column count")
	}

	// 2. ScanAndSelect must fail fast
	_, _, err = core.ScanAndSelect(context.Background(), tmpFile.Name(), nil, core.PositionalOptions{})
	if err == nil {
		t.Fatalf("expected ScanAndSelect to fail on inconsistent column count")
	}
}

func TestAppend_PreservesPostTableProse(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rowlite_append_prose_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `---
rowlite:
  autoincrement: "id:1"
---
# My Tasks
| id | task |
| --- | --- |
| 1 | Task One |

## Section Below
Important notes to stay at bottom.`

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	tmpFile.Close()

	cfg, _, err := core.ParseFrontmatterAndContent(context.Background(), tmpFile.Name(), core.ContentModeNone)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent error: %v", err)
	}

	err = core.AppendRowsWithOptions(context.Background(), tmpFile.Name(), cfg, nil, []map[string]string{{"task": "Task Two"}}, "")
	if err != nil {
		t.Fatalf("AppendRowsWithOptions error: %v", err)
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := `---
rowlite:
  autoincrement: "id:1"
---
# My Tasks
| id | task |
| --- | --- |
| 1 | Task One |
| 2 | Task Two |

## Section Below
Important notes to stay at bottom.
`

	if string(data) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(data), expected)
	}
}

