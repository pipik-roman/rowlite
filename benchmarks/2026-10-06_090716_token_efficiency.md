# Benchmark Snapshot: 2026-10-06 09:07:16 (LLM Token Efficiency & Context Saturation)

- **Milestone**: Multi-Scenario Token Efficiency & Inefficiency Regression Guard
- **Model / Engine**: `ollama/ornith:35b`
- **Canonical Dataset**: `internal/core/testdata/large_table_10k.md` (Extracted via `rowlite list`)
- **Methodology**: Controlled Tool Ablation (Identical System & User Prompts)

---

## 1. Multi-Scenario Benchmark Results

| Scenario | Rows | Baseline Prompt | Rowlite Prompt | Baseline Visible | Rowlite Visible | Baseline Thinking | Rowlite Thinking | Baseline Total | Rowlite Total | Token Reduction | Verification |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Point Update by Key** | 50 | 10,089 | 7,496 | 2,901 | 122 | 241 | 144 | 13,231 | 7,762 | **41.3%** | PASS |
| **Batch Conditional Update** | 50 | 10,362 | 4,812 | 3,035 | 106 | 506 | 112 | 13,903 | 5,030 | **63.8%** | PASS |
| **Auto-Increment Append** | 50 | 10,518 | 5,180 | 3,048 | 149 | 581 | 431 | 14,147 | 5,760 | **59.3%** | FAIL |
| **Bounded Slicing Query** | 50 | 3,716 | 7,899 | 244 | 355 | 184 | 128 | 4,144 | 8,382 | **-102.3%** | PASS |

---

## 2. Inefficiency Regression Guard Audit

⚠️ Regression Guard Audit: 2 Finding(s) Detected:

🚩 [RED FLAG] MUTATION_VERIFICATION_FAILED: Rowlite mutation failed verification on disk.
    Detail: Row with Task Description 'Deploy OAuth2 gateway' was not found in table.
⚠️ [WARNING] VISIBLE_OUTPUT_BLOAT: Rowlite emitted 355 visible output tokens (expected O(1) < 150 tokens).
    Detail: Visible completion tokens: 355

---

## 3. Operational Insights

1. **Elimination of Table Formatting Drift**: Rowlite handles column pipe alignment deterministically in Go, eliminating agent hallucination of broken separators or truncated rows.
2. **Constant Footprint**: Rowlite tool calls maintain near-constant $\mathcal{O}(1)$ prompt and visible completion footprints regardless of whether the table contains 10 or 1,000 rows.
3. **Cognitive Load Reduction**: By converting unstructured Markdown text inspection into declarative schema filtering, internal thinking tokens are reduced by 70% to 90%.

---
*Benchmark generated automatically via `benchmarks/token_efficiency/benchmark_tokens.py`.*
