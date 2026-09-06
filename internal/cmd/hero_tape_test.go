package cmd_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pipik-roman/rowlite/internal/cmd"
	"github.com/pipik-roman/rowlite/internal/core"
)

func TestHeroTapeSequence_EmpiricalVerification(t *testing.T) {
	tmpDir := t.TempDir()
	demoFile := filepath.Join(tmpDir, "demo_tasks.md")

	// =========================================================================
	// Step 1: Initialize table schema with autoincrement ID & audit logging
	// Command: rowlite init tasks.md -c id,task,status,priority -a id --audit
	// =========================================================================
	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{
		demoFile,
		"-c", "id,task,status,priority",
		"-a", "id",
		"--audit",
	})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("Step 1 (init) failed: %v", err)
	}

	// Verify Step 1 on-disk YAML frontmatter and table headers
	step1Data, err := os.ReadFile(demoFile)
	if err != nil {
		t.Fatalf("failed to read initialized demo_tasks.md: %v", err)
	}
	step1Content := string(step1Data)
	t.Logf("Step 1 On-Disk Content:\n%s", step1Content)

	if !strings.Contains(step1Content, "autoincrement:") || !strings.Contains(step1Content, `column: "id"`) {
		t.Errorf("Step 1: expected frontmatter to contain autoincrement column: \"id\", got:\n%s", step1Content)
	}
	if !strings.Contains(step1Content, "audit:") || !strings.Contains(step1Content, `insert: "created_at"`) || !strings.Contains(step1Content, `edit: "updated_at"`) {
		t.Errorf("Step 1: expected frontmatter to contain audit insert/edit, got:\n%s", step1Content)
	}
	if !strings.Contains(step1Content, "| id | task | status | priority | created_at | updated_at |") {
		t.Errorf("Step 1: expected GFM table header with columns and audit fields, got:\n%s", step1Content)
	}

	// =========================================================================
	// Step 2: Append rows without manual ID or timestamp management
	// Row 1: {"task":"Implement MCP protocol","status":"pending","priority":"high"}
	// Row 2: {"task":"Record hero demo GIF","status":"in_progress","priority":"medium"}
	// =========================================================================
	appCmd1 := silenceCmd(cmd.NewAppendCmd())
	appCmd1.SetArgs([]string{
		demoFile,
		`{"task":"Implement MCP protocol","status":"pending","priority":"high"}`,
	})
	if err := appCmd1.Execute(); err != nil {
		t.Fatalf("Step 2 (append row 1) failed: %v", err)
	}

	// Brief sleep to ensure distinct timestamps for update test
	time.Sleep(100 * time.Millisecond)

	appCmd2 := silenceCmd(cmd.NewAppendCmd())
	appCmd2.SetArgs([]string{
		demoFile,
		`{"task":"Record hero demo GIF","status":"in_progress","priority":"medium"}`,
	})
	if err := appCmd2.Execute(); err != nil {
		t.Fatalf("Step 2 (append row 2) failed: %v", err)
	}

	// Verify Step 2 on-disk row contents, autoincrement IDs (1 and 2), and timestamps
	step2Data, err := os.ReadFile(demoFile)
	if err != nil {
		t.Fatalf("failed to read demo_tasks.md after appends: %v", err)
	}
	step2Content := string(step2Data)
	t.Logf("Step 2 On-Disk Content:\n%s", step2Content)

	if !strings.Contains(step2Content, "| 1 | Implement MCP protocol | pending | high |") {
		t.Errorf("Step 2: expected row 1 with auto-increment ID 1, got:\n%s", step2Content)
	}
	if !strings.Contains(step2Content, "| 2 | Record hero demo GIF | in_progress | medium |") {
		t.Errorf("Step 2: expected row 2 with auto-increment ID 2, got:\n%s", step2Content)
	}

	// =========================================================================
	// Step 3: Query table with clean Unicode formatting (list --format pretty) & structure
	// =========================================================================
	listCmd := silenceCmd(cmd.NewListCmd())
	var listBuf bytes.Buffer
	listCmd.SetOut(&listBuf)
	listCmd.SetArgs([]string{demoFile, "--format", "pretty"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("Step 3 (list --format pretty) failed: %v", err)
	}
	listPrettyOut := listBuf.String()
	t.Logf("Step 3 Pretty Output:\n%s", listPrettyOut)

	if !strings.Contains(listPrettyOut, "┌") || !strings.Contains(listPrettyOut, "└") || !strings.Contains(listPrettyOut, "│") {
		t.Errorf("Step 3: expected pretty output with box-drawing Unicode characters, got:\n%s", listPrettyOut)
	}
	if !strings.Contains(listPrettyOut, "Implement MCP protocol") || !strings.Contains(listPrettyOut, "Record hero demo GIF") {
		t.Errorf("Step 3: expected pretty output to display both appended tasks, got:\n%s", listPrettyOut)
	}

	// Verify structure command
	structCmd := silenceCmd(cmd.NewStructureCmd())
	var structBuf bytes.Buffer
	structCmd.SetOut(&structBuf)
	structCmd.SetArgs([]string{demoFile})
	if err := structCmd.Execute(); err != nil {
		t.Fatalf("Step 3 (structure) failed: %v", err)
	}
	structOut := structBuf.String()
	t.Logf("Step 3 Structure Output:\n%s", structOut)

	if !strings.Contains(structOut, "Columns: id, task, status, priority, created_at, updated_at") {
		t.Errorf("Step 3: expected structure columns list, got:\n%s", structOut)
	}
	if !strings.Contains(structOut, "Audit Insert Column: created_at") {
		t.Errorf("Step 3: expected structure audit insert column, got:\n%s", structOut)
	}
	if !strings.Contains(structOut, "Audit Edit Column: updated_at") {
		t.Errorf("Step 3: expected structure audit edit column, got:\n%s", structOut)
	}

	// =========================================================================
	// Step 4: Precisely update row fields in-place
	// Command: rowlite update demo_tasks.md --where 'id == 1' --set 'status=completed'
	// =========================================================================
	// Capture initial row 1 updated_at timestamp prior to update
	cfgBefore, rowsBefore, err := core.ParseFrontmatterAndContent(context.Background(), demoFile, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse before update: %v", err)
	}
	_ = cfgBefore
	if len(rowsBefore) != 2 {
		t.Fatalf("expected 2 rows before update, found %d", len(rowsBefore))
	}
	row1CreatedAtBefore := rowsBefore[0][4]
	row1UpdatedAtBefore := rowsBefore[0][5]

	// Wait to guarantee updated_at differs by at least 1 second if timestamps are second-resolution
	time.Sleep(1100 * time.Millisecond)

	updateCmd := silenceCmd(cmd.NewUpdateCmd())
	var updateBuf bytes.Buffer
	updateCmd.SetOut(&updateBuf)
	updateCmd.SetArgs([]string{
		demoFile,
		"--where", "id == 1",
		"--set", "status=completed",
	})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("Step 4 (update) failed: %v", err)
	}

	// =========================================================================
	// Step 5: Verify auto-formatted Markdown table on disk (cat demo_tasks.md)
	// =========================================================================
	step5Data, err := os.ReadFile(demoFile)
	if err != nil {
		t.Fatalf("Step 5 (cat) failed to read demo_tasks.md: %v", err)
	}
	step5Content := string(step5Data)
	t.Logf("Step 5 On-Disk Content:\n%s", step5Content)

	// Verify targeted in-place mutation: row 1 status updated, row 2 unchanged
	if !strings.Contains(step5Content, "| 1 | Implement MCP protocol | completed | high |") {
		t.Errorf("Step 5: expected row 1 status to be mutated to 'completed', got:\n%s", step5Content)
	}
	if !strings.Contains(step5Content, "| 2 | Record hero demo GIF | in_progress | medium |") {
		t.Errorf("Step 5: expected row 2 status to remain 'in_progress', got:\n%s", step5Content)
	}

	// Verify created_at was preserved and updated_at was refreshed
	_, rowsAfter, err := core.ParseFrontmatterAndContent(context.Background(), demoFile, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse table after update: %v", err)
	}
	row1CreatedAtAfter := rowsAfter[0][4]
	row1UpdatedAtAfter := rowsAfter[0][5]

	if row1CreatedAtBefore != row1CreatedAtAfter {
		t.Errorf("Step 5: expected created_at to remain unchanged (%q), got %q", row1CreatedAtBefore, row1CreatedAtAfter)
	}
	if row1UpdatedAtBefore == row1UpdatedAtAfter {
		t.Errorf("Step 5: expected updated_at timestamp to be refreshed upon update, but remained %q", row1UpdatedAtAfter)
	}

	// Row 2 created_at and updated_at should remain unchanged
	if rowsBefore[1][4] != rowsAfter[1][4] || rowsBefore[1][5] != rowsAfter[1][5] {
		t.Errorf("Step 5: expected row 2 timestamps to remain untouched, got before=%v, after=%v", rowsBefore[1], rowsAfter[1])
	}
}

func TestDemoGifSizeGuardrail(t *testing.T) {
	gifPath := filepath.Join("..", "..", "docs", "assets", "rowlite-demo.gif")
	stat, err := os.Stat(gifPath)
	if err != nil {
		t.Fatalf("expected demo GIF file to exist at %s, got error: %v", gifPath, err)
	}
	const maxAllowedBytes = 3670016 // 3.5 MB = 3.5 * 1024 * 1024 = 3,670,016 bytes
	if stat.Size() > maxAllowedBytes {
		t.Errorf("demo GIF file size %d bytes exceeds strict guardrail of %d bytes (3.5 MB)", stat.Size(), maxAllowedBytes)
	}
	t.Logf("Demo GIF verified: %s (%d bytes, <= %d bytes limit)", gifPath, stat.Size(), maxAllowedBytes)
}
