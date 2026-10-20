# Rowlite Feature Lifecycle & Development Standards

This document establishes the development lifecycle and quality standards for contributing to and evolving `rowlite`. Whenever a feature is added, updated, or refactored, use this checklist to ensure complete coverage across core architecture, CLI/MCP interfaces, tests, benchmarks, and documentation.

---

## 1. Core Architecture & Logic Centralization

* **Single Source of Truth**: All data processing, table parsing, AST scanning, filter evaluation, sorting, auto-increment, and mutation logic must reside in [`internal/core`](../internal/core).
* **Index Preservation**: Query selection mechanisms must retain the 0-based positional index of matched rows (`MatchedRow.Index`) to facilitate accurate in-place mutation and deletion.
* **Integrity & Constraints**:
  - Always enforce YAML frontmatter directives (`unique`, `sort`, `autoincrement`, `audit`).
  - Write mutations atomically via temporary file swapping (`AtomicWriteTable`) to prevent table corruption.
  - Append mutation records to the 4-column change journal (`timestamp | operation | row_key | details`) when `journal_log` is configured.

---

## 2. Dual Interface Exposure: Asymmetric CLI vs. MCP Design

While core capabilities reside in `internal/core`, the CLI and MCP interfaces serve fundamentally different consumers and must be designed asymmetrically:

### CLI ([`internal/cmd`](../internal/cmd)) — Human & Shell Ergonomics
* Provide clear Cobra command metadata (`Use`, `Short`, `Long`, `Example`).
* Support terminal rendering formats (`auto`, `json`, `csv`, `tsv`, `md`, `pretty`, `expanded`) and safety flags (`--dry-run`, `--all`, shorthand aliases).
* Ensure JSON output uses raw characters (disable HTML entity escaping).

### MCP Server ([`internal/cmd/mcp.go`](../internal/cmd/mcp.go)) — AI-First Minimal Schema Principle
Every MCP tool schema and property description is injected into the LLM's context window on **every conversational turn**. To minimize the per-turn **Schema Tax**:
* **No 1:1 CLI Flag Dumping**: Never copy human CLI flags (`--dry-run`, `--all`, backward-compatibility flag aliases like `--if-checksum`, terminal display options, or redundant pagination flags on mutation/count tools) into MCP tool structs. Expose only the parameters an AI agent genuinely uses.
* **Shared Server Instructions over Per-Tool Duplication**: Place shared syntax conventions (such as `where` operators `==, !=, like, regex` and `sort` format `Col:asc|desc`) once in `mcp.ServerOptions.Instructions` rather than repeating them across every tool's description.
* **Zero-Tag Self-Describing Properties**: Omit `jsonschema:"..."` description tags on self-explanatory field names (`file`, `columns`, `unique`, `limit`, `offset`, `row`, `set`). Add brief `jsonschema` format hints only when syntax is non-obvious.
* **Cache Integration**: Leverage `TableConfigCache` hits on read operations and invalidate cache entries on mutating actions.
* **Registry Sync**: Update [`server.json`](../server.json) if MCP Registry metadata or transport requirements change.

---

## 3. Testing Standards

* **Unit Tests (`*_test.go`)**: Test core logic in isolation with boundary conditions (empty tables, 1-row tables, malformed YAML, invalid filters).
* **Integration Tests (`cmd_test.go`)**: Validate CLI commands and MCP handlers end-to-end.
* **10,000-Row Fixture (`large_table_test.go`)**: Validate all new query/mutation behaviors against the static 10k fixture ([`large_table_10k.md`](../internal/core/testdata/large_table_10k.md)) containing escaped pipes (`\|`), backslashes (`\\`), JSON, Unicode accents, Asian scripts, and emojis.

---

## 4. Performance, Benchmarking & Inefficiency Regression Checks

* **Isolated Benchmark Files (`*_bench_test.go`)**:
  - Place benchmarks strictly in `*_bench_test.go` files (e.g. [`large_table_bench_test.go`](../internal/core/large_table_bench_test.go), [`mcp_bench_test.go`](../internal/cmd/mcp_bench_test.go)) so normal `go test ./...` test runs stay fast and clean.
* **Go 1.24+ `testing.B.Loop()` Mechanics**:
  - Use `for b.Loop() { ... }` for all benchmark loops.
  - For state-mutating benchmarks, use `b.StopTimer()` and `b.StartTimer()` around data reset logic.
* **Dual Perspectives**:
  - Benchmark both **End-to-End (Cold / Disk-to-Result)** and **In-Memory (Hot / Query Engine / MCP Cache)** performance across the 10,000-row fixture.
