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

// TestAdversarial_SurroundingMarkdownIntegrity verifies that surrounding notes,
// preambles, and postambles in markdown files are never eaten or corrupted during CRUD.
func TestAdversarial_SurroundingMarkdownIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "document_with_context.md")

	initialContent := `---
rowlite:
  autoincrement:
    column: "id"
    step: 1
  audit:
    insert: "created_at"
    edit: "updated_at"
---
# Project Roadmap

Here is some introductory context that MUST be preserved.
- Note A
- Note B

| id | task | status | created_at | updated_at |
| --- | --- | --- | --- | --- |
| 1 | Setup repo | done | 2026-08-28 10:00:00 | 2026-08-28 10:00:00 |

## Additional Notes
This section appears AFTER the table and must also remain completely intact.
`
	if err := os.WriteFile(dbPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	// 1. Append a new row
	appCmd := silenceCmd(cmd.NewAppendCmd())
	appCmd.SetArgs([]string{dbPath, `{"task":"Add CI workflow","status":"pending"}`})
	if err := appCmd.Execute(); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	// 2. Update row 1
	time.Sleep(100 * time.Millisecond)
	updateCmd := silenceCmd(cmd.NewUpdateCmd())
	updateCmd.SetArgs([]string{dbPath, "--where", "id == 1", "--set", "status=verified"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// 3. Delete row 2
	delCmd := silenceCmd(cmd.NewDeleteCmd())
	delCmd.SetArgs([]string{dbPath, "--where", "id == 2"})
	if err := delCmd.Execute(); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// 4. Verify disk content preserves preamble and postamble
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("failed to read disk file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "# Project Roadmap") {
		t.Errorf("preamble heading was lost: %s", content)
	}
	if !strings.Contains(content, "Here is some introductory context that MUST be preserved.") {
		t.Errorf("preamble text was lost: %s", content)
	}
	if !strings.Contains(content, "## Additional Notes") {
		t.Errorf("postamble heading was lost: %s", content)
	}
	if !strings.Contains(content, "This section appears AFTER the table and must also remain completely intact.") {
		t.Errorf("postamble text was lost: %s", content)
	}

	// Verify table mutations occurred accurately
	if !strings.Contains(content, "| 1 | Setup repo | verified |") {
		t.Errorf("row 1 was not updated properly: %s", content)
	}
}

// TestAdversarial_SpecialCharactersAndPipes verifies proper escaping and parsing
// of pipe characters, unicode emojis, and quotes inside table cells.
func TestAdversarial_SpecialCharactersAndPipes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "special_chars.md")

	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,expression,notes", "--autoincrement", "id"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Insert row with pipe and emoji
	appCmd := silenceCmd(cmd.NewAppendCmd())
	appCmd.SetArgs([]string{dbPath, `{"expression":"A | B || C", "notes":"Rocket 🚀 and Checkmark ✅"}`})
	if err := appCmd.Execute(); err != nil {
		t.Fatalf("append with pipe and emoji failed: %v", err)
	}

	// Query via list --format pretty
	listCmd := silenceCmd(cmd.NewListCmd())
	var listBuf bytes.Buffer
	listCmd.SetOut(&listBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "pretty"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list pretty failed: %v", err)
	}
	prettyOut := listBuf.String()

	if !strings.Contains(prettyOut, "A | B || C") {
		t.Errorf("pipe content corrupted in list pretty: %s", prettyOut)
	}
	if !strings.Contains(prettyOut, "Rocket 🚀 and Checkmark ✅") {
		t.Errorf("emoji content corrupted in list pretty: %s", prettyOut)
	}

	// Update targeted row using where clause matching pipe content
	updateCmd := silenceCmd(cmd.NewUpdateCmd())
	updateCmd.SetArgs([]string{dbPath, "--where", "expression like A | B", "--set", "notes=Updated Note 🎯"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update with pipe where clause failed: %v", err)
	}

	// Verify disk representation
	_, rows, err := core.ParseFrontmatterAndContent(context.Background(), dbPath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse table after update: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][1] != "A | B || C" {
		t.Errorf("expected expression cell 'A | B || C', got %q", rows[0][1])
	}
	if rows[0][2] != "Updated Note 🎯" {
		t.Errorf("expected notes cell 'Updated Note 🎯', got %q", rows[0][2])
	}
}

// TestAdversarial_AutoincrementMonotonicityOnDeletion tests that deleted rows
// do not cause autoincrement ID collisions on subsequent appends.
func TestAdversarial_AutoincrementMonotonicityOnDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "monotonic.md")

	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,item", "--autoincrement", "id"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Append rows 1, 2, 3
	for _, item := range []string{"First", "Second", "Third"} {
		c := silenceCmd(cmd.NewAppendCmd())
		c.SetArgs([]string{dbPath, `{"item":"` + item + `"}`})
		if err := c.Execute(); err != nil {
			t.Fatalf("append %s failed: %v", item, err)
		}
	}

	// Delete row 2
	delCmd := silenceCmd(cmd.NewDeleteCmd())
	delCmd.SetArgs([]string{dbPath, "--where", "id == 2"})
	if err := delCmd.Execute(); err != nil {
		t.Fatalf("delete row 2 failed: %v", err)
	}

	// Append fourth row -> should receive id 4 (max(1, 3) + 1 = 4)
	appCmd := silenceCmd(cmd.NewAppendCmd())
	appCmd.SetArgs([]string{dbPath, `{"item":"Fourth"}`})
	if err := appCmd.Execute(); err != nil {
		t.Fatalf("append after delete failed: %v", err)
	}

	// Verify all rows in table
	_, rows, err := core.ParseFrontmatterAndContent(context.Background(), dbPath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("failed to parse table: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows remaining, got %d", len(rows))
	}
	if rows[0][0] != "1" || rows[1][0] != "3" || rows[2][0] != "4" {
		t.Errorf("expected IDs [1, 3, 4], got [%s, %s, %s]", rows[0][0], rows[1][0], rows[2][0])
	}
}
