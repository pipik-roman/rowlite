# Rowlite

Rowlite is an open-source, lightweight CLI tool and MCP server that works with Markdown tables - create, read, append (update, delete coming soon!).
Instead of writing SQL queries or ad-hoc scripts, developers and AI agents interact with tabular data using simple, declarative actions (`list`, `append`, `update`, `delete`, `dedupe`) directly over columns.

## Why I created rowlite

One day, I opened a precious Markdown table that was being managed by one of my LLM agents, and was quite surprised.
There should be ~300 entries, but there were around 20.

Because the agent was relying on fragile, ad-hoc scripts to update the file, there was difference in table column count between rows. Some rows had five columns, others had six, and the parsing/writing logic simply ate my data without warning.

I realized few things in that moment:

 - **Agents shouldn't write raw Markdown tables.** Letting an AI manually format pipe-delimited tables (even with prepared scripts) will backfire sooner or later. They need a strict, safe and predictable API. Also, it is a waste of precious tokens.
 - **The storage format shouldn't change.** I didn't want to migrate to SQLite or JSON and lose the beautiful, human-readable simplicity of a `.md` file.

I needed a tool that could guarantee the structural integrity of a standard Markdown table, but act like a database engine under the hood. It had to be simple, bulletproof, and bilingual—equally comfortable being queried by a human in a terminal or manipulated by an AI agent over an MCP connection.

And that is how rowlite was born.

*PS: The table was not in a Git repository, but I was able to recover it in the end.*

## The rowlite Philosophy
- **Schema Guardrails**: No more uneven rows or missing cells. If a row doesn't match the table definition, rowlite corrects it or indicate error.
- **100% Standard Markdown**: Your data remains completely open, human-readable, and perfectly rendered on GitHub or Obsidian.
- **Double-Agent Design**: Fast, native CLI commands for your terminal, and an out-of-the-box MCP server for your AI agents.

## Features
- **Zero-config by default**: Works with standard markdown table, no configuration needed. If you need simple CRUD, just use it.
- **Frontmatter-customization**: Advanced features can be triggered through frontmatter fields (`unique`, `sort`, `audit`, `journal_log`, `autoincrement`).
- **Auto-increment columns**: Automatically generate sequential integer IDs (e.g. `id:10` or `id:1`) for omitted cells on append.
- **In-place updates**: Safely edits Markdown GFM tables without disturbing surrounding headings, notes, or descriptions in the document.
- **Sorting and deduplication**: Declare primary columns and sorting columns for simple constraints and ordering.
- **Append**: Simple operation to append new row at the end, ideal for logging tables, preventing find-and-replace issues and inefficiency AI agents tend to have.
- **Auditing-info**: Automatic `created_at` / `updated_at` timestamps on rowlite operations.
- **Model Context Protocol native**: Fully compatible with the MCP specification for seamless integration with AI agent loops (Cursor, Claude Code, Antigravity, etc.) with thread-safe `TableConfig` caching.
- **4-Column Journal log**: Audit table change events to append-only `.log` files (`| timestamp | operation | row_key | details |`).

## Quickstart

### 1. Installation
Choose the installation method best suited for your environment:

#### Method 1: Package Managers (Recommended)
##### macOS & Linux (Homebrew)
```bash
brew tap pipik-roman/tap
brew install rowlite
```

##### Windows (Scoop)
```cmd
scoop bucket add rowlite https://github.com/pipik-roman/scoop-bucket.git
scoop install rowlite
```

#### Method 2: One-Line Install Scripts
##### Linux & macOS (POSIX Shell)
Quickly install the latest binary directly to `/usr/local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/pipik-roman/rowlite/main/install.sh | sh
```

##### Windows (PowerShell)
Installs automatically to `%LOCALAPPDATA%\Programs\rowlite` and configures your User `PATH`:

```powershell
irm https://raw.githubusercontent.com/pipik-roman/rowlite/main/install.ps1 | iex
```

