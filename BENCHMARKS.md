# Rowlite Benchmarks

This document details the performance benchmarks of `rowlite` across end-to-end (disk-to-result) and in-memory operations on large Markdown tables, implemented using the Go 1.24+ `testing.B.Loop()` standard.

## Test Environment

* **OS**: Windows 11 (amd64)
* **CPU**: Intel(R) Core(TM) i7-8565U @ 1.80GHz (4 Cores, 8 Threads)
* **Go Version**: `go1.26.5 windows/amd64`
* **Benchmark Engine**: `testing.B.Loop()` (isolated in [`large_table_bench_test.go`](internal/core/large_table_bench_test.go))
* **Dataset Size**: **10,000 rows** (7 columns, ~1.25 MB Markdown GFM table)

## Dataset Characteristics

The 10,000-row static benchmark dataset ([`internal/core/testdata/large_table_10k.md`](internal/core/testdata/large_table_10k.md)) contains realistic, complex data:
* **Multi-word headers**: `Task ID`, `Task Description`, `Category Name`, `Priority Level`, `Status State`, `Score Metric`, `Created At`.
* **Escaped characters**: Escaped pipes (`Option A \| Option B`) and Windows paths (`C:\\Windows\\System32`).
* **Special characters & JSON**: Brackets `[P0-BLOCKER]` and JSON structures `{"retry": true, "limit": 5}`.
* **Unicode & Emojis**: Accented text (`Štúdio & Media`, `Zürich & München`, `Café`), Asian characters (`日本語 Cluster`), and emojis (`🚀`, `⚠️`, `★`, `🔥`).
* **Frontmatter constraints**: `unique: ["Task ID"]`, `sort: "Task ID:asc"`, `audit.insert: "Created At"`, `autoincrement: "Task ID"`.

---

## 1. End-to-End Benchmarks (Cold / Disk-to-Result)

These benchmarks measure the **entire end-to-end operation** from raw Markdown file on disk to completed action: **Disk I/O $\rightarrow$ YAML Frontmatter Parse $\rightarrow$ GFM Tokenize & Unescape $\rightarrow$ Query/Mutation $\rightarrow$ Persistence**.

| Operation | Dataset Size | Execution Time (`ns/op`) | Memory (`B/op`) | Allocations | Description |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Streaming Early Exit** (`--head 1`) | 10,000 rows (1.25 MB) | **~2.66 ms** (2,664,566 ns) | 4.42 MB | 218 allocs | Single-pass file scan with instant break upon match (99.8% alloc reduction) |
| **Exact Match Query** (`==`) | 10,000 rows (1.25 MB) | **~13.0 ms** (13,010,420 ns) | 10.68 MB | 110,803 allocs | Full file read, parse, and exact match filter |
| **Fast-Path Count** | 10,000 rows (1.25 MB) | **~14.3 ms** (14,361,470 ns) | 9.70 MB | 110,785 allocs | Fast-path count without loading row bodies |
| **Filtered Count** | 10,000 rows (1.25 MB) | **~14.4 ms** (14,421,382 ns) | 9.97 MB | 110,551 allocs | Streaming single-pass filtered count |
| **Streaming Tail** (`--tail 1`) | 10,000 rows (1.25 MB) | **~15.5 ms** (15,575,106 ns) | 9.68 MB | 110,535 allocs | Single-pass stream scan with $O(1)$ ring buffer |
| **Row Append** | 10,000 rows (1.25 MB) | **~15.2 ms** (15,207,642 ns) | 9.70 MB | 110,805 allocs | Uniqueness check, auto-increment generation, audit stamp & disk append |
| **Substring Search** (`like`) | 10,000 rows (1.25 MB) | **~16.5 ms** (16,543,174 ns) | 11.15 MB | 130,813 allocs | Full file read, parse, and case-insensitive Unicode search |
| **Positional Slice** (`--index`) | 10,000 rows (1.25 MB) | **~17.7 ms** (17,765,811 ns) | 12.15 MB | 110,831 allocs | Full file read, parse, and stepped slice evaluation (`0..9999..100`) |
| **Streaming Update** (`--head 10`) | 10,000 rows (1.25 MB) | **~44.8 ms** (44,878,272 ns) | 24.82 MB | 265,267 allocs | Stream read, mutate 10 rows, format, and atomic workfile write |
| **Single Index Update** (`--index 5000`) | 10,000 rows (1.25 MB) | **~49.5 ms** (49,514,783 ns) | 27.42 MB | 319,829 allocs | Stream read, mutate target row, format, and atomic workfile write |
| **Table In-Place Sort** | 10,000 rows (1.25 MB) | **~32.0 ms** (32,044,938 ns) | 10.68 MB | 110,807 allocs | Full file read, parse, and numeric/lexical sort |
| **Regex Match** | 10,000 rows (1.25 MB) | **~55.8 ms** (55,810,641 ns) | 49.69 MB | 340,872 allocs | Full file read, parse, and regular expression evaluation |

---

## 2. In-Memory Benchmarks (Hot / MCP Server Cache)

These benchmarks isolate the **query engine performance in memory** when the table is already cached in RAM (such as during repeated MCP tool calls via [`TableConfigCache`](internal/cmd/mcp.go)):

| Operation | Dataset Size | Execution Time (`ns/op`) | Memory (`B/op`) | Allocations | Description |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Streaming Early Exit** (`--head 1`) | 10,000 rows in RAM | **~81.5 ns** (81 ns) | 32 B | 1 alloc | In-memory stream scan with instant pushdown exit |
| **Row Mutation** (`ApplyRowUpdate`) | In-Memory (Hot) | **~362 ns** (362 ns) | 448 B | 3 allocs | Pure in-memory cell assignment and edit audit stamp |
| **Positional Slice** (`--index`) | 10,000 rows in RAM | **~3.65 µs** (3,654 ns) | 5.56 KB | 8 allocs | Pure in-memory stepped slice extraction (`0..9999..100`) |
| **Streaming Tail** (`--tail 1`) | 10,000 rows in RAM | **~0.15 ms** (157,949 ns) | 32 B | 1 alloc | In-memory stream scan with $O(1)$ ring buffer |
| **Exact Match Query** (`==`) | 10,000 rows in RAM | **~0.19 ms** (189,303 ns) | 32 B | 1 alloc | Pure in-memory row selection by ID |
| **Substring Search** (`like`) | 10,000 rows in RAM | **~2.46 ms** (2,460,302 ns) | 473 KB | 20,011 allocs | Pure in-memory case-insensitive Unicode search |
| **Table In-Place Sort** | 10,000 rows in RAM | **~20.4 ms** (20,434,261 ns) | 144 B | 5 allocs | In-memory numeric and lexical sorting with timer control |
| **Regex Match** | 10,000 rows in RAM | **~52.0 ms** (52,029,104 ns) | 38.99 MB | 230,068 allocs | In-memory regular expression evaluation |

---

## How to Reproduce

Run the automated Go benchmark suite:

```bash
# Run all large-table benchmarks with memory profiling excluding unit tests
go test -bench=BenchmarkLargeTable -benchmem -run=^$ ./internal/core/...
```

---

## 3. LLM Token Efficiency & Context Saturation Benchmarks

To measure LLM token consumption, schema tax, and context saturation when AI agents interact with Markdown tables via whole-file filesystem tools versus Rowlite MCP, see the [LLM Token Efficiency Benchmark Suite](benchmarks/token_efficiency/README.md) and the latest [Snapshot Report](benchmarks/token_efficiency/REPORT.md).
