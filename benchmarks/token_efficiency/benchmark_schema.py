#!/usr/bin/env python3
"""
Rowlite MCP Tool Schema Size & Token Footprint Benchmark.
Measures and compares per-tool and total MCP JSON Schema sizes:
  - Property Count
  - Raw Serialized JSON Bytes
  - Exact Token Count (tiktoken cl100k_base)
  - Comparison against legacy 54-property (2,111-token) baseline or historical snapshots
"""

import argparse
from datetime import datetime
import json
import os
import sys
from typing import Any, Dict, List, Optional, Tuple

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
sys.path.insert(0, REPO_ROOT)

from benchmarks.token_efficiency.mcp_client import StdioMCPClient

# Historical pre-slimming baseline (1:1 CLI flag dump, 54 properties, 2,111 tokens)
LEGACY_SCHEMA_BASELINE: Dict[str, Dict[str, Any]] = {
    "rowlite_init": {
        "properties_count": 8,
        "properties": ["file", "columns", "unique", "sort", "audit_insert", "audit_edit", "journal_log", "autoincrement"],
        "tokens": 293,
        "bytes": 1246,
    },
    "rowlite_list": {
        "properties_count": 8,
        "properties": ["file", "where", "head", "tail", "index", "offset", "limit", "include_metadata"],
        "tokens": 324,
        "bytes": 1384,
    },
    "rowlite_count": {
        "properties_count": 7,
        "properties": ["file", "where", "head", "tail", "index", "offset", "limit"],
        "tokens": 234,
        "bytes": 1012,
    },
    "rowlite_append": {
        "properties_count": 4,
        "properties": ["file", "row", "if_match", "if_checksum"],
        "tokens": 198,
        "bytes": 865,
    },
    "rowlite_update": {
        "properties_count": 12,
        "properties": ["file", "where", "head", "tail", "index", "offset", "limit", "set", "if_match", "if_checksum", "all", "dry_run"],
        "tokens": 434,
        "bytes": 1842,
    },
    "rowlite_delete": {
        "properties_count": 11,
        "properties": ["file", "where", "head", "tail", "index", "offset", "limit", "if_match", "if_checksum", "all", "dry_run"],
        "tokens": 389,
        "bytes": 1654,
    },
    "rowlite_sort": {
        "properties_count": 3,
        "properties": ["file", "by", "trigger"],
        "tokens": 140,
        "bytes": 612,
    },
    "rowlite_get_structure": {
        "properties_count": 1,
        "properties": ["file"],
        "tokens": 90,
        "bytes": 418,
    },
}
LEGACY_TOTAL_TOKENS = 2111
LEGACY_TOTAL_PROPS = 54
LEGACY_TOTAL_BYTES = 9033


def measure_mcp_schema(rowlite_bin: str = "rowlite") -> Dict[str, Any]:
    import tiktoken

    enc = tiktoken.get_encoding("cl100k_base")
    with StdioMCPClient(command=rowlite_bin, cwd=REPO_ROOT) as client:
        tools = client.list_tools()

    per_tool: Dict[str, Dict[str, Any]] = {}
    total_bytes = 0
    total_props = 0

    for t in tools:
        fn = t.get("function", {})
        name = fn.get("name", "unknown")
        params = fn.get("parameters", {})
        props = list(params.get("properties", {}).keys())
        serialized = json.dumps(t)
        b_len = len(serialized.encode("utf-8"))
        tok_len = len(enc.encode(serialized))

        per_tool[name] = {
            "name": name,
            "description": fn.get("description", ""),
            "properties_count": len(props),
            "properties": props,
            "bytes": b_len,
            "tokens": tok_len,
        }
        total_bytes += b_len
        total_props += len(props)

    total_serialized = json.dumps(tools)
    total_tokens = len(enc.encode(total_serialized))

    return {
        "timestamp": datetime.now().isoformat(),
        "total_tools": len(tools),
        "total_properties": total_props,
        "total_bytes": len(total_serialized.encode("utf-8")),
        "total_tokens": total_tokens,
        "tools": per_tool,
    }


def format_schema_comparison_console(current: Dict[str, Any], baseline_map: Dict[str, Dict[str, Any]], base_total_tokens: int, base_total_props: int) -> str:
    lines = [
        "\n================================================================================================================",
        " ROWLITE MCP TOOL SCHEMA SIZE BENCHMARK (Per-Tool & Total Token Footprint - tiktoken cl100k_base)",
        "================================================================================================================",
        f" {'MCP Tool Name':<24} | {'Legacy Props':<12} | {'Curr Props':<10} | {'Legacy Tokens':<14} | {'Curr Tokens':<12} | {'Token Reduction':<16} | {'Exposed Properties'}",
        "--------------------------+--------------+------------+----------------+--------------+------------------+------------------------------",
    ]

    for name, info in sorted(current["tools"].items()):
        base = baseline_map.get(name, {})
        b_props = base.get("properties_count", 0)
        b_tok = base.get("tokens", 0)
        c_props = info["properties_count"]
        c_tok = info["tokens"]
        red_pct = ((b_tok - c_tok) / b_tok * 100.0) if b_tok > 0 else 0.0
        props_str = ", ".join(info["properties"])
        lines.append(
            f" {name:<24} | {b_props:>12} | {c_props:>10} | {b_tok:>14} | {c_tok:>12} | {red_pct:>14.1f}% | [{props_str}]"
        )

    tot_tok = current["total_tokens"]
    tot_props = current["total_properties"]
    tot_red = ((base_total_tokens - tot_tok) / base_total_tokens * 100.0) if base_total_tokens > 0 else 0.0

    lines.extend([
        "--------------------------+--------------+------------+----------------+--------------+------------------+------------------------------",
        f" {'TOTAL MCP SCHEMA TAX':<24} | {base_total_props:>12} | {tot_props:>10} | {base_total_tokens:>14} | {tot_tok:>12} | {tot_red:>14.1f}% | ({current['total_bytes']} bytes vs {LEGACY_TOTAL_BYTES} B)",
        "================================================================================================================",
        f" 3-Turn Trajectory Savings: {(base_total_tokens - tot_tok) * 3:,} fewer prompt tokens per 3-turn agent task!",
        "================================================================================================================",
    ])
    return "\n".join(lines)