#### Method 3: Pre-compiled Binaries
1. Go to the [Releases Page](https://github.com/pipik-roman/rowlite/releases/latest).
2. Download the appropriate archive for your operating system and CPU architecture:
   - **Windows**: `rowlite_<version>_windows_amd64.zip` (or `arm64`)
   - **macOS**: `rowlite_<version>_darwin_arm64.tar.gz` (Apple Silicon) or `darwin_amd64.tar.gz` (Intel)
   - **Linux**: `rowlite_<version>_linux_amd64.tar.gz` (or `arm64`)
3. Extract the archive and move the `rowlite` executable into a folder included in your system's `PATH`.

#### Method 4: From Source (For Go Developers)
If you have Go installed on your machine (1.23+):

```bash
go install github.com/pipik-roman/rowlite/cmd/rowlite@latest
```

Ensure your Go bin directory (typically `~/go/bin` on Unix or `%USERPROFILE%\go\bin` on Windows) is in your system `PATH`.

### 2. Automatic MCP Setup
Rowlite includes an interactive setup driver that automatically configures host AI tools (Cursor, VS Code, Copilot CLI, Claude Desktop, Antigravity, Zed, Cline, Crush, etc.):

```bash
# Run the interactive setup wizard
rowlite mcp-setup

# Or target a specific client globally
rowlite mcp-setup --client cursor
rowlite mcp-setup --client copilot
```

> [!TIP]
> **Need manual JSON configurations or workspace-level setups?**
> Check out the [Complete MCP Configuration Guide](docs/mcp-setup.md) for manual JSON config blocks and setup flags.

## CLI Commands
- `rowlite help` - See CLI help for details
- `rowlite init <file> [--columns "id,name"] [--unique "id"] [--sort "id:asc"] [--autoincrement "id:10"] [--journal-log "journal.log"]` - Create new markdown table file or update frontmatter constraints in-place. 
- `rowlite structure <file>` - Display column names, constraints, and audit log mappings.
- `rowlite list <file> [--where "..."] [--head N] [--tail N] [--index "..."] [--offset O] [--limit L] [--format json|csv|tsv|md|pretty] [--include-row-number]` - Query, filter, and slice rows.
- `rowlite count <file> [--where "..."] [--head N] [--tail N] [--index "..."] [--offset O] [--limit L]` - Count queried, filtered, or sliced rows.
- `rowlite append <file> --row '{"col1": "val1"}'` - Append a row, applying validations. Use JSON or CSV.
- `rowlite dedupe <file> --column <name>` - Purge duplicate records in place.
- `rowlite sort <file> --by "column:order,..."` - Sort the table. Useful for one-time sort or trigger the sorting from now-on.

### Querying & Slicing Tables
Both `rowlite list` and `rowlite count` share a unified, two-stage query evaluation pipeline:

$$\text{Table Content} \xrightarrow{\text{1. Content Filtering (--where)}} \text{Matched Records} \xrightarrow{\text{2. Positional Slicing (--head, --tail, --index, pagination)}} \text{Target Rows}$$

When combined, positional parameters (`--head`, `--tail`, `--index`, `--offset`, `--limit`) slice directly across the records that match your `--where` filters.

#### 1. Content Filtering (`--where`)
Filter rows based on column values. Column names are resolved using a **Case-Insensitive & Normalized Lookup** (e.g. `ITEM name`, `item_name`, or `item-name` automatically match the `Item Name` header).

* **Supported Operators**:
  * `==` : Exact string equality.
  * `!=` : Inequality.
  * `like` : Case-insensitive substring match.
  * `regex` : Go regular expression match.

#### 2. Positional Selection & Slicing
Target rows by their logical positions or apply pagination:

* `--head <N>`, `-H <N>` : Extract first $N$ rows (from start of matching set).
* `--tail <N>`, `-T <N>` : Extract last $N$ rows (from end of matching set).
* `--index <spec>`, `-i <spec>` : Target specific row index, list (`"0,2,4"`), range (`"2..10"` / `"2:10"`), or stepped slice (`"0..20..2"` / `"0:20:2"`). Supports negative indices (e.g. `"-1"` targets the last row).
* `--offset <O>` & `--limit <L>` : Zero-based sliding pagination window.
* `--include-row-number` : Inject `#` / `__row_index__` into output for stable row references.

#### Combined Query Examples
```powershell
# 1. Content filter combined with positional limits: get top 5 critical tasks
rowlite list tasks.md --where "Priority == High" --head 5

# 2. Substring filter with pagination: get second page of active items (10 per page)
rowlite list tasks.md --where "status like active" --offset 10 --limit 10

# 3. Exact filter with row numbering: inspect last 3 completed tasks
rowlite list tasks.md --where "status == done" --tail 3 --include-row-number

# 4. Count matching rows within a positional slice
rowlite count tasks.md --where "status == active" --head 10

# 5. Stepped slice across whole table (even-numbered rows)
rowlite list tasks.md --index "0..50..2"
```

## ⚡ Performance

Built in Go for speed, low memory overhead, and zero external dependencies. `rowlite` executes operations on tens of thousands of rows in milliseconds:

| Dataset | Operation | End-to-End Time (Disk $\rightarrow$ Result) | In-Memory / Cached |
| :--- | :--- | :--- | :--- |
| **10,000 rows** (1.25 MB) | Exact Match Query (`--where`) | **~13.0 ms** | **~0.19 ms** |
| **10,000 rows** (1.25 MB) | Positional Slicing (`--index`) | **~17.7 ms** | **~3.65 µs** |
| **10,000 rows** (1.25 MB) | Row Append (Auto-Increment + Audit) | **~15.2 ms** | — |
| **10,000 rows** (1.25 MB) | Fast-Path Count (zero filters) | **~14.3 ms** | — |
| **10,000 rows** (1.25 MB) | Unicode Substring Search (`like`) | **~16.5 ms** | **~2.46 ms** |
| **10,000 rows** (1.25 MB) | Full Table In-Place Sort | **~32.0 ms** | **~20.4 ms** |

> For complete end-to-end and in-memory benchmark metrics, test environment specifications, and reproduction steps, see [BENCHMARKS.md](BENCHMARKS.md).

## Roadmap
Currently (2026-08-01) rowlite supports only bare minimum - append operation, simple query, count. Ideally markdown with table only.

Other features/improvements coming soon:
- auto increment column (next release)
- update row/cell (next release)
- journalling changes (next release)
- MCP table metadata caching (next release)
- positional selection / filtering by row index (next release)
- delete row (future release)
- releases to MCP directories (future release)
- insert (future release)
- views (auto-updated filtered copy of original table) (future release)

Other planned improvements:



## Tech Stack
- Built in **Go** (for performance).
- CLI framework: [Cobra](https://github.com/spf13/cobra)
- Markdown GFM parsing: [Goldmark](https://github.com/yuin/goldmark)
- Frontmatter parsing: [frontmatter](https://github.com/adrg/frontmatter)
- MCP communication: [official Go SDK](https://github.com/modelcontextprotocol/go-sdk)

## License
MIT License. See [LICENSE](LICENSE) for details.
