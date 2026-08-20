package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

func TestComputeETag(t *testing.T) {
	data1 := []byte("| id | name |\n| --- | --- |\n| 1 | Alice |\n")
	data2 := []byte("| id | name |\n| --- | --- |\n| 1 | Alice |\n")
	data3 := []byte("| id | name |\n| --- | --- |\n| 2 | Bob |\n")

	etag1 := core.ComputeETag(data1)
	etag2 := core.ComputeETag(data2)
	etag3 := core.ComputeETag(data3)

	if len(etag1) != 16 {
		t.Errorf("expected 16 hex chars, got %d (%q)", len(etag1), etag1)
	}

	if etag1 != etag2 {
		t.Errorf("expected deterministic ETags for identical content, got %q and %q", etag1, etag2)
	}

	if etag1 == etag3 {
		t.Errorf("expected different ETags for different content, both got %q", etag1)
	}
}

func TestVerifyETag(t *testing.T) {
	// Empty expectedETag passes unconditionally (no precondition)
	if err := core.VerifyETag("", "a1b2c3d4e5f60718"); err != nil {
		t.Errorf("expected empty expectedETag to pass, got err: %v", err)
	}

	// Exact match passes
	if err := core.VerifyETag("a1b2c3d4e5f60718", "a1b2c3d4e5f60718"); err != nil {
		t.Errorf("expected exact match to pass, got err: %v", err)
	}

	// Case-insensitive match passes
	if err := core.VerifyETag("A1B2C3D4E5F60718", "a1b2c3d4e5f60718"); err != nil {
		t.Errorf("expected case-insensitive match to pass, got err: %v", err)
	}

	// Mismatch returns PreconditionFailedError
	err := core.VerifyETag("expected12345678", "current87654321")
	if err == nil {
		t.Fatalf("expected error on mismatch, got nil")
	}

	var precondErr *core.PreconditionFailedError
	if !errors.As(err, &precondErr) {
		t.Fatalf("expected PreconditionFailedError type, got %T: %v", err, err)
	}
	if precondErr.ExpectedETag != "expected12345678" || precondErr.CurrentETag != "current87654321" {
		t.Errorf("unexpected PreconditionFailedError fields: %+v", precondErr)
	}
}

func TestAppendWithOptions_OCCPrecondition(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "occ_test.md")
	content := "| id | name |\n| --- | --- |\n| 1 | Alice |\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	ctx := context.Background()

	// 1. Read initial table to obtain ETag
	cfg, _, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeNone)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}
	initialETag := cfg.ETag
	if initialETag == "" {
		t.Fatal("expected non-empty initial ETag")
	}

	// 2. Append with valid ifMatch
	newRow := map[string]string{"id": "2", "name": "Bob"}
	err = core.AppendRowWithOptions(ctx, filePath, newRow, initialETag)
	if err != nil {
		t.Fatalf("AppendRowWithOptions with valid ifMatch failed: %v", err)
	}

	// 3. Verify file now has 2 rows and new ETag
	cfgAfter, rowsAfter, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}
	if len(rowsAfter) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rowsAfter))
	}
	if cfgAfter.ETag == initialETag {
		t.Errorf("expected ETag to change after append, got %q", cfgAfter.ETag)
	}

	// 4. Attempt append with stale initialETag (should fail)
	staleRow := map[string]string{"id": "3", "name": "Charlie"}
	err = core.AppendRowWithOptions(ctx, filePath, staleRow, initialETag)
	if err == nil {
		t.Fatal("expected AppendRowWithOptions with stale ETag to fail, but it succeeded")
	}

	var precondErr *core.PreconditionFailedError
	if !errors.As(err, &precondErr) {
		t.Fatalf("expected PreconditionFailedError, got %T: %v", err, err)
	}
	if precondErr.ExpectedETag != initialETag || precondErr.CurrentETag != cfgAfter.ETag {
		t.Errorf("precondition error mismatch: expected %q, current %q; got %+v", initialETag, cfgAfter.ETag, precondErr)
	}

	// 5. Verify table content remained untouched on failed precondition
	_, rowsFinal, err := core.ParseFrontmatterAndContent(ctx, filePath, core.ContentModeAll)
	if err != nil {
		t.Fatalf("ParseFrontmatterAndContent failed: %v", err)
	}
	if len(rowsFinal) != 2 {
		t.Errorf("expected table row count to remain 2, got %d", len(rowsFinal))
	}
}
