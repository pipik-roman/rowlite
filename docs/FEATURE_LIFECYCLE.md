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

## 2. Dual Interface Exposure (CLI & MCP)

Every capability must be exposed consistently across both interaction models:

### CLI ([`internal/cmd`](../internal/cmd))
* Provide clear Cobra command metadata (`Use`, `Short`, `Long`, `Example`).
* Support standard output formats (`auto`, `json`, `csv`, `tsv`, `md`, `pretty`).
* Ensure JSON output uses raw characters (disable HTML entity escaping).

### MCP Server ([`internal/cmd/mcp.go`](../internal/cmd/mcp.go))
* Expose tools with strongly typed structs and comprehensive `jsonschema:"..."` field descriptions.
* Integrate with `TableConfigCache`: leverage cache hits on read operations and invalidate cache entries on mutating actions.
* Update [`server.json`](../server.json) (MCP Registry manifest) if tool definitions, arguments, or environment requirements change.

---

## 3. Testing Standards

* **Unit Tests (`*_test.go`)**: Test core logic in isolation with boundary conditions (empty tables, 1-row tables, malformed YAML, invalid filters).
* **Integration Tests (`cmd_test.go`)**: Validate CLI commands and MCP handlers end-to-end.
* **10,000-Row Fixture (`large_table_test.go`)**: Validate all new query/mutation behaviors against the static 10k fixture ([`large_table_10k.md`](../internal/core/testdata/large_table_10k.md)) containing escaped pipes (`\|`), backslashes (`\\`), JSON, Unicode accents, Asian scripts, and emojis.

---

## 4. Performance & Benchmarking

* **Isolated Benchmark Files (`*_bench_test.go`)**:
  - Place benchmarks strictly in `*_bench_test.go` files (e.g. [`large_table_bench_test.go`](../internal/core/large_table_bench_test.go)) so normal `go test ./...` test runs stay fast and clean.
* **Go 1.24+ `testing.B.Loop()` Mechanics**:
  - Use `for b.Loop() { ... }` for all benchmark loops.
  - For state-mutating benchmarks, use `b.StopTimer()` and `b.StartTimer()` around data reset logic.
* **Dual Perspectives**:
  - Benchmark both **End-to-End (Cold / Disk-to-Result)** and **In-Memory (Hot / Query Engine / MCP Cache)** performance.
* **Update [`BENCHMARKS.md`](../BENCHMARKS.md)**: Record updated execution times (`ns/op`), heap sizes (`B/op`), and allocation counts (`allocs/op`).

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

