package cmd_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/pipik-roman/rowlite/internal/cmd"
	"github.com/pipik-roman/rowlite/internal/core"
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

func TestDebugFlag_MalformedTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "bad.md")

	content := `---
type: table
---
# Test Bad Table
| Id | Tags | Idea | Resources |
|----|------|------| |
`
	if err := os.WriteFile(dbPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write bad table file: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	rootCmd := silenceCmd(cmd.NewRootCmd())
	rootCmd.SetArgs([]string{"list", dbPath, "--debug"})

	execErr := rootCmd.Execute()
	w.Close()

	if execErr == nil {
		t.Fatalf("expected execution to fail for malformed table, got nil")
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "DEBUG [table scan]: candidate table header found at line 5") {
		t.Errorf("expected debug output to contain candidate header line info, got:\n%s", output)
	}
	if !strings.Contains(output, "DEBUG [table scan]: separator validation failed at line 6") {
		t.Errorf("expected debug output to contain separator validation failure at line 6, got:\n%s", output)
	}
	if !strings.Contains(execErr.Error(), "invalid separator row at line 6: column 4 is empty and missing hyphens ('-')") {
		t.Errorf("expected execErr to contain detailed line 6 error, got: %v", execErr)
	}
}

func TestListCmdFormats(t *testing.T) {
	dbPath := filepath.Join("testdata", "clean_table.md")

	// 1. Test format "json"
	listCmd := silenceCmd(cmd.NewListCmd())
	var jsonBuf bytes.Buffer
	listCmd.SetOut(&jsonBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "json"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("json format list failed: %v", err)
	}
	jsonOut := jsonBuf.String()
	if !strings.Contains(jsonOut, `"id": "101"`) || !strings.Contains(jsonOut, `"task": "Fix Auth"`) || !strings.Contains(jsonOut, `"status": "active"`) {
		t.Errorf("expected JSON output containing fields, got: %s", jsonOut)
	}

	// 2. Test format "csv"
	var csvBuf bytes.Buffer
	listCmd = silenceCmd(cmd.NewListCmd())
	listCmd.SetOut(&csvBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "csv"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("csv format list failed: %v", err)
	}
	csvOut := csvBuf.String()
	if !strings.Contains(csvOut, "id,task,status\n") || !strings.Contains(csvOut, "101,Fix Auth,active\n") {
		t.Errorf("expected CSV output, got: %s", csvOut)
	}

	// 3. Test format "tsv"
	var tsvBuf bytes.Buffer
	listCmd = silenceCmd(cmd.NewListCmd())
	listCmd.SetOut(&tsvBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "tsv"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("tsv format list failed: %v", err)
	}
	tsvOut := tsvBuf.String()
	if !strings.Contains(tsvOut, "id\ttask\tstatus\n") || !strings.Contains(tsvOut, "101\tFix Auth\tactive\n") {
		t.Errorf("expected TSV output, got: %s", tsvOut)
	}

	// 4. Test format "pretty"
	var prettyBuf bytes.Buffer
	listCmd = silenceCmd(cmd.NewListCmd())
	listCmd.SetOut(&prettyBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "pretty"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("pretty format list failed: %v", err)
	}
	prettyOut := prettyBuf.String()
	// Box drawing characters should be present
	if !strings.Contains(prettyOut, "┌") || !strings.Contains(prettyOut, "Fix Auth") || !strings.Contains(prettyOut, "└") {
		t.Errorf("expected pretty table output with box characters, got: %s", prettyOut)
	}

	// 5. Test format "md"
	var mdBuf bytes.Buffer
	listCmd = silenceCmd(cmd.NewListCmd())
	listCmd.SetOut(&mdBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "md"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("md format list failed: %v", err)
	}
	mdOut := mdBuf.String()
	if !strings.Contains(mdOut, "| id | task | status |") {
		t.Errorf("expected GFM table headers, got: %s", mdOut)
	}

	// 6. Test invalid format error
	listCmd = silenceCmd(cmd.NewListCmd())
	listCmd.SetArgs([]string{dbPath, "--format", "invalid_mode"})
	if err := listCmd.Execute(); err == nil {
		t.Fatal("expected error with invalid format, got nil")
	} else if !strings.Contains(err.Error(), "invalid format:") {
		t.Errorf("expected invalid format error, got: %v", err)
	}

	// 7. Test format "auto" falling back to GFM in non-TTY bytes.Buffer
	var autoBuf bytes.Buffer
	listCmd = silenceCmd(cmd.NewListCmd())
	listCmd.SetOut(&autoBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "auto"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("auto format list failed: %v", err)
	}
	autoOut := autoBuf.String()
	if !strings.Contains(autoOut, "| id | task | status |") {
		t.Errorf("expected auto fallback to GFM Markdown table, got: %s", autoOut)
	}
}