def format_schema_markdown_table(current: Dict[str, Any]) -> List[str]:
    tot_tok = current["total_tokens"]
    tot_props = current["total_properties"]
    tot_bytes = current["total_bytes"]

    lines = [
        "| MCP Tool | Properties | Schema Bytes | Schema Tokens (`cl100k_base`) | Parameters |",
        "| :--- | :---: | :---: | :---: | :--- |",
    ]
    for name, info in sorted(current["tools"].items()):
        c_props = info["properties_count"]
        c_bytes = info["bytes"]
        c_tok = info["tokens"]
        props_str = ", ".join(f"`{p}`" for p in info["properties"])
        lines.append(
            f"| **{name}** | {c_props} | {c_bytes:,} B | {c_tok} | {props_str} |"
        )
    lines.append(
        f"| **TOTAL** | **{tot_props}** | **{tot_bytes:,} B** | **{tot_tok:,}** | **8 MCP Tools** |"
    )
    return lines


def format_schema_snapshot_markdown(current: Dict[str, Any]) -> str:
    tot_tok = current["total_tokens"]
    tot_props = current["total_properties"]
    tot_bytes = current["total_bytes"]

    md = [
        f"# Benchmark Snapshot: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')} (MCP Tool Schema Size Footprint)",
        "",
        "- **Milestone**: AI-First Minimal MCP Schema Footprint & Server Instructions",
        "- **Tokenizer**: `tiktoken` (`cl100k_base`)",
        f"- **Total Schema Tax**: **{tot_tok:,} tokens** per turn ({tot_bytes:,} bytes JSON schema)",
        f"- **Total Properties**: **{tot_props} properties** across {current['total_tools']} tools",
        "",
        "---",
        "",
        "## Per-Tool & Total MCP Schema Size",
        "",
    ]
    md.extend(format_schema_markdown_table(current))
    md.extend([
        "",
        "---",
        "",
        "## Architectural Principles Enforced",
        "",
        "1. **Asymmetric CLI vs. MCP Surface**: Removed non-AI human CLI options (`--dry-run`, `--all`, `--if-checksum`, and redundant positional slicing on mutations).",
        "2. **Shared Server-Level Instructions (`mcp.ServerOptions.Instructions`)**: Moved cross-cutting filter (`==, !=, like, regex`) and sort (`Col:asc|desc`) syntax rules to the MCP initialization handshake.",
        "3. **Zero-Tag Self-Describing Properties**: Omitted redundant `jsonschema` descriptions on self-explanatory property names (`file`, `columns`, `unique`, `limit`, `offset`, `row`, `set`).",
        "",
        "---",
        "*Generated via `benchmarks/token_efficiency/benchmark_schema.py`.*",
    ])
    return "\n".join(md) + "\n"


def run_schema_benchmark(rowlite_bin: str = "rowlite", save_snapshot: bool = False, output_json: Optional[str] = None) -> Dict[str, Any]:
    current = measure_mcp_schema(rowlite_bin=rowlite_bin)
    print(format_schema_comparison_console(current, LEGACY_SCHEMA_BASELINE, LEGACY_TOTAL_TOKENS, LEGACY_TOTAL_PROPS))

    if output_json:
        with open(output_json, "w", encoding="utf-8") as f:
            json.dump(current, f, indent=2)
        print(f"[+] Saved schema benchmark JSON to: {output_json}")

    if save_snapshot:
        now = datetime.now()
        file_ts = now.strftime("%Y-%m-%d_%H%M%S")
        snap_filename = f"{file_ts}_mcp_schema_size.md"
        benchmarks_dir = os.path.join(REPO_ROOT, "benchmarks")
        snap_path = os.path.join(benchmarks_dir, snap_filename)
        with open(snap_path, "w", encoding="utf-8") as f:
            f.write(format_schema_snapshot_markdown(current))
        print(f"[+] Saved MCP schema size snapshot to: {snap_path}")

    return current


def main():
    parser = argparse.ArgumentParser(description="Rowlite MCP Tool Schema Size Benchmark")
    parser.add_argument("--rowlite-bin", type=str, default="rowlite", help="Path to rowlite binary")
    parser.add_argument("--save-snapshot", action="store_true", help="Save markdown snapshot to benchmarks/")
    parser.add_argument("--output-json", type=str, default=None, help="Write JSON metrics to file")
    args = parser.parse_args()

    run_schema_benchmark(
        rowlite_bin=args.rowlite_bin,
        save_snapshot=args.save_snapshot,
        output_json=args.output_json,
    )


if __name__ == "__main__":
    main()
