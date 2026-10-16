# Benchmark Snapshot: 2026-10-05 14:19:42 (LLM Token Efficiency & Context Saturation)

- **Milestone**: Multi-Scenario Token Efficiency & Inefficiency Regression Guard
- **Model / Engine**: `Deterministic Simulation (tiktoken cl100k_base)`
- **Canonical Dataset**: `internal/core/testdata/large_table_10k.md` (Extracted via `rowlite list`)
- **Methodology**: Controlled Tool Ablation (Identical System & User Prompts)

---

## 1. Multi-Scenario Benchmark Results

| Scenario | Rows | Baseline Prompt | Rowlite Prompt | Baseline Visible | Rowlite Visible | Baseline Thinking | Rowlite Thinking | Baseline Total | Rowlite Total | Token Reduction | Verification |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Point Update by Key** | 100 | 14,697 | 4,545 | 5,379 | 63 | 468 | 75 | 20,544 | 4,683 | **77.2%** | PASS |
| **Batch Conditional Update** | 100 | 14,725 | 4,553 | 5,394 | 63 | 652 | 80 | 20,771 | 4,696 | **77.4%** | PASS |
| **Auto-Increment Append** | 100 | 14,785 | 4,601 | 5,417 | 86 | 448 | 70 | 20,650 | 4,757 | **77.0%** | PASS |
| **Bounded Slicing Query** | 100 | 4,940 | 4,699 | 61 | 57 | 353 | 65 | 5,354 | 4,821 | **10.0%** | PASS |

---

## 2. Inefficiency Regression Guard Audit

⚠️ Regression Guard Audit: 1 Finding(s) Detected:

🚩 [RED FLAG] TOKEN_SAVINGS_SUBPAR: Total token reduction on 100 rows is only 10.0% (expected >= 50%).
    Detail: Baseline: 5354, Rowlite: 4821

---

## 3. Operational Insights

1. **Elimination of Table Formatting Drift**: Rowlite handles column pipe alignment deterministically in Go, eliminating agent hallucination of broken separators or truncated rows.
2. **Constant Footprint**: Rowlite tool calls maintain near-constant $\mathcal{O}(1)$ prompt and visible completion footprints regardless of whether the table contains 10 or 1,000 rows.
3. **Cognitive Load Reduction**: By converting unstructured Markdown text inspection into declarative schema filtering, internal thinking tokens are reduced by 70% to 90%.

---
*Benchmark generated automatically via `benchmarks/token_efficiency/benchmark_tokens.py`.*
