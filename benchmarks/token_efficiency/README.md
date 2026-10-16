# Rowlite Token Efficiency Benchmark Suite

Empirical tool ablation benchmark evaluating LLM token consumption, latency, conversational roundtrips, and context saturation when inspecting and mutating Markdown tables with and without Rowlite.

---

## 1. Methodology: Controlled Tool Ablation

In accordance with rigorous empirical benchmarking practices, **both experimental arms use identical prompts and identical initial disk state**:

*   **Identical System Prompt**: Neutral assistant directive instructing the model to fulfill the user request and confirm concisely.
*   **Identical User Prompts**: Fixed declarative user instructions for each scenario.
*   **Identical Environment**: Markdown table fixture extracted from the canonical 10,000-row dataset ([`internal/core/testdata/large_table_10k.md`](../../internal/core/testdata/large_table_10k.md)) initialized on disk at `table.md`.
*   **Ablated Variable**: The tools exposed to the agent across 3 experimental arms:
    *   **Arm 1 (Whole-File Baseline)**: Generic filesystem operations (`read_file`, `write_file`). The model must read the entire table into context and emit the full updated Markdown file to perform mutations.
    *   **Arm 2 (Targeted Grep & Replace Strong-Man Baseline)**: Surgical editing tools (`grep_search`, `replace_file_content`, fallback `read_file`, `write_file`). The model searches for specific line patterns and attempts targeted contiguous replacements.
    *   **Arm 3 (Rowlite Native MCP Server)**: Native stdio Model Context Protocol (MCP) server (`rowlite mcp`). The model calls targeted declarative tools (`rowlite_update`, `rowlite_append`, `rowlite_list`, `rowlite_get_structure`).

---

## 2. Definition of Performed Test Scenarios

The suite evaluates 4 core operational patterns against the canonical table fixture containing columns:
`| Task ID | Task Description | Category Name | Priority Level | Status State | Score Metric | Created At |`

### Scenario 1: Point Update by Key (`point_update`)
*   **Objective**: Update a single cell indexed by primary key without scanning or mutating extraneous rows.
*   **System Prompt**:
    ```text
    You are a helpful assistant operating in a workspace containing 'table.md'. Fulfill the user request accurately using the available tools. When finished, respond with a concise confirmation.
    ```
*   **User Prompt**:
    ```text
    In 'table.md', update the 'Status State' of Task ID 2 from 'pending' to 'active'.
    ```
*   **Tool Expectations**:
    *   *Baseline*: Calls `read_file("table.md")` (ingesting all rows), then calls `write_file("table.md", modified_content)` re-emitting the entire table.
    *   *Rowlite MCP*: Calls `rowlite_get_structure("table.md")`, then calls `rowlite_update(file="table.md", where=["Task ID == 2"], set={"Status State": "active"})`.
*   **Verification**: Verifies on disk that row `Task ID == 2` has `Status State == 'active'`, all other rows are unmodified, and Markdown pipe alignments remain intact.

---

### Scenario 2: Batch Conditional Update (`batch_update`)
*   **Objective**: Perform a multi-row predicate update across all records matching a filter condition.
*   **System Prompt**:
    ```text
    You are a helpful assistant operating in a workspace containing 'table.md'. Fulfill the user request accurately using the available tools. When finished, respond with a concise confirmation.
    ```
*   **User Prompt**:
    ```text
    In 'table.md', update all tasks where 'Priority Level' is 'high' by setting 'Status State' to 'in_progress'.
    ```
*   **Tool Expectations**:
    *   *Baseline*: Reads the entire table, scans each line in-context, and rewrites the entire file.
    *   *Rowlite MCP*: Calls `rowlite_get_structure`, then calls `rowlite_update(file="table.md", where=["Priority Level == high"], set={"Status State": "in_progress"})`.
*   **Verification**: Scans all rows on disk to ensure every row where `Priority Level == 'high'` has `Status State == 'in_progress'` and non-matching rows are preserved.

---

### Scenario 3: Auto-Increment Append (`append`)
*   **Objective**: Append a new record, validating surrogate primary key generation, audit timestamps, and schema conformity.
*   **System Prompt**:
    ```text
    You are a helpful assistant operating in a workspace containing 'table.md'. Fulfill the user request accurately using the available tools. When finished, respond with a concise confirmation.
    ```
*   **User Prompt**:
    ```text
    In 'table.md', append a new task with Description 'Deploy OAuth2 gateway' in Category 'DevOps & CI/CD' with Priority 'high', Status 'active', and Score '99'.
    ```
*   **Tool Expectations**:
    *   *Baseline*: Reads the whole file to find the last ID, formats a new pipe-delimited line, and rewrites the file.
    *   *Rowlite MCP*: Calls `rowlite_get_structure`, then calls `rowlite_append(file="table.md", row={"Task Description": "Deploy OAuth2 gateway", "Category Name": "DevOps & CI/CD", "Priority Level": "high", "Status State": "active", "Score Metric": "99"})`.
*   **Verification**: Confirms on disk that the new row exists, received a valid incremented primary key, and conforms to table formatting.

---

### Scenario 4: Bounded Slicing Query (`query`)
*   **Objective**: Query and extract a subset of records without context window saturation or table file mutation.
*   **System Prompt**:
    ```text
    You are a helpful assistant operating in a workspace containing 'table.md'. Fulfill the user request accurately using the available tools. When finished, respond with a concise confirmation.
    ```
*   **User Prompt**:
    ```text
    In 'table.md', find and list the first 3 tasks in the 'Security & Auth' category.
    ```
*   **Tool Expectations**:
    *   *Baseline*: Reads the entire table file into context, parses the rows in the model's working memory, and returns the answer.
    *   *Rowlite MCP*: Calls `rowlite_get_structure`, then calls `rowlite_list(file="table.md", where=["Category Name == Security & Auth"], limit=3)`, returning only the 3 matching rows.
