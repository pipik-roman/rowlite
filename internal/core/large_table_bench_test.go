package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipik-roman/rowlite/internal/core"
)

// ---------------- Benchmarks ----------------

// --- End-to-End Benchmarks (Full Operation: Disk Read + Parse + Execute) ---

func BenchmarkLargeTable10k_EndToEnd_Select_ExactMatch(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	filters, _ := core.ParseFilters([]string{"Task ID == 5000"})

	for b.Loop() {
		cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil || len(matches) != 1 {
			b.Fatalf("SelectRows error: %v, matches: %d", err, len(matches))
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Select_Like_Unicode(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	filters, _ := core.ParseFilters([]string{"Category Name like Štúdio"})

	for b.Loop() {
		cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil || len(matches) == 0 {
			b.Fatalf("SelectRows error: %v, matches: %d", err, len(matches))
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Select_Regex(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	filters, _ := core.ParseFilters([]string{`Task Description regex ^Special Tag #\d+`})

	for b.Loop() {
		cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil || len(matches) == 0 {
			b.Fatalf("SelectRows error: %v, matches: %d", err, len(matches))
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Count_FastPath(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()

	for b.Loop() {
		count, err := core.Count(ctx, path, nil)
		if err != nil || count != 10000 {
			b.Fatalf("Count error: %v, count: %d", err, count)
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Count_Filtered(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	where := []string{"Priority Level == critical"}

	for b.Loop() {
		count, err := core.Count(ctx, path, where)
		if err != nil || count == 0 {
			b.Fatalf("Count error: %v, count: %d", err, count)
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Sort(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()

	for b.Loop() {
		cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
		if err != nil {
			b.Fatalf("failed: %v", err)
		}
		err = core.SortRows(cfg.Headers, rows, "Score Metric:desc")
		if err != nil {
			b.Fatalf("SortRows error: %v", err)
		}
	}
}

func BenchmarkLargeTable10k_EndToEnd_Append(b *testing.B) {
	fixturePath := filepath.Join("testdata", "large_table_10k.md")
	initialBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		b.Fatalf("failed to read fixture: %v", err)
	}
	ctx := context.Background()
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "append_bench.md")

	newRow := map[string]string{
		"Task Description": "Benchmarked Task with special chars 🚀",
		"Category Name":    "DevOps & CI/CD",
		"Priority Level":   "critical",
		"Status State":     "active",
		"Score Metric":     "999",
	}

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(tmpFile, initialBytes, 0644)
		b.StartTimer()

		err := core.AppendRow(ctx, tmpFile, newRow)
		if err != nil {
			b.Fatalf("AppendRow error: %v", err)
		}
	}
}

// --- In-Memory Benchmarks (Hot Cache / Query Engine Only) ---

func BenchmarkLargeTable10k_InMemory_Select_ExactMatch(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	filters, _ := core.ParseFilters([]string{"Task ID == 5000"})

	for b.Loop() {
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil || len(matches) != 1 {
			b.Fatalf("SelectRows error: %v, matches: %d", err, len(matches))
		}
	}
}

func BenchmarkLargeTable10k_InMemory_Select_Like_Unicode(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	filters, _ := core.ParseFilters([]string{"Category Name like Štúdio"})

	for b.Loop() {
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil || len(matches) == 0 {
			b.Fatalf("SelectRows error: %v, matches: %d", err, len(matches))
		}
	}
}

func BenchmarkLargeTable10k_InMemory_Select_Regex(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	cfg, rows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	filters, _ := core.ParseFilters([]string{`Task Description regex ^Special Tag #\d+`})

	for b.Loop() {
		matches, err := core.SelectRows(cfg.Headers, rows, filters)
		if err != nil || len(matches) == 0 {
			b.Fatalf("SelectRows error: %v, matches: %d", err, len(matches))
		}
	}
}

func BenchmarkLargeTable10k_InMemory_Sort(b *testing.B) {
	path := filepath.Join("testdata", "large_table_10k.md")
	ctx := context.Background()
	cfg, originalRows, err := core.ParseFrontmatterAndContent(ctx, path, core.ContentModeAll)
	if err != nil {
		b.Fatalf("failed: %v", err)
	}
	rowsCopy := make([][]string, len(originalRows))

	for b.Loop() {
		b.StopTimer()
		for i := range originalRows {
			rowsCopy[i] = append([]string(nil), originalRows[i]...)
		}
		b.StartTimer()

		err := core.SortRows(cfg.Headers, rowsCopy, "Score Metric:desc")
		if err != nil {
			b.Fatalf("SortRows error: %v", err)
		}
	}
}