func TestRendererAlignments(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "align.md")

	content := `---
type: table
---
# Aligned Table
| Left | Center | Right |
| :--- | :---: | ---: |
| L1   | C1     | R1    |
`
	if err := os.WriteFile(dbPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write align.md: %v", err)
	}

	// 1. Test pretty format alignment
	listCmd := silenceCmd(cmd.NewListCmd())
	var prettyBuf bytes.Buffer
	listCmd.SetOut(&prettyBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "pretty"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("pretty list failed: %v", err)
	}
	prettyOut := prettyBuf.String()
	// Right-aligned column "Right" (width 5, val "R1"): 1 leading cell space + 3 pad spaces + R1
	if !strings.Contains(prettyOut, "│    R1 │") {
		t.Errorf("expected right-aligned value '│    R1 │' in pretty output, got:\n%s", prettyOut)
	}
	// Center-aligned column "Center" (width 6, val "C1"): leftPad 2, rightPad 2
	if !strings.Contains(prettyOut, "│   C1   │") {
		t.Errorf("expected center-aligned value '│   C1   │' in pretty output, got:\n%s", prettyOut)
	}

	// 2. Test md format alignment preservation
	listCmd = silenceCmd(cmd.NewListCmd())
	var mdBuf bytes.Buffer
	listCmd.SetOut(&mdBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "md"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("md list failed: %v", err)
	}
	mdOut := mdBuf.String()
	if !strings.Contains(mdOut, "| :--- | :---: | ---: |") {
		t.Errorf("expected preserved alignments in md separator row, got:\n%s", mdOut)
	}
}

