package cmd_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/roman-pipik/rowlite/internal/cmd"
)

func silenceCmd(c *cobra.Command) *cobra.Command {
	c.SilenceErrors = true
	c.SilenceUsage = true
	return c
}

func TestInitCmd(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "db.md")

	c := silenceCmd(cmd.NewInitCmd())
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetArgs([]string{dbPath, "--columns", "id,name,age", "--unique", "id", "--sort", "id:asc"})

	err := c.Execute()
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("expected database file to be created, but it does not exist")
	}

	// Run structure command to verify configuration
	structCmd := silenceCmd(cmd.NewStructureCmd())
	var structBuf bytes.Buffer
	structCmd.SetOut(&structBuf)
	structCmd.SetArgs([]string{dbPath})

	err = structCmd.Execute()
	if err != nil {
		t.Fatalf("structure command failed: %v", err)
	}

	out := structBuf.String()
	if !strings.Contains(out, "Columns: id, name, age") {
		t.Errorf("expected output to contain columns, got: %s", out)
	}
	if !strings.Contains(out, "Unique Keys: id") {
		t.Errorf("expected output to contain unique keys, got: %s", out)
	}
	if !strings.Contains(out, "Sort Configuration: id:asc") {
		t.Errorf("expected output to contain sort config, got: %s", out)
	}
}

func TestInitCmdErrors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.md")

	c := silenceCmd(cmd.NewInitCmd())
	c.SetArgs([]string{dbPath}) // no --columns flag
	err := c.Execute()
	if err == nil {
		t.Fatalf("expected init command to fail without --columns on a new file, but it succeeded")
	}
	if !strings.Contains(err.Error(), "--columns flag is required") {
		t.Errorf("expected error message to contain '--columns flag is required', got: %v", err)
	}
}

func TestAppendAndListAndCountCmd(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "db.md")

	// 1. Initialize
	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,name,age"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// 2. Append row 1 using positional argument
	appendCmd1 := silenceCmd(cmd.NewAppendCmd())
	var appBuf1 bytes.Buffer
	appendCmd1.SetOut(&appBuf1)
	appendCmd1.SetArgs([]string{dbPath, `{"id":"2", "name":"Bob", "age":"30"}`})
	if err := appendCmd1.Execute(); err != nil {
		t.Fatalf("append 1 failed: %v", err)
	}

	// 3. Append row 2 using --row flag
	appendCmd2 := silenceCmd(cmd.NewAppendCmd())
	var appBuf2 bytes.Buffer
	appendCmd2.SetOut(&appBuf2)
	appendCmd2.SetArgs([]string{dbPath, "--row", `{"id":"1", "name":"Alice", "age":"25"}`})
	if err := appendCmd2.Execute(); err != nil {
		t.Fatalf("append 2 failed: %v", err)
	}

	// 4. List rows
	listCmd := silenceCmd(cmd.NewListCmd())
	var listBuf bytes.Buffer
	listCmd.SetOut(&listBuf)
	listCmd.SetArgs([]string{dbPath})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	listOut := listBuf.String()
	// Output should contain headers and rows in GFM format
	if !strings.Contains(listOut, "| id | name | age |") {
		t.Errorf("list output missing headers, got: %s", listOut)
	}
	if !strings.Contains(listOut, "| 2 | Bob | 30 |") {
		t.Errorf("list output missing Bob, got: %s", listOut)
	}
	if !strings.Contains(listOut, "| 1 | Alice | 25 |") {
		t.Errorf("list output missing Alice, got: %s", listOut)
	}

	// 5. Count rows (total)
	countCmd := silenceCmd(cmd.NewCountCmd())
	var countBuf bytes.Buffer
	countCmd.SetOut(&countBuf)
	countCmd.SetArgs([]string{dbPath})
	if err := countCmd.Execute(); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if strings.TrimSpace(countBuf.String()) != "2" {
		t.Errorf("expected count 2, got: %s", countBuf.String())
	}

	// 6. Count rows with filter
	countCmd2 := silenceCmd(cmd.NewCountCmd())
	var countBuf2 bytes.Buffer
	countCmd2.SetOut(&countBuf2)
	countCmd2.SetArgs([]string{dbPath, "--where", "age==30"})
	if err := countCmd2.Execute(); err != nil {
		t.Fatalf("count with filter failed: %v", err)
	}
	if strings.TrimSpace(countBuf2.String()) != "1" {
		t.Errorf("expected filtered count 1, got: %s", countBuf2.String())
	}
}

func TestSortCmd(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "db.md")

	// 1. Initialize without auto-sorting
	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,name"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// 2. Append rows out of order
	appendCmd := silenceCmd(cmd.NewAppendCmd())
	appendCmd.SetArgs([]string{dbPath, `{"id":"2", "name":"Bob"}`})
	if err := appendCmd.Execute(); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	appendCmd2 := silenceCmd(cmd.NewAppendCmd())
	appendCmd2.SetArgs([]string{dbPath, `{"id":"1", "name":"Alice"}`})
	if err := appendCmd2.Execute(); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	// 3. List and verify they are in order of insertion (2, then 1)
	listCmd1 := silenceCmd(cmd.NewListCmd())
	var listBuf1 bytes.Buffer
	listCmd1.SetOut(&listBuf1)
	listCmd1.SetArgs([]string{dbPath})
	if err := listCmd1.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(listBuf1.String()), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got: %v", lines)
	}
	if !strings.Contains(lines[2], "Bob") || !strings.Contains(lines[3], "Alice") {
		t.Errorf("expected Bob then Alice, got: %s", listBuf1.String())
	}

	// 4. Sort table by id:asc
	sortCmd := silenceCmd(cmd.NewSortCmd())
	sortCmd.SetArgs([]string{dbPath, "--by", "id:asc"})
	if err := sortCmd.Execute(); err != nil {
		t.Fatalf("sort failed: %v", err)
	}

	// 5. List again and verify sorted order (1, then 2)
	listCmd2 := silenceCmd(cmd.NewListCmd())
	var listBuf2 bytes.Buffer
	listCmd2.SetOut(&listBuf2)
	listCmd2.SetArgs([]string{dbPath})
	if err := listCmd2.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	lines2 := strings.Split(strings.TrimSpace(listBuf2.String()), "\n")
	if len(lines2) < 4 {
		t.Fatalf("expected at least 4 lines, got: %v", lines2)
	}
	if !strings.Contains(lines2[2], "Alice") || !strings.Contains(lines2[3], "Bob") {
		t.Errorf("expected Alice then Bob, got: %s", listBuf2.String())
	}
}

func TestDebugFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "db.md")

	// 1. Initialize a database file
	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,name"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// 2. Redirect os.Stderr to a pipe
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	// 3. Execute root command with --debug list
	rootCmd := silenceCmd(cmd.NewRootCmd())
	rootCmd.SetArgs([]string{"list", dbPath, "--debug"})

	execErr := rootCmd.Execute()
	w.Close() // close writer so read doesn't block

	if execErr != nil {
		t.Fatalf("root list --debug execution failed: %v", execErr)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "DEBUG TableConfig") {
		t.Errorf("expected debug output to contain 'DEBUG TableConfig', got:\n%s", output)
	}
}
