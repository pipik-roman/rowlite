# Rowlite Historical Benchmark Archive

This directory stores timestamped benchmark snapshots executed across versions and architectural milestones. To prevent confusion, benchmarks are partitioned into three dedicated domains:

1. **[Core Engine Performance Benchmarks](#1-core-engine-performance-benchmarks-go-disk--in-memory)**: Low-level Go disk I/O, streaming pushdown, memory allocations, and hot-cache query throughput on 10,000-row datasets.
2. **[LLM Token Efficiency Benchmarks](#2-llm-token-efficiency--context-saturation-benchmarks)**: Multi-turn agent tool ablation evaluating conversational roundtrips, context window consumption, visible output reduction, and thinking tokens.
3. **[MCP Tool Schema Size Benchmarks](#3-mcp-tool-schema-size--token-budget-benchmarks)**: Property counts, serialized JSON schema bytes, and token footprints for MCP server tool definitions.

---

## 1. Core Engine Performance Benchmarks (Go Disk & In-Memory)

Measures latency profiles ($\text{ns/op}$), memory allocations ($\text{B/op}$, $\text{allocs/op}$), and throughput of Rowlite's Go core engine across the canonical 10,000-row fixture ([`internal/core/testdata/large_table_10k.md`](../internal/core/testdata/large_table_10k.md)).

| Timestamp | Milestone / State | Commit | Focus | Engine Performance Highlights |
| :--- | :--- | :--- | :--- | :--- |
| **2026-08-27 09:05:00** | [Table Row Deletion & Unified Streaming Engine](2026-08-27_090500_delete_and_unified_streaming.md) | `f43c105` | Streaming Deletion | StreamTransformTable engine, single-pass forward mutations (3x speedup), sequential `[]RowAction` model, and fail-fast validation. |
| **2026-08-20 16:27:00** | [Streaming Table Updates](2026-08-20_162700_streaming_update.md) | `Working` | In-Place Mutation | Stream-based in-place updates ($\mathcal{O}(1)$ RAM), pre-state isolation, auto-audit stamping, and OCC preconditions. |
| **2026-08-20 15:02:00** | [Optimistic Concurrency & Checksum](2026-08-20_150200_optimistic_concurrency_checksum.md) | `Working` | OCC Preconditions | Content-derived SHA-256 ETag checksums and `--if-match` / `--if-checksum` precondition validation. |
| **2026-08-18 10:58:00** | [Single-Pass Streaming Pushdown](2026-08-18_105800_streaming_pushdown.md) | `Working` | Read Pushdown | Early exit & ring buffer pushdown: 5.5x cold speedup, 99.8% alloc reduction, 81.5ns hot head. |
| **2026-08-18 10:42:00** | [Baseline (Pre-Pushdown)](2026-08-18_104200_baseline.md) | `962cff3` | Engine Baseline | Baseline performance across 10k dataset prior to streaming pushdown optimization. |

---

## 2. LLM Token Efficiency & Context Saturation Benchmarks

Empirical tool ablation evaluating real LLM and simulated agent interactions when mutating and querying Markdown tables. Compares **Baseline whole-file filesystem tools** (`read_file`, `write_file`) against **Rowlite native MCP tools** (`rowlite_update`, `rowlite_append`, `rowlite_list`).

For complete test definitions, scenario prompts, and CLI arguments, see the [Token Efficiency Benchmark Suite Guide](token_efficiency/README.md).

<!-- TOKEN_BENCHMARKS_TABLE -->
| Timestamp | Snapshot | Model / Engine | Scenarios | Highlights |
| :--- | :--- | :--- | :--- | :--- |
| **2026-10-16 11:56:34** | [LLM Token Efficiency Snapshot](2026-10-16_115634_token_efficiency.md) | `qwen3.8-27b` | 4 scenarios | Up to 79.1% vs whole-file, 76.1% vs grep-replace across 50 rows. |
| **2026-10-16 08:57:44** | [LLM Token Efficiency Snapshot](2026-10-16_085744_token_efficiency.md) | `qwen3.8-27b` | 4 scenarios | Up to 12.4% vs whole-file, 76.6% vs grep-replace across 50 rows. |
| **2026-10-16 00:53:10** | [LLM Token Efficiency Snapshot](2026-10-16_005310_token_efficiency.md) | `qwen3.8-27b` | 4 scenarios | Up to 12.4% vs whole-file, 76.6% vs grep-replace across 50 rows. |
| **2026-10-13 14:04:34** | [LLM Token Efficiency Snapshot](2026-10-13_140434_token_efficiency.md) | `qwen3.8-27b` | 4 scenarios (50 rows) | Verified on DGX Spark (SGLang). Up to 79.2% token savings, 94.6% visible output cut, 8.5x latency speedup. |
| **2026-10-11 20:02:03** | [LLM Token Efficiency Snapshot](2026-10-11_200203_token_efficiency.md) | `ollama/ornith:35b` | 4 scenarios (50 rows) | Verified on DGX. Up to 82.0% token savings, 96.7% visible output cut, 10.5x speedup. Full turn traces included. |
| **2026-10-06 09:07:16** | [LLM Token Efficiency Snapshot](2026-10-06_090716_token_efficiency.md) | `ollama/ornith:35b` | 4 scenarios (50 rows) | Pre-slimdown evaluation with legacy 54-property schema. |
| **2026-10-05 14:19:42** | [LLM Token Efficiency Snapshot](2026-10-05_141942_token_efficiency.md) | `Simulation (cl100k_base)` | 4 scenarios (50 rows) | Deterministic simulation baseline: up to 85.3% token savings. |

---

## 3. MCP Tool Schema Size & Token Budget Benchmarks

Tracks the schema footprint (property counts, raw serialized JSON bytes, and `cl100k_base` token tax) of all exposed tools in [`internal/cmd/mcp.go`](../internal/cmd/mcp.go) to enforce the **AI-First Minimal MCP Schema Principle** defined in [`docs/FEATURE_LIFECYCLE.md`](../docs/FEATURE_LIFECYCLE.md).

| Architecture / Milestone | Snapshot | Scope | Properties | Schema Bytes | Schema Tokens (`cl100k_base`) | Highlights / Status |
| :--- | :--- | :--- | :---: | :---: | :---: | :--- |
| **Rowlite MCP Server** | [Latest Snapshot](2026-10-10_123346_mcp_schema_size.md) | 8 Tools | **22** | **2,928 B** | **811 tokens** | Minimal AI-First schema; enforced via `TestMCPSchemaSizeBudget` ($\le 22$ props, $\le 2,500$ B). |