*   **Verification**: Confirms that the final assistant response identifies the correct tasks and that `table.md` was not modified.

---

## 3. MCP Tool Schema Size Benchmark (`benchmark_schema.py`)

In addition to trajectory execution, the benchmark suite isolates the **MCP Schema Tax** — the upfront token cost incurred on every conversational turn just by exposing tool definitions to the LLM.

*   **Metric**: Evaluates property count, raw serialized JSON bytes, and exact token footprint using `tiktoken` (`cl100k_base`).
*   **Principle**: Enforces the **AI-First Minimal MCP Schema Principle** defined in [`docs/FEATURE_LIFECYCLE.md`](../../docs/FEATURE_LIFECYCLE.md):
    *   Eliminates human CLI flags (`--dry-run`, `--all`, `--if-checksum`, positional slice flags on mutations).
    *   Shared syntax instructions live once in `mcp.ServerOptions.Instructions`.
    *   Zero-tag self-describing properties (`file`, `columns`, `row`, `set`).
*   **Budget Enforcement**: Monitored in Go CI via [`internal/cmd/mcp_bench_test.go`](../../internal/cmd/mcp_bench_test.go) (`TestMCPSchemaSizeBudget`: $\le 22$ total properties, $\le 2,500$ serialized bytes).

---

## 4. Telemetry Vectors & Measurement

| Telemetry Vector | Description |
| :--- | :--- |
| **Conversational Roundtrips (Turns)** | Exact number of request/response exchanges between the model and tools. |
| **Turn 1 Prompt Tokens (Schema Tax)** | Initial context footprint including system prompt, user prompt, and tool definitions. |
| **Subsequent Context Ingestion** | Tokens accumulated across subsequent turns as tool outputs are appended to conversation history. |
| **Visible Output Tokens** | Output tokens consumed by tool calls, tool arguments, and user-facing text. |
| **Thinking / Reasoning Tokens** | Internal reasoning tokens emitted by thinking models (e.g. Ornith, Qwen, DeepSeek). |
| **Cognitive Ratio** | $\frac{\text{Reasoning Tokens}}{\text{Visible Output Tokens}}$ — measures whether model capacity is spent reasoning vs regurgitating raw rows. |
| **Wall-Clock Latency (ms)** | End-to-end execution time per turn and across the complete trajectory. |
| **Mutation Verification** | Automated disk assertion checking that file state matches expected values. |

---

## 5. How to Run

### A. Deterministic Simulation Mode (Zero API Keys / Offline CI)
Runs reproducible tool ablation simulations with live table disk writes, live stdio MCP server execution, and exact `tiktoken` counting:
```bash
# Run all 4 scenarios across 50 rows (automatically includes MCP schema size analysis)
python -m uv run --with tiktoken python benchmarks/token_efficiency/benchmark_tokens.py

# Run with turn-by-turn prompt & output trace
python -m uv run --with tiktoken python benchmarks/token_efficiency/benchmark_tokens.py --verbose-turns

# Generate timestamped release snapshot into benchmarks/
python -m uv run --with tiktoken python benchmarks/token_efficiency/benchmark_tokens.py --save-snapshot
```

### B. Live LLM Agent Loop (Local or Remote Ollama / LiteLLM)
Executes an interactive multi-turn agent loop against an Ollama instance or LiteLLM provider.
*(Empirically validated on an NVIDIA DGX Spark with ARM64 / Blackwell architecture running `ornith:35b`)*:

```bash
# Configure connection for local or remote Ollama daemon (defaults to http://localhost:11434)
# Windows PowerShell:
$env:OLLAMA_HOST = "http://<remote-ollama-host>:11434"
# Linux / macOS:
export OLLAMA_HOST="http://<remote-ollama-host>:11434"

# Run all scenarios against model, print turn traces, and save release snapshot
python -m uv run --with httpx,tiktoken python benchmarks/token_efficiency/benchmark_tokens.py --model ollama/ornith:35b --verbose-turns --save-snapshot
```

### C. Release-to-Release Regression Audit
Compare current execution telemetry against a previous release's exported JSON:
```bash
python -m uv run --with tiktoken python benchmarks/token_efficiency/benchmark_tokens.py --compare-with benchmarks/v1.0.0_results.json
```

---

## 6. Command Line Arguments

| Argument | Default | Description |
| :--- | :--- | :--- |
| `--model <string>` | `None` | Model identifier (e.g. `ollama/ornith:35b` or `qwen3.8-27b`). When omitted, runs deterministic simulation. |
| `--api-base <url>` | `None` | Base URL for OpenAI-compatible endpoint (e.g. `http://localhost:8000/v1` or remote SGLang / vLLM). |
| `--api-key <key>` | `None` | API key for model endpoint (or via `OPENAI_API_KEY` env var). |
| `--rows <int>` | `50` | Number of rows extracted from canonical dataset. |
| `--scenario <name>` | `all` | Scenario to run: `all`, `point_update`, `batch_update`, `append`, `query`. |
| `--save-snapshot` | `False` | Saves timestamped release benchmark report into `benchmarks/` and updates archive index. |
| `--compare-with <path>` | `None` | Path to prior release JSON metrics to trigger automated regression guard checks. |
| `--verbose-turns` | `False` | Prints full turn-by-turn prompt, output, thinking, tool calls, and latency trace. |
| `--rowlite-bin <path>` | `rowlite` | Path or command name for the `rowlite` binary. |
| `--output-json <path>` | `None` | Exports complete telemetry metrics and schema analysis to a JSON file. |
| `--output-md <path>` | `None` | Exports Markdown summary report to custom path. |
