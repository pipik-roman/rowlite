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
- **Zero-config by default**: Works with standard markdown table, no configuration needed. If you need to simple CRUD, just use it.
- **Frontmatter-customization**: Advanced features can be triggered through frontmatter fields.
- **In-place updates**: Safely edits Markdown GFM tables without disturbing surrounding headings, notes, or descriptions in the document.
- **Sorting and deduplication**: Declare primary columns and sorting columns for simple constrints and ordering.
- **Append**: Simple operation to append new row at the end, ideal for logging tables, preventing find-and-replace issues and inefficiency AI agents tend to have.
- **Auditing-info**: Let insert/update timestamps on rowlite
- **Model Context Protocol native**: Fully compatible with the MCP specification for seamless integration with AI agent loops (Cursor, Claude Code, etc.).
- **Journal log**: Want to feel safe? Rowlite can write journal entries to additional file

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
- `rowlite init <file>` - Create new markdown file with table or update current settings for existing file/table. 
- `rowlite structure <file>` - Display column names, constraints, and audit log mappings.
- `rowlite list <file> --where "<column> <op> <value>"` - Query and filter rows.
- `rowlite count <file> --where "<column> <op> <value>"` - Count queried or filtered rows.
- `rowlite append <file> --row '{"col1": "val1"}'` - Append a row, applying validations. Use JSON or CSV.
- `rowlite dedupe <file> --column <name>` - Purge duplicate records in place.
- `rowlite sort <file> --by "column:order,..."` - Sort the table. Usefull for one-time sort or trigger the sorting from now-on.

### Query Filtering (`--where`)
Both the `list` and `count` commands support filtering rows using the `--where` flag. Column names in `--where` clauses are resolved using a **Case-Insensitive & Normalized Lookup** (e.g. `ITEM name`, `item_name` or `item-name` match `item name` header automatically).

#### Supported Operators
- `==` : Exact match.
- `!=` : Inequality match.
- `like` : Case-insensitive substring match.
- `regex` : Go regular expression match.

#### Examples
```powershell
# Filter by exact priority
rowlite list tasks.md --where "Priority == High"

# Case-insensitive search in column (resolves tags column normalized and case-insensitively)
rowlite list tasks.md --where "tags like beach"

# Go regular expression matching
rowlite list tasks.md --where "Title regex ^[A-Z]"
```

## Roadmap
Currently (2026-08-01) rowlite supports only bare minimum - append operation, simple query, count. Ideally markdown with table only.

Other features coming soon:
- releases
- delete row
- edit row/cell
- insert
- further improvements

## Tech Stack
- Built in **Go** (for performance).
- CLI framework: [Cobra](https://github.com/spf13/cobra)
- Markdown GFM parsing: [Goldmark](https://github.com/yuin/goldmark)
- Frontmatter parsing: [frontmatter](https://github.com/adrg/frontmatter)
- MCP communication: [official Go SDK](https://github.com/modelcontextprotocol/go-sdk)

## License
MIT License. See [LICENSE](LICENSE) for details.