func TestMCPConfigCache(t *testing.T) {
	cache := cmd.NewTableConfigCache()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "cache_test.md")
	content := `---
rowlite:
  unique: [id]
  autoincrement:
    column: id
    step: 5
---
| id | name |
| --- | --- |
| 5 | Alice |
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 1st call: parses and caches
	cfg1, _, err := cache.GetOrParse(context.Background(), filePath, core.ContentModeNone)
	if err != nil {
		t.Fatalf("GetOrParse 1 failed: %v", err)
	}
	if cfg1.AutoIncrement.Column != "id" || cfg1.AutoIncrement.Step != 5 {
		t.Errorf("unexpected AutoIncrement config: %+v", cfg1.AutoIncrement)
	}

	// 2nd call: cache hit (file not modified)
	cfg2, _, err := cache.GetOrParse(context.Background(), filePath, core.ContentModeNone)
	if err != nil {
		t.Fatalf("GetOrParse 2 failed: %v", err)
	}
	if cfg2.AutoIncrement.Column != "id" || cfg2.AutoIncrement.Step != 5 {
		t.Errorf("unexpected cached AutoIncrement config: %+v", cfg2.AutoIncrement)
	}
}

func TestListCmd_LargeTable10k_SpecialCharacters(t *testing.T) {
	dbPath := filepath.Join("testdata", "large_table_10k.md")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("10k table fixture not found at %s", dbPath)
	}

	// 1. Test CLI list with JSON format and special character query (🚀)
	listCmd := silenceCmd(cmd.NewListCmd())
	var jsonBuf bytes.Buffer
	listCmd.SetOut(&jsonBuf)
	listCmd.SetArgs([]string{dbPath, "--where", "Task Description like 🚀", "--format", "json"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list cmd failed: %v", err)
	}
	jsonOut := jsonBuf.String()
	if !strings.Contains(jsonOut, "Deploy service to Kubernetes cluster 🚀") || !strings.Contains(jsonOut, `"Category Name": "DevOps & CI/CD"`) {
		t.Errorf("expected JSON output with emoji and special chars, got: %s", jsonOut)
	}

	// 2. Test CLI list with escaped pipe query
	listCmd = silenceCmd(cmd.NewListCmd())
	var csvBuf bytes.Buffer
	listCmd.SetOut(&csvBuf)
	listCmd.SetArgs([]string{dbPath, "--where", "Task Description like Option A | Option B", "--format", "csv"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list cmd with pipe search failed: %v", err)
	}
	csvOut := csvBuf.String()
	if !strings.Contains(csvOut, "Option A | Option B") {
		t.Errorf("expected CSV output with pipe search, got: %s", csvOut)
	}

	// 3. Test CLI count fast path
	countCmd := silenceCmd(cmd.NewCountCmd())
	var countBuf bytes.Buffer
	countCmd.SetOut(&countBuf)
	countCmd.SetArgs([]string{dbPath})
	if err := countCmd.Execute(); err != nil {
		t.Fatalf("count cmd failed: %v", err)
	}
	if strings.TrimSpace(countBuf.String()) != "10000" {
		t.Errorf("expected count 10000, got: %s", countBuf.String())
	}
}

func TestListAndCountPositionalOptions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "items.md")

	// 1. Initialize
	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,name,status"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// 2. Append 6 rows
	rows := []string{
		`{"id":"1", "name":"Alpha", "status":"active"}`,
		`{"id":"2", "name":"Beta", "status":"pending"}`,
		`{"id":"3", "name":"Gamma", "status":"active"}`,
		`{"id":"4", "name":"Delta", "status":"active"}`,
		`{"id":"5", "name":"Epsilon", "status":"inactive"}`,
		`{"id":"6", "name":"Zeta", "status":"active"}`,
	}
	for i, r := range rows {
		appCmd := silenceCmd(cmd.NewAppendCmd())
		appCmd.SetArgs([]string{dbPath, r})
		if err := appCmd.Execute(); err != nil {
			t.Fatalf("append %d failed: %v", i, err)
		}
	}

	// 3. Test Head 2
	listCmd := silenceCmd(cmd.NewListCmd())
	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--head", "2"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("head list failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") || strings.Contains(out, "Gamma") {
		t.Errorf("unexpected head 2 output: %s", out)
	}

	// 4. Test Tail 2
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--tail", "2"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("tail list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Epsilon") || !strings.Contains(out, "Zeta") || strings.Contains(out, "Delta") {
		t.Errorf("unexpected tail 2 output: %s", out)
	}

	// 5. Test Head 2 and Tail 2 partition
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--head", "2", "--tail", "2"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("head/tail partition list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") || !strings.Contains(out, "Epsilon") || !strings.Contains(out, "Zeta") || strings.Contains(out, "Gamma") {
		t.Errorf("unexpected head/tail partition output: %s", out)
	}

	// 6. Test Index list "1, 3, 5"
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--index", "1, 3, 5"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("index list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Beta") || !strings.Contains(out, "Delta") || !strings.Contains(out, "Zeta") || strings.Contains(out, "Alpha") {
		t.Errorf("unexpected index list output: %s", out)
	}

	// 7. Test Stepped Slice "0..5..2"
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--index", "0..5..2"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("stepped slice list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Gamma") || !strings.Contains(out, "Epsilon") || strings.Contains(out, "Beta") {
		t.Errorf("unexpected stepped slice output: %s", out)
	}

	// 8. Test Negative range "-2..-1"
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--index", "-2..-1"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("negative range list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Epsilon") || !strings.Contains(out, "Zeta") || strings.Contains(out, "Alpha") {
		t.Errorf("unexpected negative range output: %s", out)
	}

	// 9. Test Offset and Limit pagination
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--offset", "2", "--limit", "2"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("offset/limit list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Gamma") || !strings.Contains(out, "Delta") || strings.Contains(out, "Alpha") || strings.Contains(out, "Epsilon") {
		t.Errorf("unexpected offset/limit output: %s", out)
	}

	// 10. Test Where combined with Head
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--where", "status==active", "--head", "2"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("where + head list failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Gamma") || strings.Contains(out, "Delta") || strings.Contains(out, "Zeta") {
		t.Errorf("unexpected where + head output: %s", out)
	}

	// 11. Test --include-row-number with JSON
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--head", "2", "--format", "json", "--include-row-number"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("json with row number failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, `"__row_index__": 0`) || !strings.Contains(out, `"__row_index__": 1`) {
		t.Errorf("expected __row_index__ in JSON output: %s", out)
	}

	// 12. Test --include-row-number with Pretty
	listCmd = silenceCmd(cmd.NewListCmd())
	buf.Reset()
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--head", "1", "--format", "pretty", "--include-row-number"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("pretty with row number failed: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "│ # ") || !strings.Contains(out, "│ 0 ") {
		t.Errorf("expected # column in pretty table: %s", out)
	}

	// 13. Test Count with positional flags
	countCmd := silenceCmd(cmd.NewCountCmd())
	var countBuf bytes.Buffer
	countCmd.SetOut(&countBuf)
	countCmd.SetArgs([]string{dbPath, "--head", "3"})
	if err := countCmd.Execute(); err != nil {
		t.Fatalf("count head failed: %v", err)
	}
	if strings.TrimSpace(countBuf.String()) != "3" {
		t.Errorf("expected count 3, got: %s", countBuf.String())
	}

	// 14. Test Count with Where + Head
	countCmd = silenceCmd(cmd.NewCountCmd())
	countBuf.Reset()
	countCmd.SetOut(&countBuf)
	countCmd.SetArgs([]string{dbPath, "--where", "status==active", "--head", "2"})
	if err := countCmd.Execute(); err != nil {
		t.Fatalf("count where+head failed: %v", err)
	}
	if strings.TrimSpace(countBuf.String()) != "2" {
		t.Errorf("expected count 2, got: %s", countBuf.String())
	}

	// 15. Test Count with Stepped Slice
	countCmd = silenceCmd(cmd.NewCountCmd())
	countBuf.Reset()
	countCmd.SetOut(&countBuf)
	countCmd.SetArgs([]string{dbPath, "--index", "0..5..2"})
	if err := countCmd.Execute(); err != nil {
		t.Fatalf("count index slice failed: %v", err)
	}
	if strings.TrimSpace(countBuf.String()) != "3" {
		t.Errorf("expected count 3, got: %s", countBuf.String())
	}
}

func TestFilterFirst_ExecutionOrder_DistinctFromPositionFirst(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "filter_first.md")

	// 1. Initialize table
	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,title,status"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// 2. Insert records where first 2 rows are "Complete", and subsequent rows are "Pending"
	// Physical Row 0: Complete
	// Physical Row 1: Complete
	// Physical Row 2: Pending (First Pending row)
	// Physical Row 3: Pending (Second Pending row)
	// Physical Row 4: Complete
	// Physical Row 5: Pending (Third Pending row)
	testRows := []string{
		`{"id":"1", "title":"Task 1", "status":"Complete"}`,
		`{"id":"2", "title":"Task 2", "status":"Complete"}`,
		`{"id":"3", "title":"Task 3", "status":"Pending"}`,
		`{"id":"4", "title":"Task 4", "status":"Pending"}`,
		`{"id":"5", "title":"Task 5", "status":"Complete"}`,
		`{"id":"6", "title":"Task 6", "status":"Pending"}`,
	}
	for i, r := range testRows {
		appCmd := silenceCmd(cmd.NewAppendCmd())
		appCmd.SetArgs([]string{dbPath, r})
		if err := appCmd.Execute(); err != nil {
			t.Fatalf("append %d failed: %v", i, err)
		}
	}

	// -------------------------------------------------------------------------
	// Case 1: --where "status == Pending" --head 2
	// If Position-First: takes physical rows 0 & 1 ("Complete"), filters for "Pending" -> 0 rows returned!
	// If Filter-First: filters to rows [2, 3, 5], takes first 2 -> Task 3 & Task 4 returned.
	// -------------------------------------------------------------------------
	listCmd := silenceCmd(cmd.NewListCmd())
	var listBuf bytes.Buffer
	listCmd.SetOut(&listBuf)
	listCmd.SetArgs([]string{dbPath, "--where", "status==Pending", "--head", "2", "--include-row-number"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	out := listBuf.String()

	// Must contain Task 3 and Task 4
	if !strings.Contains(out, "Task 3") || !strings.Contains(out, "Task 4") {
		t.Fatalf("Filter-First violation! Expected Task 3 and Task 4, got output:\n%s", out)
	}
	// Must NOT contain Task 1, Task 2, or Task 5
	if strings.Contains(out, "Task 1") || strings.Contains(out, "Task 2") || strings.Contains(out, "Task 5") {
		t.Errorf("Unexpected Complete tasks present in filtered output:\n%s", out)
	}
	// Verify row indices reflect original table positions 2 and 3
	if !strings.Contains(out, "| 2 |") || !strings.Contains(out, "| 3 |") {
		t.Errorf("Expected original row indices 2 and 3 in output:\n%s", out)
	}

	// -------------------------------------------------------------------------
	// Case 2: --where "status == Pending" --index 0
	// If Position-First: physical row 0 is "Complete", so filtering returns 0 rows.
	// If Filter-First: index 0 of Pending set targets physical row 2 ("Task 3").
	// -------------------------------------------------------------------------
	listCmd = silenceCmd(cmd.NewListCmd())
	listBuf.Reset()
	listCmd.SetOut(&listBuf)
	listCmd.SetArgs([]string{dbPath, "--where", "status==Pending", "--index", "0"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list index 0 failed: %v", err)
	}
	out = listBuf.String()
	if !strings.Contains(out, "Task 3") || strings.Contains(out, "Task 1") || strings.Contains(out, "Task 4") {
		t.Fatalf("Filter-First index 0 failed! Expected only Task 3, got:\n%s", out)
	}

	// -------------------------------------------------------------------------
	// Case 3: Count with --where "status == Pending" --head 2
	// If Position-First: count would be 0.
	// If Filter-First: count must be 2.
	// -------------------------------------------------------------------------
	countCmd := silenceCmd(cmd.NewCountCmd())
	var countBuf bytes.Buffer
	countCmd.SetOut(&countBuf)
	countCmd.SetArgs([]string{dbPath, "--where", "status==Pending", "--head", "2"})
	if err := countCmd.Execute(); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if countStr := strings.TrimSpace(countBuf.String()); countStr != "2" {
		t.Fatalf("Filter-First count failed! Expected 2, got %s (would be 0 under incorrect position-first order)", countStr)
	}
}

func TestListCmd_IncludeMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "meta_test.md")

	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,name"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	appCmd := silenceCmd(cmd.NewAppendCmd())
	appCmd.SetArgs([]string{dbPath, `{"id":"1", "name":"Alice"}`})
	if err := appCmd.Execute(); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	// 1. Test JSON format with --include-metadata
	listCmd := silenceCmd(cmd.NewListCmd())
	var jsonBuf bytes.Buffer
	listCmd.SetOut(&jsonBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "json", "--include-metadata"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list json with metadata failed: %v", err)
	}
	jsonOut := jsonBuf.String()
	if !strings.Contains(jsonOut, `"etag":`) || !strings.Contains(jsonOut, `"total_rows": 1`) || !strings.Contains(jsonOut, `"status": "success"`) {
		t.Errorf("expected json metadata envelope with etag, got: %s", jsonOut)
	}

	// 2. Test Markdown format with --include-metadata
	listCmd = silenceCmd(cmd.NewListCmd())
	var mdBuf bytes.Buffer
	listCmd.SetOut(&mdBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "md", "--include-metadata"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list md with metadata failed: %v", err)
	}
	mdOut := mdBuf.String()
	if !strings.Contains(mdOut, "ETag:") || !strings.Contains(mdOut, "Rows: 1") {
		t.Errorf("expected md metadata header, got: %s", mdOut)
	}

	// 3. Test CSV format with --include-metadata
	listCmd = silenceCmd(cmd.NewListCmd())
	var csvBuf bytes.Buffer
	listCmd.SetOut(&csvBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "csv", "--include-metadata"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list csv with metadata failed: %v", err)
	}
	csvOut := csvBuf.String()
	if !strings.Contains(csvOut, "# etag:") || !strings.Contains(csvOut, "total_rows: 1") {
		t.Errorf("expected csv comment header with etag, got: %s", csvOut)
	}
}

func TestAppendCmd_IfMatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "append_occ.md")

	initCmd := silenceCmd(cmd.NewInitCmd())
	initCmd.SetArgs([]string{dbPath, "--columns", "id,name"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Read structure to get initial ETag
	structCmd := silenceCmd(cmd.NewStructureCmd())
	var structBuf bytes.Buffer
	structCmd.SetOut(&structBuf)
	structCmd.SetArgs([]string{dbPath})
	if err := structCmd.Execute(); err != nil {
		t.Fatalf("structure failed: %v", err)
	}
	structOut := structBuf.String()
	var initialETag string
	for _, line := range strings.Split(structOut, "\n") {
		if strings.HasPrefix(line, "ETag: ") {
			initialETag = strings.TrimSpace(strings.TrimPrefix(line, "ETag: "))
			break
		}
	}
	if initialETag == "" {
		t.Fatalf("failed to extract ETag from structure output:\n%s", structOut)
	}

	// 1. Append with matching --if-match
	appCmd := silenceCmd(cmd.NewAppendCmd())
	appCmd.SetArgs([]string{dbPath, `{"id":"1", "name":"Alice"}`, "--if-match", initialETag})
	if err := appCmd.Execute(); err != nil {
		t.Fatalf("append with valid --if-match failed: %v", err)
	}

	// 2. Attempt append with stale ETag (should fail)
	appCmd2 := silenceCmd(cmd.NewAppendCmd())
	appCmd2.SetArgs([]string{dbPath, `{"id":"2", "name":"Bob"}`, "--if-match", initialETag})
	err := appCmd2.Execute()
	if err == nil {
		t.Fatalf("expected append with stale --if-match to fail, but it succeeded")
	}
	if !strings.Contains(err.Error(), "precondition failed") {
		t.Errorf("expected precondition failed error message, got: %v", err)
	}

	// 3. Verify append with --if-checksum alias
	structBuf.Reset()
	if err := structCmd.Execute(); err != nil {
		t.Fatalf("structure failed: %v", err)
	}
	var secondETag string
	for _, line := range strings.Split(structBuf.String(), "\n") {
		if strings.HasPrefix(line, "ETag: ") {
			secondETag = strings.TrimSpace(strings.TrimPrefix(line, "ETag: "))
			break
		}
	}
	appCmd3 := silenceCmd(cmd.NewAppendCmd())
	appCmd3.SetArgs([]string{dbPath, `{"id":"2", "name":"Bob"}`, "--if-checksum", secondETag})
	if err := appCmd3.Execute(); err != nil {
		t.Fatalf("append with --if-checksum failed: %v", err)
	}

	// 4. Verify append with --checksum alias
	structBuf.Reset()
	if err := structCmd.Execute(); err != nil {
		t.Fatalf("structure failed: %v", err)
	}
	var thirdETag string
	for _, line := range strings.Split(structBuf.String(), "\n") {
		if strings.HasPrefix(line, "ETag: ") {
			thirdETag = strings.TrimSpace(strings.TrimPrefix(line, "ETag: "))
			break
		}
	}
	appCmd4 := silenceCmd(cmd.NewAppendCmd())
	appCmd4.SetArgs([]string{dbPath, `{"id":"3", "name":"Charlie"}`, "--checksum", thirdETag})
	if err := appCmd4.Execute(); err != nil {
		t.Fatalf("append with --checksum failed: %v", err)
	}
}

func TestUpdateCmd(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tasks.md")

	content := `| id | title | status | priority |
