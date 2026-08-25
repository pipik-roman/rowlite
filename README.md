# Rowlite

Rowlite is an open-source, lightweight CLI tool and MCP server that works with Markdown tables - create, read, append, update, and delete.
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
- `rowlite structure <file>` - Display column names, constraints, audit log mappings, and current ETag checksum.
- `rowlite list <file> [--where "..."] [--head N] [--tail N] [--index "..."] [--offset O] [--limit L] [--format json|csv|tsv|md|pretty] [--include-row-number] [--include-metadata]` - Query, filter, and slice rows.
- `rowlite count <file> [--where "..."] [--head N] [--tail N] [--index "..."] [--offset O] [--limit L]` - Count queried, filtered, or sliced rows.
- `rowlite append <file> --row '{"col1": "val1"}' [--if-match "<etag>" | --if-checksum "<etag>" | --checksum "<etag>"]` - Append a row, applying validations and optional OCC precondition. Use JSON or CSV.
- `rowlite update <file> [--where "..."] [--set "col=val"] [--data '{"col":"val"}'] [--index "..."] [--head N] [--tail N] [--all] [--dry-run] [--if-match "<etag>"]` - Update matching rows in-place (supports pre-state isolation, auto-audit stamping, OCC, and `--all` conditionless safety).
- `rowlite delete <file> [--where "..."] [--head N] [--tail N] [--index "..."] [--offset O] [--limit L] [--all] [--dry-run] [--if-match "<etag>"]` - Delete matching rows from table (supports WHERE filters, positional slicing, change journaling, dry-run, OCC, and `--all` conditionless safety).
- `rowlite dedupe <file> --column <name>` - Purge duplicate records in place.
- `rowlite sort <file> --by "column:order,..."` - Sort the table. Useful for one-time sort or trigger the sorting from now-on.

### Querying & Slicing Tables
You can filter table rows by content using `--where`, or select rows by position using `--head`, `--tail`, `--index`, and pagination flags (`--offset`, `--limit`).

When combined, positional flags slice directly within the filtered results (e.g. `--where "status == active" --head 5` returns the first 5 active tasks).

#### 1. Content Filtering (`--where`)
Filter rows based on column values. Column names are matched flexibly (e.g. `ITEM name`, `item_name`, or `item-name` all match the `Item Name` header).

* **Supported Operators**:
  * `==` : Exact match.
  * `!=` : Not equal.
  * `like` : Case-insensitive text search.
  * `regex` : Regular expression match.

#### 2. Selecting Rows by Position (0-Based Indexing)
Target rows by line position or paginate through large datasets using **0-based indexing** (matching standard programming language array slicing like `table[0]` and Python `.iloc`):

* `--head <N>`, `-H <N>` : Get the first $N$ rows ($0 \dots N-1$).
* `--tail <N>`, `-T <N>` : Get the last $N$ rows.
* `--index <spec>`, `-i <spec>` : Target specific 0-based rows (`0` for first row), lists (`"0,2,4"`), ranges (`"2..10"`), or stepped slices (`"0..20..2"`). Negative numbers count from the end (e.g. `"-1"` is the last row).
* `--offset <O>` & `--limit <L>` : Skip $O$ rows (0-based) and take up to $L$ rows.
* `--include-row-number` : Include 0-based row numbers (`#` column or `__row_index__` in JSON) in the output.
* `--include-metadata`, `-m` : Include table ETag checksum and row totals in output.

> [!NOTE]
> **0-Based Slicing vs. 1-Based Auto-Increment IDs**:
> Positional parameters (`--index`, `--offset`, `--head`, `--tail`, `#`) are strictly **0-based** to align with programming memory offsets and array operations.
> In contrast, auto-increment surrogate keys (`autoincrement: "id:1"`) are **1-based** ($1, 2, 3, \dots$) to adhere to relational database primary key standards (like SQLite ROWID and MySQL AUTO_INCREMENT) and provide immutable entity identity across row mutations.
> 
> When rows are deleted, auto-increment surrogate keys continue monotonically calculating against the maximum remaining ID on subsequent appends—they do not re-number, decrement, or recycle IDs, ensuring stable entity references across deletions.

#### 3. Optimistic Concurrency Control (`--if-match` / `--if-checksum` / `--checksum`)
When multiple human users or AI agents interact with the same table, race conditions can cause lost updates or sequence conflicts. Rowlite computes a canonical SHA-256 ETag checksum over the table to guarantee state continuity:

```bash
# 1. Read table and obtain current ETag / Checksum
rowlite list tasks.md --format json --include-metadata
# Returns: "meta": { "etag": "a3f8c9b1", "total_rows": 42 }

# 2. Append safely with precondition guarantee (accepts --if-match, --if-checksum, or --checksum)
rowlite append tasks.md '{"task": "Deploy service"}' --if-match a3f8c9b1
# Or using the intuitive checksum alias:
rowlite append tasks.md '{"task": "Deploy service"}' --if-checksum a3f8c9b1
# If another process modified tasks.md in the meantime, the append safely aborts with PRECONDITION_FAILED
```

#### Examples
```powershell
# 1. Get the first 5 high-priority tasks
rowlite list tasks.md --where "Priority == High" --head 5

# 2. Get the second page of active items (10 per page)
rowlite list tasks.md --where "status like active" --offset 10 --limit 10

# 3. Inspect the last 3 completed tasks with row numbers
rowlite list tasks.md --where "status == done" --tail 3 --include-row-number

# 4. Count active tasks up to a limit of 10
rowlite count tasks.md --where "status == active" --head 10

# 5. Extract even-numbered rows across the table
rowlite list tasks.md --index "0..50..2"

# 6. Delete completed tasks safely with dry-run preview first
rowlite delete tasks.md --where "status == done" --dry-run
rowlite delete tasks.md --where "status == done"
```

## ⚡ Performance

Built in Go for speed, low memory overhead, and zero external dependencies. Benchmarked on a 10,000-row (1.25 MB) Markdown table:

| Operation | Execution Time |
| :--- | :--- |
| First Rows (`--head`) | **~2.6 ms** |
| Exact Match (`--where`) | **~13.0 ms** |
| Fast Count (no filters) | **~14.3 ms** |
| Add Row (Auto-Increment + Audit) | **~15.2 ms** |
| Text Search (`like`) | **~16.5 ms** |
| Row Slicing & Ranges (`--index`) | **~17.7 ms** |
| Sort Table | **~32.0 ms** |
| Streaming Update (`--head 10`) | **~44.8 ms** |
| Streaming Delete (`--head 10`) | **~41.2 ms** |
| In-Memory Row Mutation | **~362 ns** |

> For in-memory caching benchmarks, memory metrics, and reproduction steps, see [BENCHMARKS.md](BENCHMARKS.md).

## Roadmap
Currently rowlite supports table initialization, querying/filtering, slicing, appending, in-place updating, and row deletion.

Planned features for future releases:
- Insert row at index (`insert`)
- Views (auto-updated filtered copy of original table)
- Official package distributions and package registries

## Tech Stack
- Built in **Go** (for performance).
- CLI framework: [Cobra](https://github.com/spf13/cobra)
- Markdown GFM parsing: [Goldmark](https://github.com/yuin/goldmark)
- Frontmatter parsing: [frontmatter](https://github.com/adrg/frontmatter)
- MCP communication: [official Go SDK](https://github.com/modelcontextprotocol/go-sdk)

## License
MIT License. See [LICENSE](LICENSE) for details.