* **MCP Schema Size & Token Efficiency Benchmarks**:
  - Whenever adding or modifying MCP tools in [`internal/cmd/mcp.go`](../internal/cmd/mcp.go), run both the Go schema size benchmark (`go test -bench=BenchmarkMCPSchemaSize ./internal/cmd`) and the Python token efficiency schema benchmark (`benchmarks/token_efficiency/benchmark_schema.py`) to compare total and per-tool token footprints (`cl100k_base`) against prior snapshots and prevent schema tax inflation.
* **Mandatory Timestamped Snapshot Archiving (`benchmarks/`)**:
  - Whenever introducing a new feature, optimization, or core refactor, always execute benchmarks (`-count=3`) and save a timestamped snapshot: `benchmarks/YYYY-MM-DD_HHMMSS_<feature_name>.md`.
  - Update [`benchmarks/README.md`](../benchmarks/README.md) with the new entry and commit metadata.
* **Performance Delta Auditing & Inefficiency Jump Checks**:
  - Compare new benchmark numbers against prior snapshot baselines.
  - Calculate relative latency speedup/slowdown ($\Delta\text{ns/op}$) and memory allocation deltas ($\Delta\text{B/op}$, $\Delta\text{allocs/op}$).
  - **Inefficiency Guard**: Actively check for and reject unexpected performance regressions (e.g., unintended $O(N)$ allocations on $O(K)$ queries, memory leaks, unbuffered full-table scans, or >10% MCP schema token inflation).
* **Update Documentation & Governance**:
  - Record the latest verified figures in [`BENCHMARKS.md`](../BENCHMARKS.md).
  - Update the ⚡ Performance summary table in [`README.md`](../README.md).

---

## 5. Documentation & README Governance

* **[`README.md`](../README.md)**:
  - Keep the main README concise and focused on quickstart, installation, and core command usage.
  - Update the Features list and command reference table.
  - Update the **⚡ Performance** summary table if benchmark numbers shift.
  - **Roadmap Governance**: **Do not remove completed items from the Roadmap until right before release.** Keep progress transparent and visible.
* **Dedicated Guides in [`docs/`](.)**:
  - For extensive features, client setup instructions, or complex syntax (e.g. MCP integration, journaling), create or update deep-dive guides in `docs/` (e.g. [`docs/mcp-setup.md`](mcp-setup.md)).

---

## 6. GitHub Actions CI, Security & Release Pipelines

* **Vulnerability Scanning (`govulncheck`)**:
  - The CI workflow executes `govulncheck` on every push. Keep `go.mod` toolchain versions aligned with the latest patch release (`go-version: '1.26.x'` with `check-latest: true`) so standard library vulnerability checks pass.
* **Continuous Integration (`ci.yml`)**:
  - Ensure all unit and integration tests run cleanly on CI (`ubuntu-latest`).
* **Zero-Bloat Dependency Footprint**:
  - Rely on the Go standard library wherever possible. Avoid adding unnecessary external packages to `go.mod`.
* **Local Release & Cross-Compilation Test**:
  - Run a local release dry-run before pushing major features or tagging releases:
    ```bash
    # 1. Validate GoReleaser syntax and configuration
    goreleaser check

    # 2. Build local snapshot across all target platforms (Linux, macOS, Windows)
    goreleaser build --snapshot --clean
    ```
  - Verifies that cross-compilation targets (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`) compile cleanly without OS-specific breakage.
* **Release Configurations**:
  - Keep [`.goreleaser.yaml`](../.goreleaser.yaml) and MCP Registry workflows ([`.github/workflows/release-mcp-registry.yml`](../.github/workflows/release-mcp-registry.yml)) aligned with build matrix and manifest schema requirements.
* **Dual-Branch Governance (`develop` vs. `main`)**:
  - `main` always matches the latest public release tag (`v*.*.*`). The `README.md` and benchmarks on `main` represent public release state.
  - Active feature development, benchmarks, refactoring, and bugfixes land on `develop`.
  - Direct pushes to `main` are restricted by GitHub Rulesets. All promotions to `main` arrive via Pull Requests from `develop`.
* **Pre-Release Verification (`vX.Y.Z-rc.N`)**:
  - Tag candidate releases (e.g. `v0.4.0-rc.1`) from `develop` when validating complex feature suites or MCP schema modifications.
  - GoReleaser automatically marks these builds as Pre-release on GitHub, avoiding unexpected upgrades for `go install ...@latest` consumers.
* **Promotion & Release Checklist**:
  1. Complete feature verification and unit/benchmark tests on `develop`.
  2. Open and merge a Pull Request from `develop` into `main`.
  3. Ensure all CI checks (`test`, `govulncheck`, `Hermetic Integrity Guard`) pass.
  4. Tag the new version on `main`: `git tag -a v0.4.0 -m "Release v0.4.0 - ..."`
  5. Push the tag to trigger GoReleaser: `git push origin v0.4.0`.