| --- | --- | --- | --- |
| 1 | Task 1 | Pending | High |
| 2 | Task 2 | Pending | Medium |
| 3 | Task 3 | Completed | Low |
`
	if err := os.WriteFile(dbPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 1. Update using --where and --set
	updateCmd1 := silenceCmd(cmd.NewUpdateCmd())
	var out1 bytes.Buffer
	updateCmd1.SetOut(&out1)
	updateCmd1.SetArgs([]string{dbPath, "--where", "status == Pending", "--set", "status=Processing"})
	if err := updateCmd1.Execute(); err != nil {
		t.Fatalf("update with --where and --set failed: %v", err)
	}
	if !strings.Contains(out1.String(), "Successfully updated 2 row(s)") {
		t.Errorf("unexpected output: %s", out1.String())
	}

	// 2. Update single row using --index and --data
	updateCmd2 := silenceCmd(cmd.NewUpdateCmd())
	var out2 bytes.Buffer
	updateCmd2.SetOut(&out2)
	updateCmd2.SetArgs([]string{dbPath, "--index", "0", "--data", `{"priority":"Urgent"}`})
	if err := updateCmd2.Execute(); err != nil {
		t.Fatalf("update with --index and --data failed: %v", err)
	}
	if !strings.Contains(out2.String(), "Successfully updated 1 row(s)") {
		t.Errorf("unexpected output: %s", out2.String())
	}

	// 3. Verify final list content
	listCmd := silenceCmd(cmd.NewListCmd())
	var listBuf bytes.Buffer
	listCmd.SetOut(&listBuf)
	listCmd.SetArgs([]string{dbPath, "-o", "json"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	listJson := listBuf.String()
	if !strings.Contains(listJson, "Urgent") || !strings.Contains(listJson, "Processing") {
		t.Errorf("expected updated values in list json, got:\n%s", listJson)
	}
}

func TestUpdateCmd_OCC(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "items.md")
	content := "| id | status |\n| --- | --- |\n| 1 | Open |\n"
	if err := os.WriteFile(dbPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	structCmd := silenceCmd(cmd.NewStructureCmd())
	var structBuf bytes.Buffer
	structCmd.SetOut(&structBuf)
	structCmd.SetArgs([]string{dbPath})
	if err := structCmd.Execute(); err != nil {
		t.Fatalf("structure failed: %v", err)
	}
	var etag string
	for _, line := range strings.Split(structBuf.String(), "\n") {
		if strings.HasPrefix(line, "ETag: ") {
			etag = strings.TrimSpace(strings.TrimPrefix(line, "ETag: "))
			break
		}
	}
	if etag == "" {
		t.Fatalf("failed to obtain ETag")
	}

	// 1. Success with valid ETag
	upCmd1 := silenceCmd(cmd.NewUpdateCmd())
	upCmd1.SetArgs([]string{dbPath, "--where", "id == 1", "--set", "status=Active", "--if-match", etag})
	if err := upCmd1.Execute(); err != nil {
		t.Fatalf("update with matching ETag failed: %v", err)
	}

	// 2. Mismatch with stale ETag
	upCmd2 := silenceCmd(cmd.NewUpdateCmd())
	upCmd2.SetArgs([]string{dbPath, "--where", "id == 1", "--set", "status=Closed", "--if-match", etag})
	err := upCmd2.Execute()
	if err == nil {
		t.Fatalf("expected update with stale ETag to fail, but it succeeded")
	}
	if !strings.Contains(err.Error(), "precondition failed") {
		t.Errorf("expected precondition failed error message, got: %v", err)
	}
}




