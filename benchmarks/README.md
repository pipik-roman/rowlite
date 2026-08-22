# Rowlite Historical Benchmark Archive

This directory stores timestamped benchmark snapshots executed across versions and architectural milestones to track performance deltas, memory allocations, and latency profiles over large datasets.

## Naming Convention
Snapshots are named according to:
`YYYY-MM-DD_HHMMSS_<milestone_name>.md`

## Benchmark Snapshots Index

| Timestamp | Milestone / State | Commit | Highlights |
| :--- | :--- | :--- | :--- |
| **2026-08-18 10:42:00** | [Baseline (Pre-Pushdown)](2026-08-18_104200_baseline.md) | `962cff3` | Baseline performance across 10k dataset prior to streaming pushdown optimization. |
| **2026-08-18 10:58:00** | [Single-Pass Streaming Pushdown](2026-08-18_105800_streaming_pushdown.md) | `Working` | Early exit & ring buffer pushdown: 5.5x cold speedup, 99.8% alloc reduction, 81.5ns hot head. |
| **2026-08-20 15:02:00** | [Optimistic Concurrency & Checksum](2026-08-20_150200_optimistic_concurrency_checksum.md) | `Working` | Content-derived SHA-256 ETag checksums and `--if-match` / `--if-checksum` precondition validation. |
| **2026-08-20 16:27:00** | [Streaming Table Updates](2026-08-20_162700_streaming_update.md) | `Working` | Stream-based in-place updates ($\mathcal{O}(1)$ RAM), pre-state isolation, auto-audit stamping, and OCC preconditions. |


