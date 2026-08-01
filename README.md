# Rowlite

Rowlite is a zero-config, non-SQL, lightweight command-line tool and Model Context Protocol (MCP) server that treats standard Markdown files as flat, schema-validated databases.

Instead of writing SQL queries, developers and AI agents interact with tabular data using simple, declarative actions (`list`, `append`, `update`, `delete`, `dedupe`) directly over columns.

## Why I created rowlite

One day, I opened a precious Markdown table that was being managed by one of my LLM agents, and my heart sank.

**Data was missing. The grid was broken.**

Because the agent was relying on fragile, ad-hoc scripts to update the file, a slight hiccup in the prompt logic caused a schema drift. Some rows had five columns, others had six, and the parsing logic simply ate my data without warning.

I realized few things in that moment:

 - **Agents shouldn't write raw Markdown tables.** Letting an AI manually format pipe-delimited tables (even with prepared scripts) will backfire sooner or later. They need a strict, predictable API. Also, it is a waste of precious tokens.
 - **The storage format shouldn't change.** I didn't want to migrate to SQLite or JSON and lose the beautiful, human-readable simplicity of a `.md` file.

I needed a tool that could guarantee the structural integrity of a standard Markdown table, but act like a database engine under the hood. It had to be simple, bulletproof, and bilingual—equally comfortable being queried by a human in a terminal or manipulated by an AI agent over an MCP connection.

And that is how rowlite was born.

### The rowlite Philosophy
- **Schema Guardrails**: No more uneven rows or missing cells. If a row doesn't match the table definition, rowlite corrects it or rejects it.
- **100% Standard Markdown**: Your data remains completely open, human-readable, and perfectly rendered on GitHub or Obsidian.
- **Double-Agent Design**: Fast, native CLI commands for your terminal, and an out-of-the-box MCP server for your AI agents.

*PS: The table was not in a Git repository, but I was able to recover it in the end.*

## Features
- **Zero-config by default**: Reads columns from the first row of GFM tables without requiring database creation.
- **In-place updates**: Safely edits Markdown GFM tables without disturbing surrounding headings, notes, or descriptions in the document.
- **Auto-sorting and deduplication**: Declared primary/unique keys and sorting configurations automatically organize rows upon every append or mutation.
- **Atomicity**: Uses temporary file renaming for all write-back operations to prevent database truncation or corruption.
- **Model Context Protocol native**: Fully compatible with the MCP specification for seamless integration with AI agent loops (Cursor, Claude Code, etc.).

## Quickstart

### 1. Install Rowlite
Install `rowlite` globally via Go:

```bash
go install github.com/pipik-roman/rowlite/cmd/rowlite@latest
```

Ensure your Go bin directory (typically `~/go/bin` or `%USERPROFILE%\go\bin`) is in your system's `PATH`.

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
> Check out the [Complete MCP Configuration Guide](docs/mcp-setup.md) for target path matrices, manual JSON config blocks, and setup flags.

## CLI Commands
- `rowlite structure <file>` - Display column names, constraints, and audit log mappings.
- `rowlite list <file> --where "<column> <op> <value>"` - Query and filter rows.
- `rowlite append <file> --row '{"col1": "val1"}'` - Append a row, applying schema validations and auto-sorting.
- `rowlite dedupe <file> --column <name>` - Purge duplicate records in place.

### Query Filtering (`--where`)
Both the `list` and `count` commands support filtering rows using the `--where` flag. Column names in `--where` clauses are resolved using a **Case-Insensitive & Normalized Lookup Pipeline** (e.g. `tags`, `item_name` or `item-name` match their respective headers automatically).

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

## Tech Stack
- Built in **Go** (for performance).
- CLI framework: [Cobra](https://github.com/spf13/cobra)
- Markdown GFM parsing: [Goldmark](https://github.com/yuin/goldmark)
- Frontmatter parsing: [frontmatter](https://github.com/adrg/frontmatter)
- MCP communication: [official Go SDK](https://github.com/modelcontextprotocol/go-sdk)

## License
MIT License. See [LICENSE](LICENSE) for details.
