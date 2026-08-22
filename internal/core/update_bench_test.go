package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

func BenchmarkLargeTable10k_EndToEnd_Update_Streaming_Head10(b *testing.B) {
	origPath := filepath.Join("testdata", "large_table_10k.md")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		b.Fatalf("failed to read 10k table: %v", err)
	}

	tmpDir := b.TempDir()
	testPath := filepath.Join(tmpDir, "bench_10k_update.md")
	ctx := context.Background()
	h10 := 10
	posOpts := core.PositionalOptions{Head: &h10}
	updates := map[string]string{"Status State": "bench_updated"}

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(testPath, origBytes, 0644)
		b.StartTimer()

		count, err := core.UpdateWithOptions(ctx, testPath, []string{"Status State == in_progress"}, posOpts, updates, "")
		if err != nil || count != 10 {
			b.Fatalf("UpdateWithOptions failed: %v, count: %d", err, count)
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Update_Streaming_SingleIndex(b *testing.B) {
	origPath := filepath.Join("testdata", "large_table_10k.md")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		b.Fatalf("failed to read 10k table: %v", err)
	}

	tmpDir := b.TempDir()
	testPath := filepath.Join(tmpDir, "bench_10k_update_single.md")
	ctx := context.Background()
	posOpts := core.PositionalOptions{Index: "5000"}
	updates := map[string]string{"Priority Level": "Urgent"}

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(testPath, origBytes, 0644)
		b.StartTimer()

		count, err := core.UpdateWithOptions(ctx, testPath, nil, posOpts, updates, "")
		if err != nil || count != 1 {
			b.Fatalf("UpdateWithOptions failed: %v, count: %d", err, count)
		}
	}
}

func BenchmarkLargeTable10k_Hot_ApplyRowUpdate(b *testing.B) {
	headers := []string{"Task ID", "Task Description", "Category Name", "Priority Level", "Status State", "Score Metric", "Created At"}
	row := []string{"5000", "Sample Task #5000", "Backend", "Normal", "in_progress", "42.5", "2026-01-01 10:00:00"}
	updates := map[string]string{"Priority Level": "Urgent", "Status State": "completed"}
	cfg := core.TableConfig{
		Headers: headers,
		Audit: core.AuditConfig{
			Edit: "Modified At",
		},
	}
	ts := "2026-08-20 16:30:00"

	for b.Loop() {
		newRow, mutated, err := core.ApplyRowUpdate(headers, row, updates, cfg, ts)
		if err != nil || len(newRow) != len(headers) || len(mutated) == 0 {
			b.Fatalf("ApplyRowUpdate failed: %v", err)
		}
	}
}
