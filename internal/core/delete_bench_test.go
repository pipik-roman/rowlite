package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

func BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_Head10(b *testing.B) {
	origPath := filepath.Join("testdata", "large_table_10k.md")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		b.Fatalf("failed to read 10k table: %v", err)
	}

	tmpDir := b.TempDir()
	testPath := filepath.Join(tmpDir, "bench_10k_delete.md")
	ctx := context.Background()
	h10 := 10
	posOpts := core.PositionalOptions{Head: &h10}

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(testPath, origBytes, 0644)
		b.StartTimer()

		count, err := core.DeleteWithOptions(ctx, testPath, []string{"Status State == in_progress"}, posOpts, "", false, false)
		if err != nil || count != 10 {
			b.Fatalf("DeleteWithOptions failed: %v, count: %d", err, count)
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_SingleIndex(b *testing.B) {
	origPath := filepath.Join("testdata", "large_table_10k.md")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		b.Fatalf("failed to read 10k table: %v", err)
	}

	tmpDir := b.TempDir()
	testPath := filepath.Join(tmpDir, "bench_10k_delete_single.md")
	ctx := context.Background()
	posOpts := core.PositionalOptions{Index: "5000"}

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(testPath, origBytes, 0644)
		b.StartTimer()

		count, err := core.DeleteWithOptions(ctx, testPath, nil, posOpts, "", false, false)
		if err != nil || count != 1 {
			b.Fatalf("DeleteWithOptions failed: %v, count: %d", err, count)
		}
	}
}
