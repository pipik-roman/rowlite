# Rowlite

Rowlite is a zero-config, non-SQL, lightweight command-line tool and Model Context Protocol (MCP) server that treats standard Markdown files as flat, schema-validated databases.

Instead of writing SQL queries, developers and AI agents interact with tabular data using simple, declarative actions (`list`, `append`, `update`, `delete`, `dedupe`) directly over columns.

## Features
- **Zero-config by default**: Reads columns from the first row of GFM tables without requiring database creation.
- **In-place updates**: Safely edits Markdown GFM tables without disturbing surrounding headings, notes, or descriptions in the document.
- **Auto-sorting and deduplication**: Declared primary/unique keys and sorting configurations automatically organize rows upon every append or mutation.
- **Atomicity**: Uses temporary file renaming for all write-back operations to prevent database truncation or corruption.
- **Model Context Protocol native**: Fully compatible with the MCP specification for seamless integration with AI agent loops (Cursor, Claude Code, etc.).

## Installation
*(Go installation details will be added as version releases compile).*

## CLI Commands
- `rowlite structure <file>` - Display column names, constraints, and audit log mappings.
- `rowlite list <file> --where "<column> <op> <value>"` - Query and filter rows.
- `rowlite append <file> --row '{"col1": "val1"}'` - Append a row, applying schema validations and auto-sorting.
- `rowlite dedupe <file> --column <name>` - Purge duplicate records in place.

## Tech Stack
- Built in **Go** (for millisecond startup performance).
- CLI framework: [Cobra](https://github.com/spf13/cobra)
- Markdown GFM parsing: [Goldmark](https://github.com/yuin/goldmark)
- Frontmatter parsing: [frontmatter](https://github.com/adrg/frontmatter)
- MCP communication: [official Go SDK](https://github.com/modelcontextprotocol/go-sdk)

## License
MIT License. See [LICENSE](LICENSE) for details.
