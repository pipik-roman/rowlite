# Rowlite MCP Client Configuration Guide

This document provides a comprehensive guide for configuring `rowlite` as a Model Context Protocol (MCP) server across various AI host clients, both automatically using the CLI setup driver and manually via configuration files.

---

## 1. Automatic Configuration (CLI Setup Driver)

`rowlite` includes a built-in setup driver (`rowlite mcp-setup`, aliased as `rowlite setup-mcp`) designed to automatically register the server with your target AI clients using the absolute path of your compiled binary.

By default, the driver modifies **Global User Configurations** to make Rowlite tools available system-wide across all projects without cluttering individual repositories.

### Flag Reference

| Flag | Shorthand | Values | Default | Description |
|---|---|---|---|---|
| `--client` | `-c` | `cursor`, `vscode`, `copilot`, `antigravity`, `claude`, `zed`, `cline`, `crush`, `all` | *Interactive Wizard* | Specifies which AI client environment(s) to configure. |
| `--scale` | `-s` | `global`, `workspace` | `global` | Determines whether to target global user profiles or local workspace directories. |
| `--dry-run` | *None* | `true`, `false` | `false` | Displays projected JSON changes for the targeted client without writing to files. |

### Operational Guarantees
* **Targeted Modifications**: The driver updates configuration files only for the specific applications targeted.
* **Explicit Workspace Opt-in**: Local repository workspace files (e.g. `.vscode/mcp.json`) are only modified when `--scale workspace` is explicitly provided. This prevents unprompted Git working tree contamination.
* **Atomic File Merging**: Modifies the JSON structure safely, keeping any other pre-existing MCP servers in the target configurations untouched.
* **Secure Temp Writes**: Writes are staged using temporary files with restrictive permissions (`0600`) and replaced atomically using `os.Rename` to avoid write corruption.

### Common CLI Usage Patterns

#### A. Configure Cursor Globally
Register the Rowlite MCP server globally for Cursor:
```bash
rowlite mcp-setup --client cursor
```
*Modified Path:* `~/.cursor/mcp.json`

#### B. Configure GitHub Copilot CLI Globally
Register the Rowlite MCP server globally for GitHub Copilot CLI (makes tools available in all terminal sessions):
```bash
rowlite mcp-setup --client copilot
```
*Modified Path:* `~/.copilot/mcp-config.json`

#### C. Configure Visual Studio Code for a Specific Workspace
Initialize a workspace-level configuration inside a specific repository for team collaboration:
```bash
rowlite mcp-setup --client vscode --scale workspace
```
*Modified Path:* `./.vscode/mcp.json`

#### D. Interactive Configuration Wizard
Running the setup driver without arguments starts the auto-discovery wizard, scanning your system for installed tools and prompting before making any changes:
```bash
rowlite mcp-setup
```

### Target Configuration Path Matrix

| Client | Global Path (`--scale global`) | Workspace Path (`--scale workspace`) | Schema Key |
|---|---|---|---|
| **Cursor** | `~/.cursor/mcp.json` | `.cursor/mcp.json` | `mcpServers` |
| **Visual Studio Code** | *User Profile Store* | `.vscode/mcp.json` | `servers` |
| **GitHub Copilot CLI** | `~/.copilot/mcp-config.json` | `.copilot/mcp-config.json` | `mcpServers` |
| **Google Antigravity** | `~/.gemini/config/mcp_config.json` | `.agents/mcp_config.json` | `mcpServers` |
| **Claude Desktop** | `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)<br>`%APPDATA%\Claude\claude_desktop_config.json` (Windows) | *N/A* | `mcpServers` |
| **Claude Code** | `~/.claude.json` | `.mcp.json` | `mcpServers` |
| **Zed** | `~/.config/zed/settings.json` | *N/A* | `context_servers` |
| **Cline** | *User Profile Store (globalStorage/saoudrizwan.claude-dev)* | *N/A* | `mcpServers` |
| **Crush (Charm)** | `~/.config/crush/crush.json` | `.crush.json` | `mcp` |

---

## 2. Manual Configuration

If you prefer manual configuration, edit the target JSON settings file for your AI client and insert the appropriate block below. Make sure to replace `/path/to/rowlite` with the actual absolute path to your compiled `rowlite` binary.

### A. Cursor, Claude Desktop, Antigravity, Windsurf, Copilot CLI, Cline
These clients search for a top-level `"mcpServers"` key in their global or workspace configuration files:

```json
{
  "mcpServers": {
    "rowlite": {
      "command": "/path/to/rowlite",
      "args": ["mcp"]
    }
  }
}
```

> [!NOTE]
> For GitHub Copilot CLI, you may need to explicitly expose tools by adding `"tools": ["*"]` inside the `rowlite` configuration object.

### B. Visual Studio Code (Workspace Extensions)
VS Code extensions (such as Cline or Roo Code in workspace scope) search for a top-level `"servers"` key in `.vscode/mcp.json`:

```json
{
  "servers": {
    "rowlite": {
      "command": "/path/to/rowlite",
      "args": ["mcp"]
    }
  }
}
```

### C. Zed
Zed reads a `"context_servers"` key in its global settings (`~/.config/zed/settings.json`):

```json
{
  "context_servers": {
    "rowlite": {
      "command": {
        "path": "/path/to/rowlite",
        "args": ["mcp"]
      }
    }
  }
}
```

### D. Crush (Charm)
Crush uses a top-level `"mcp"` key in global (`~/.config/crush/crush.json`) or workspace (`.crush.json`) settings files:

```json
{
  "mcp": {
    "rowlite": {
      "type": "stdio",
      "command": "/path/to/rowlite",
      "args": ["mcp"]
    }
  }
}
```
