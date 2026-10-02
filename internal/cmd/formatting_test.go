package cmd_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pipik-roman/rowlite/internal/cmd"
)

// TestListCmd_PrettyWideTable_FormattingBounds verifies that
// rendering a wide narrative table with --format pretty enforces terminal
// viewport bounds (max line length <= 100 characters).
func TestListCmd_PrettyWideTable_FormattingBounds(t *testing.T) {
	dbPath := filepath.Join("testdata", "wide_ideas.table.md")

	listCmd := silenceCmd(cmd.NewListCmd())
	var prettyBuf bytes.Buffer
	listCmd.SetOut(&prettyBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "pretty", "--max-width", "100"})

	if err := listCmd.Execute(); err != nil {
		t.Fatalf("pretty format execution failed: %v", err)
	}

	output := prettyBuf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected non-empty pretty table output, got empty")
	}

	const expectedWidth = 100
	for idx, line := range lines {
		cleanLine := strings.TrimRight(line, "\r")
		lineWidth := utf8.RuneCountInString(cleanLine)
		if lineWidth > expectedWidth {
			t.Errorf("line %d exceeds maximum width %d: got %d characters\nLine: %s",
				idx, expectedWidth, lineWidth, cleanLine)
		}
	}
}

// TestListCmd_StaggeredFolding_ZeroTruncation verifies that narrative columns
// are soft-wrapped without truncating any words or sentences.
func TestListCmd_StaggeredFolding_ZeroTruncation(t *testing.T) {
	dbPath := filepath.Join("testdata", "wide_ideas.table.md")

	listCmd := silenceCmd(cmd.NewListCmd())
	var prettyBuf bytes.Buffer
	listCmd.SetOut(&prettyBuf)
	listCmd.SetArgs([]string{dbPath, "--format", "pretty", "--max-width", "100"})

	if err := listCmd.Execute(); err != nil {
		t.Fatalf("pretty format execution failed: %v", err)
	}

	output := prettyBuf.String()

	// Verify all key narrative words from Row 1 are intact
	expectedWords := []string{
		"High-throughput", "asynchronous", "message", "broker", "zero-copy",
		"memory", "ring", "buffers", "scatter-gather", "saturation",
	}
	for _, word := range expectedWords {
		if !strings.Contains(output, word) {
			t.Errorf("expected output to contain word %q (zero truncation), but was missing", word)
		}
	}

	// Verify column headers exist in the single top header block
	if !strings.Contains(output, "Id") || !strings.Contains(output, "Tags") || !strings.Contains(output, "Idea") {
		t.Errorf("expected top header line 1 with Id, Tags, Idea")
	}
	if !strings.Contains(output, "Resources") || !strings.Contains(output, "created_at") || !strings.Contains(output, "updated_at") {
		t.Errorf("expected top header line 2 with Resources, created_at, updated_at")
	}
}

// TestListCmd_NoWrap_Flag verifies that --nowrap preserves the unconstrained 1-line-per-record grid.
func TestListCmd_NoWrap_Flag(t *testing.T) {
	dbPath := filepath.Join("testdata", "wide_ideas.table.md")

	listCmd := silenceCmd(cmd.NewListCmd())
	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "--format", "pretty", "--nowrap", "--head", "1"})

	if err := listCmd.Execute(); err != nil {
		t.Fatalf("pretty nowrap failed: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected non-empty output")
	}

	// In nowrap mode, line 0 top border and data lines exceed 200 characters
	firstLine := strings.TrimRight(lines[0], "\r")
	if utf8.RuneCountInString(firstLine) < 200 {
		t.Errorf("expected nowrap line to be wide (>200 chars), got %d chars", utf8.RuneCountInString(firstLine))
	}
}

// TestListCmd_ExpandedCardView verifies that -x / --expanded outputs clean vertical key-value cards.
func TestListCmd_ExpandedCardView(t *testing.T) {
	dbPath := filepath.Join("testdata", "wide_ideas.table.md")

	listCmd := silenceCmd(cmd.NewListCmd())
	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetArgs([]string{dbPath, "-x", "--head", "2", "--max-width", "90"})

	if err := listCmd.Execute(); err != nil {
		t.Fatalf("expanded format failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "-[ RECORD 1 ]") || !strings.Contains(output, "-[ RECORD 2 ]") {
		t.Errorf("expected output to contain -[ RECORD 1 ] and -[ RECORD 2 ], got:\n%s", output)
	}
	if !strings.Contains(output, "Id         :") || !strings.Contains(output, "Idea       :") {
		t.Errorf("expected key-value label formatting, got:\n%s", output)
	}
}
