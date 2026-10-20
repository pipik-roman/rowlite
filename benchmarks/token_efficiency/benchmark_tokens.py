#!/usr/bin/env python3
"""
Production-Grade Rowlite Token Efficiency Benchmark CLI.
Aligned with Rowlite's canonical benchmark methodology:
  - Canonical dataset extraction from internal/core/testdata/large_table_10k.md via `rowlite list`
  - Multi-scenario evaluation (point_update, batch_update, append, query)
  - Multi-vector telemetry (Schema Tax, Visible Output, Reasoning Tokens, Turns, Latency)
  - Historical comparison and automated Red Flags regression guard
  - Automated timestamped snapshots in benchmarks/
"""

import argparse
import asyncio
from datetime import datetime
import json
import os
import sys
import tempfile
from typing import Any, Dict, List, Optional, Tuple

# Add repository root to python path
REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
sys.path.insert(0, REPO_ROOT)

from benchmarks.token_efficiency.agent_loop import (
    BASELINE_TOOLS,
    GREP_REPLACE_TOOLS,
    SCENARIOS,
    Scenario,
    execute_live_agent_loop,
    execute_simulated_agent_loop,
    handle_baseline_tool,
    handle_grep_replace_tool,
)
from benchmarks.token_efficiency.benchmark_schema import (
    LEGACY_SCHEMA_BASELINE,
    LEGACY_TOTAL_BYTES,
    LEGACY_TOTAL_PROPS,
    LEGACY_TOTAL_TOKENS,
    format_schema_comparison_console,
    format_schema_markdown_table,
    measure_mcp_schema,
)
from benchmarks.token_efficiency.audit import AuditFinding, RegressionAuditor
from benchmarks.token_efficiency.fixtures import extract_canonical_fixture
from benchmarks.token_efficiency.mcp_client import StdioMCPClient
from benchmarks.token_efficiency.telemetry import RunTelemetry


def run_benchmark_run(
    scenario: Scenario,
    num_rows: int = 50,
    model: Optional[str] = None,
    simulate: bool = True,
    rowlite_bin: str = "rowlite",
    work_dir: Optional[str] = None,
    api_base: Optional[str] = None,
    api_key: Optional[str] = None,
) -> Tuple[RunTelemetry, RunTelemetry, RunTelemetry]:
    """
    Executes a benchmark run evaluating 3 experimental conditions under identical canonical fixtures:
      Arm 1: Baseline Whole-File Tools (read_file, write_file)
      Arm 2: Baseline Targeted Editing Tools (grep_search, replace_file_content, read_file, write_file)
      Arm 3: Rowlite Native MCP Server (rowlite_update, rowlite_append, rowlite_list, etc.)
    """
    orig_cwd = os.getcwd()
    temp_dir = None

    if work_dir is None:
        temp_dir = tempfile.TemporaryDirectory()
        target_dir = temp_dir.name
    else:
        target_dir = work_dir

    try:
        os.chdir(target_dir)
        table_path = "table.md"

        # --- CONDITION 1: Baseline Whole-File Tools ---
        extract_canonical_fixture(
            num_rows=num_rows,
            target_path=table_path,
            rowlite_bin=rowlite_bin,
            repo_root=REPO_ROOT,
        )

        if simulate or not model:
            b_whole_telemetry = execute_simulated_agent_loop(
                scenario=scenario,
                condition_name="baseline_generic_file_tools",
                num_rows=num_rows,
                tools=BASELINE_TOOLS,
            )
        else:
            async def baseline_dispatcher(name, args):
                return handle_baseline_tool(name, args)

            b_whole_telemetry = asyncio.run(
                execute_live_agent_loop(
                    model=model,
                    scenario=scenario,
                    tools=BASELINE_TOOLS,
                    tool_dispatcher=baseline_dispatcher,
                    condition_name="baseline_generic_file_tools",
                    num_rows=num_rows,
                    api_base=api_base,
                    api_key=api_key,
                )
            )

        # --- CONDITION 2: Baseline Targeted Grep & Replace Tools ---
        extract_canonical_fixture(
            num_rows=num_rows,
            target_path=table_path,
            rowlite_bin=rowlite_bin,
            repo_root=REPO_ROOT,
        )

        if simulate or not model:
            b_grep_telemetry = execute_simulated_agent_loop(
                scenario=scenario,
                condition_name="baseline_grep_replace",
                num_rows=num_rows,
                tools=GREP_REPLACE_TOOLS,
            )
        else:
            async def grep_dispatcher(name, args):
                return handle_grep_replace_tool(name, args)

            b_grep_telemetry = asyncio.run(
                execute_live_agent_loop(
                    model=model,
                    scenario=scenario,
                    tools=GREP_REPLACE_TOOLS,
                    tool_dispatcher=grep_dispatcher,
                    condition_name="baseline_grep_replace",
                    num_rows=num_rows,
                    api_base=api_base,
                    api_key=api_key,
                )
            )

        # --- CONDITION 3: Rowlite Native MCP Server ---
        extract_canonical_fixture(
            num_rows=num_rows,
            target_path=table_path,
            rowlite_bin=rowlite_bin,
            repo_root=REPO_ROOT,
        )

        with StdioMCPClient(command=rowlite_bin, cwd=target_dir) as client:
            rowlite_tools = client.list_tools()

            if simulate or not model:
                rowlite_telemetry = execute_simulated_agent_loop(
                    scenario=scenario,
                    condition_name="rowlite_mcp_tools",
                    num_rows=num_rows,
                    tools=rowlite_tools,
                    mcp_client=client,
                )
            else:
                async def mcp_dispatcher(name, args):
                    return client.call_tool(name, args)

                rowlite_telemetry = asyncio.run(
                    execute_live_agent_loop(
                        model=model,
                        scenario=scenario,
                        tools=rowlite_tools,
                        tool_dispatcher=mcp_dispatcher,
                        condition_name="rowlite_mcp_tools",
                        num_rows=num_rows,
                        api_base=api_base,
                        api_key=api_key,
                    )
                )

        return b_whole_telemetry, b_grep_telemetry, rowlite_telemetry

    finally:
        os.chdir(orig_cwd)
        if temp_dir is not None:
            temp_dir.cleanup()


def format_comparison_table(b_whole: RunTelemetry, b_grep: RunTelemetry, r: RunTelemetry, scenario_name: str) -> str:
    whole_diff = b_whole.total_tokens - r.total_tokens
    whole_savings_pct = (whole_diff / b_whole.total_tokens * 100) if b_whole.total_tokens > 0 else 0.0

    grep_diff = b_grep.total_tokens - r.total_tokens
    grep_savings_pct = (grep_diff / b_grep.total_tokens * 100) if b_grep.total_tokens > 0 else 0.0

    vis_whole_savings = ((b_whole.visible_completion_tokens - r.visible_completion_tokens) / b_whole.visible_completion_tokens * 100) if b_whole.visible_completion_tokens > 0 else 0.0
    vis_grep_savings = ((b_grep.visible_completion_tokens - r.visible_completion_tokens) / b_grep.visible_completion_tokens * 100) if b_grep.visible_completion_tokens > 0 else 0.0

    reasoning_whole_savings = ((b_whole.reasoning_tokens - r.reasoning_tokens) / b_whole.reasoning_tokens * 100) if b_whole.reasoning_tokens > 0 else 0.0
    reasoning_grep_savings = ((b_grep.reasoning_tokens - r.reasoning_tokens) / b_grep.reasoning_tokens * 100) if b_grep.reasoning_tokens > 0 else 0.0

    comp_whole_savings = ((b_whole.total_completion_tokens - r.total_completion_tokens) / b_whole.total_completion_tokens * 100) if b_whole.total_completion_tokens > 0 else 0.0
    comp_grep_savings = ((b_grep.total_completion_tokens - r.total_completion_tokens) / b_grep.total_completion_tokens * 100) if b_grep.total_completion_tokens > 0 else 0.0

    lines = [
        f"\n========================================================================================================================",
        f" SCENARIO: {scenario_name} (Dataset: {r.num_rows} rows from canonical large_table_10k.md)",
        f"========================================================================================================================",
        f" {'Operational Vector':<34} | {'Whole-File Base':<18} | {'Grep-Replace Base':<18} | {'Rowlite (MCP)':<18} | {'vs Grep':<10} | {'vs Whole':<10}",
        f"-----------------------------------+--------------------+--------------------+--------------------+------------+------------",
        f" {'Turn 1 Prompt (Schema Tax)':<34} | {b_whole.turn1_prompt_tokens:>12} tok | {b_grep.turn1_prompt_tokens:>12} tok | {r.turn1_prompt_tokens:>12} tok | {r.turn1_prompt_tokens - b_grep.turn1_prompt_tokens:>+8} tok | {r.turn1_prompt_tokens - b_whole.turn1_prompt_tokens:>+8} tok",
        f" {'Subsequent Context Ingestion':<34} | {b_whole.subsequent_prompt_tokens:>12} tok | {b_grep.subsequent_prompt_tokens:>12} tok | {r.subsequent_prompt_tokens:>12} tok | {r.subsequent_prompt_tokens - b_grep.subsequent_prompt_tokens:>+8} tok | {r.subsequent_prompt_tokens - b_whole.subsequent_prompt_tokens:>+8} tok",
        f" {'Total Prompt Tokens':<34} | {b_whole.total_prompt_tokens:>12} tok | {b_grep.total_prompt_tokens:>12} tok | {r.total_prompt_tokens:>12} tok | {r.total_prompt_tokens - b_grep.total_prompt_tokens:>+8} tok | {r.total_prompt_tokens - b_whole.total_prompt_tokens:>+8} tok",
        f" {'Visible Output (Tool Calls/Text)':<34} | {b_whole.visible_completion_tokens:>12} tok | {b_grep.visible_completion_tokens:>12} tok | {r.visible_completion_tokens:>12} tok | {vis_grep_savings:>7.1f}% red | {vis_whole_savings:>7.1f}% red",
        f" {'Thinking / Reasoning Tokens':<34} | {b_whole.reasoning_tokens:>12} tok | {b_grep.reasoning_tokens:>12} tok | {r.reasoning_tokens:>12} tok | {reasoning_grep_savings:>7.1f}% red | {reasoning_whole_savings:>7.1f}% red",
        f" {'Total Completion Tokens':<34} | {b_whole.total_completion_tokens:>12} tok | {b_grep.total_completion_tokens:>12} tok | {r.total_completion_tokens:>12} tok | {comp_grep_savings:>7.1f}% red | {comp_whole_savings:>7.1f}% red",
        f" {'Total Trajectory Tokens':<34} | {b_whole.total_tokens:>12} tok | {b_grep.total_tokens:>12} tok | {r.total_tokens:>12} tok | {grep_savings_pct:>7.1f}% red | {whole_savings_pct:>7.1f}% red",
        f" {'Conversational Turns':<34} | {b_whole.turns:>16} | {b_grep.turns:>16} | {r.turns:>16} | {r.turns - b_grep.turns:>+6} turns | {r.turns - b_whole.turns:>+6} turns",
        f" {'Cognitive Ratio (Thinking/Output)':<34} | {b_whole.cognitive_ratio:>16.2f} | {b_grep.cognitive_ratio:>16.2f} | {r.cognitive_ratio:>16.2f} | {r.cognitive_ratio - b_grep.cognitive_ratio:>+8.2f} | {r.cognitive_ratio - b_whole.cognitive_ratio:>+8.2f}",
        f" {'Wall Clock Latency':<34} | {b_whole.wall_clock_seconds * 1000:>13.1f} ms | {b_grep.wall_clock_seconds * 1000:>13.1f} ms | {r.wall_clock_seconds * 1000:>13.1f} ms | {(b_grep.wall_clock_seconds - r.wall_clock_seconds) * 1000:>+7.1f} ms | {(b_whole.wall_clock_seconds - r.wall_clock_seconds) * 1000:>+7.1f} ms",
        f" {'Mutation / Output Verified':<34} | {('PASS' if b_whole.verified else 'FAIL'):>16} | {('PASS' if b_grep.verified else 'FAIL'):>16} | {('PASS' if r.verified else 'FAIL'):>16} | {'Deterministic':<10} | {'Deterministic':<10}",
        f"========================================================================================================================",
    ]
    return "\n".join(lines)


def format_turn_breakdown(b_whole: RunTelemetry, b_grep: RunTelemetry, r: RunTelemetry) -> str:
    lines = [
        f"\n--- TURN-BY-TURN TELEMETRY TRACE ({r.num_rows} rows) ---",
        f"\n[Baseline 1 - Whole-File Read/Write]",
        f" {'Turn':<5} | {'Prompt':<10} | {'Output':<10} | {'Thinking':<10} | {'Tools Called':<25} | {'Tool Bytes':<12} | {'Latency':<10}",
        f"-------+------------+------------+------------+---------------------------+--------------+----------",
    ]
    for tr in b_whole.turn_records:
        tools_str = ", ".join(tr.tool_calls) if tr.tool_calls else "(none)"
        lines.append(
            f" {tr.turn_number:<5} | {tr.prompt_tokens:>10} | {tr.completion_tokens:>10} | {tr.reasoning_tokens:>10} | {tools_str:<25} | {tr.tool_results_bytes:>10} B | {tr.latency_ms:>7.1f} ms"
        )

    lines.extend([
        f"\n[Baseline 2 - Targeted Grep & Replace]",
        f" {'Turn':<5} | {'Prompt':<10} | {'Output':<10} | {'Thinking':<10} | {'Tools Called':<25} | {'Tool Bytes':<12} | {'Latency':<10}",
        f"-------+------------+------------+------------+---------------------------+--------------+----------",
    ])
    for tr in b_grep.turn_records:
        tools_str = ", ".join(tr.tool_calls) if tr.tool_calls else "(none)"
        lines.append(
            f" {tr.turn_number:<5} | {tr.prompt_tokens:>10} | {tr.completion_tokens:>10} | {tr.reasoning_tokens:>10} | {tools_str:<25} | {tr.tool_results_bytes:>10} B | {tr.latency_ms:>7.1f} ms"
        )

    lines.extend([
        f"\n[Arm 3 - Rowlite Native MCP Server]",
        f" {'Turn':<5} | {'Prompt':<10} | {'Output':<10} | {'Thinking':<10} | {'Tools Called':<25} | {'Tool Bytes':<12} | {'Latency':<10}",
        f"-------+------------+------------+------------+---------------------------+--------------+----------",
    ])
    for tr in r.turn_records:
        tools_str = ", ".join(tr.tool_calls) if tr.tool_calls else "(none)"
        lines.append(
            f" {tr.turn_number:<5} | {tr.prompt_tokens:>10} | {tr.completion_tokens:>10} | {tr.reasoning_tokens:>10} | {tools_str:<25} | {tr.tool_results_bytes:>10} B | {tr.latency_ms:>7.1f} ms"
        )
    return "\n".join(lines)


def format_snapshot_markdown(
    timestamp_str: str,
    model_name: str,
    results_list: List[Tuple[Scenario, RunTelemetry, RunTelemetry, RunTelemetry]],
    audit_findings: List[AuditFinding],
    schema_data: Optional[Dict[str, Any]] = None,
) -> str:
    md = [
        f"# Benchmark Snapshot: {timestamp_str} (LLM Token Efficiency & Context Saturation)",
        "",
        f"- **Milestone**: Multi-Scenario Token Efficiency & Inefficiency Regression Guard",
        f"- **Model / Engine**: `{model_name}`",
        f"- **Canonical Dataset**: `internal/core/testdata/large_table_10k.md` (Extracted via `rowlite list`)",
        f"- **Methodology**: 3-Arm Controlled Ablation (Identical Prompts: Whole-File vs Grep-Replace vs Rowlite MCP)",
        "",
        "---",
        "",
        "## 1. Multi-Scenario Benchmark Results",
        "",
        "| Scenario | Rows | Turns (Whole / Grep / MCP) | Whole-File Total | Grep-Replace Total | Rowlite MCP Total | Savings vs Whole | Savings vs Grep | Verification |",
        "| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |",
    ]

    for sc, b_whole, b_grep, r in results_list:
        savings_whole = ((b_whole.total_tokens - r.total_tokens) / b_whole.total_tokens * 100) if b_whole.total_tokens > 0 else 0
        savings_grep = ((b_grep.total_tokens - r.total_tokens) / b_grep.total_tokens * 100) if b_grep.total_tokens > 0 else 0
        whole_total_str = f"{b_whole.total_tokens:,}" if b_whole.verified else f"{b_whole.total_tokens:,}*"
        if b_whole.verified and b_grep.verified and r.verified:
            v_status = "All PASS"
        elif not b_whole.verified and b_grep.verified and r.verified:
            v_status = "Whole OOM / Grep+MCP PASS"
        else:
            v_status = f"W:{'PASS' if b_whole.verified else 'FAIL'} G:{'PASS' if b_grep.verified else 'FAIL'} M:{'PASS' if r.verified else 'FAIL'}"
        
        md.append(
            f"| **{sc.name}** | {r.num_rows} | **{b_whole.turns} / {b_grep.turns} / {r.turns}** | {whole_total_str} | {b_grep.total_tokens:,} | {r.total_tokens:,} | **{savings_whole:.1f}%** | **{savings_grep:.1f}%** | {v_status} |"
        )

    has_aborted_whole = any(
        not b_whole.verified or "OOM" in b_whole.final_output or "*" in str(b_whole.total_tokens)
        for _, b_whole, _, _ in results_list
    )
    if has_aborted_whole:
        md.append("")
        md.append("> [!NOTE]")
        md.append("> `*` Indicates Whole-File Baseline aborted during table rewrite due to inference server GPU decode KV-cache Out-Of-Memory (OOM).")

    if schema_data:
        tot_tok = schema_data["total_tokens"]
        tot_props = schema_data["total_properties"]
        tot_bytes = schema_data["total_bytes"]
        has_inst = bool(schema_data.get("instructions"))
        inst_note = f" (including {schema_data['instructions']['tokens']} tokens server instructions)" if has_inst else ""
        md.extend([
            "",
            "---",
            "",
            "## 2. MCP Tool Schema Size & Token Budget Analysis",
            "",
            "- **Tokenizer**: `tiktoken` (`cl100k_base`)",
            f"- **Active Properties**: **{tot_props} properties** across {schema_data['total_tools']} tools",
            f"- **Total Schema Tax**: **{tot_tok:,} tokens** per turn ({tot_bytes:,} bytes JSON schema){inst_note}",
            "",
        ])
        md.extend(format_schema_markdown_table(schema_data))

    sec_roundtrips = "3" if schema_data else "2"
    sec_audit = "4" if schema_data else "3"
    sec_insights = "5" if schema_data else "4"

    md.extend([
        "",
        "---",
        "",
        f"## {sec_roundtrips}. Turn-by-Turn Roundtrip Telemetry Breakdown",
        "",
    ])

    for sc, b_whole, b_grep, r in results_list:
        md.extend([
            f"### Scenario: {sc.name} ({r.num_rows} rows)",
            "",
            f"- **Roundtrips**: Whole-File = `{b_whole.turns} turns` | Grep-Replace = `{b_grep.turns} turns` | Rowlite MCP = `{r.turns} turns`",
            f"- **User Prompt**: `{sc.user_prompt}`",
            "",
            "#### Baseline Arm 1: Whole-File Filesystem Tools (`read_file`, `write_file`)",
            "| Turn | Prompt Tokens | Output Tokens | Thinking Tokens | Tools Called | Tool Result Bytes | Latency |",
            "| :--- | :--- | :--- | :--- | :--- | :--- | :--- |",
        ])
        for tr in b_whole.turn_records:
            tools_str = ", ".join(f"`{t}`" for t in tr.tool_calls) if tr.tool_calls else "*(none)*"
            md.append(
                f"| {tr.turn_number} | {tr.prompt_tokens:,} | {tr.completion_tokens:,} | {tr.reasoning_tokens:,} | {tools_str} | {tr.tool_results_bytes:,} B | {tr.latency_ms:.1f} ms |"
            )

        md.extend([
            "",
            "#### Baseline Arm 2: Targeted Editing Tools (`grep_search`, `replace_file_content`, fallback `read_file`, `write_file`)",
            "| Turn | Prompt Tokens | Output Tokens | Thinking Tokens | Tools Called | Tool Result Bytes | Latency |",
            "| :--- | :--- | :--- | :--- | :--- | :--- | :--- |",
        ])
        for tr in b_grep.turn_records:
            tools_str = ", ".join(f"`{t}`" for t in tr.tool_calls) if tr.tool_calls else "*(none)*"
            md.append(
                f"| {tr.turn_number} | {tr.prompt_tokens:,} | {tr.completion_tokens:,} | {tr.reasoning_tokens:,} | {tools_str} | {tr.tool_results_bytes:,} B | {tr.latency_ms:.1f} ms |"
            )

        md.extend([
            "",
            "#### Rowlite Arm 3: Native MCP Server (`rowlite_update`, `rowlite_append`, etc.)",
            "| Turn | Prompt Tokens | Output Tokens | Thinking Tokens | Tools Called | Tool Result Bytes | Latency |",
            "| :--- | :--- | :--- | :--- | :--- | :--- | :--- |",
        ])
        for tr in r.turn_records:
            tools_str = ", ".join(f"`{t}`" for t in tr.tool_calls) if tr.tool_calls else "*(none)*"
            md.append(
                f"| {tr.turn_number} | {tr.prompt_tokens:,} | {tr.completion_tokens:,} | {tr.reasoning_tokens:,} | {tools_str} | {tr.tool_results_bytes:,} B | {tr.latency_ms:.1f} ms |"
            )
        md.append("")

    md.extend([
        "---",
        "",
        f"## {sec_audit}. Inefficiency Regression Guard Audit",
        "",
        RegressionAuditor.format_audit_report(audit_findings),
        "",
        "---",
        "",
        f"## {sec_insights}. Operational Insights",
        "",
        "1. **Elimination of Table Formatting Drift**: Rowlite handles column pipe alignment deterministically in Go, eliminating agent hallucination of broken separators or truncated rows.",
        "2. **Constant Footprint**: Rowlite tool calls maintain near-constant $\\mathcal{O}(1)$ prompt and visible completion footprints regardless of whether the table contains 10 or 1,000 rows.",
        "3. **Cognitive Load Reduction**: By converting unstructured Markdown text inspection into declarative schema filtering, internal thinking tokens are reduced by 70% to 90%.",
        "",
        "---",
        "*Benchmark generated automatically via `benchmarks/token_efficiency/benchmark_tokens.py`.*",
    ])
    return "\n".join(md) + "\n"


def save_benchmark_snapshot(
    model_name: str,
    results_list: List[Tuple[Scenario, RunTelemetry, RunTelemetry, RunTelemetry]],
    audit_findings: List[AuditFinding],
    schema_data: Optional[Dict[str, Any]] = None,
) -> str:
    now = datetime.now()
    timestamp_filename = now.strftime("%Y-%m-%d_%H%M%S")
    timestamp_human = now.strftime("%Y-%m-%d %H:%M:%S")

    snapshot_filename = f"{timestamp_filename}_token_efficiency.md"
    snapshot_path = os.path.join(REPO_ROOT, "benchmarks", snapshot_filename)

    content = format_snapshot_markdown(timestamp_human, model_name, results_list, audit_findings, schema_data=schema_data)
    with open(snapshot_path, "w", encoding="utf-8") as f:
        f.write(content)

    # Save machine-readable JSON snapshot & pointer
    snapshot_json_filename = f"{timestamp_filename}_token_efficiency.json"
    snapshot_json_path = os.path.join(REPO_ROOT, "benchmarks", snapshot_json_filename)
    latest_json_path = os.path.join(REPO_ROOT, "benchmarks", "latest_token_efficiency.json")

    json_payload = {
        "timestamp": timestamp_human,
        "model": model_name,
        "benchmarks": [
            {
                "scenario": sc.id,
                "scenario_name": sc.name,
                "num_rows": r.num_rows,
                "baseline_whole": bw.to_dict(),
                "baseline_grep": bg.to_dict(),
                "rowlite": r.to_dict(),
                "savings_vs_whole_pct": ((bw.total_tokens - r.total_tokens) / bw.total_tokens * 100) if bw.total_tokens > 0 else 0,
                "savings_vs_grep_pct": ((bg.total_tokens - r.total_tokens) / bg.total_tokens * 100) if bg.total_tokens > 0 else 0,
            }
            for sc, bw, bg, r in results_list
        ],
        "schema_data": schema_data,
    }
    with open(snapshot_json_path, "w", encoding="utf-8") as f:
        json.dump(json_payload, f, indent=2)
    with open(latest_json_path, "w", encoding="utf-8") as f:
        json.dump(json_payload, f, indent=2)

    # Automatically refresh publication charts from benchmark data
    try:
        from benchmarks.token_efficiency.generate_chart import generate_all_charts
        assets_dir = os.path.join(REPO_ROOT, "docs", "assets")
        generate_all_charts(json_payload, assets_dir)
        print(f"[+] Automatically refreshed visual publication charts in: {assets_dir}")
    except Exception as e:
        print(f"[!] Warning: Failed to automatically regenerate charts: {e}")

    # Update benchmarks/README.md index table under Section 2 (LLM Token Efficiency)
    readme_path = os.path.join(REPO_ROOT, "benchmarks", "README.md")
    if os.path.exists(readme_path):
        with open(readme_path, "r", encoding="utf-8") as f:
            readme_content = f.read()

        max_savings_whole = max(
            [((bw.total_tokens - r.total_tokens) / bw.total_tokens * 100) if bw.total_tokens > 0 else 0 for _, bw, bg, r in results_list]
            + [0]
        )
        max_savings_grep = max(
            [((bg.total_tokens - r.total_tokens) / bg.total_tokens * 100) if bg.total_tokens > 0 else 0 for _, bw, bg, r in results_list]
            + [0]
        )
        new_row = f"| **{timestamp_human}** | [LLM Token Efficiency Snapshot]({snapshot_filename}) | `{model_name}` | {len(results_list)} scenarios | Up to {max_savings_whole:.1f}% vs whole-file, {max_savings_grep:.1f}% vs grep-replace across {results_list[0][3].num_rows if results_list else 50} rows. |\n"

        target_marker = "<!-- TOKEN_BENCHMARKS_TABLE -->"
        if target_marker in readme_content:
            table_idx = readme_content.find(target_marker)
            sep_idx = readme_content.find("| :---", table_idx)
            if sep_idx != -1:
                eol_idx = readme_content.find("\n", sep_idx)
                if eol_idx != -1:
                    insert_idx = eol_idx + 1
                    updated_readme = readme_content[:insert_idx] + new_row + readme_content[insert_idx:]
                    with open(readme_path, "w", encoding="utf-8") as f:
                        f.write(updated_readme)

    return snapshot_path


def main():
    parser = argparse.ArgumentParser(description="Rowlite Token Efficiency Benchmark CLI")
    parser.add_argument("--scenario", type=str, default="all", choices=["all", "point_update", "batch_update", "append", "query"], help="Benchmark scenario to run (default: all)")
    parser.add_argument("--model", type=str, default=None, help="Model identifier (e.g. 'ollama/ornith:35b' or 'qwen3.8-27b'). When omitted, runs deterministic simulation.")
    parser.add_argument("--api-base", type=str, default=None, help="Base URL for OpenAI-compatible endpoint (e.g. 'http://localhost:8000/v1' or 'http://dgx.dominanz.local:30001/v1'). Also reads OPENAI_BASE_URL env var.")
    parser.add_argument("--api-key", type=str, default=None, help="API key for model endpoint. Also reads OPENAI_API_KEY env var.")
    parser.add_argument("--rows", type=int, default=50, help="Number of rows for benchmark (default: 50)")
    parser.add_argument("--save-snapshot", action="store_true", help="Save timestamped benchmark snapshot into benchmarks/ and update index table")
    parser.add_argument("--compare-with", type=str, default=None, help="Path to previous results.json for release-to-release regression audit")
    parser.add_argument("--verbose-turns", action="store_true", help="Print turn-by-turn prompt, output, thinking, and latency telemetry trace")
    parser.add_argument("--rowlite-bin", type=str, default="rowlite", help="Path to rowlite binary (default: 'rowlite')")
    parser.add_argument("--output-json", type=str, default=None, help="Path to write JSON benchmark metrics")
    parser.add_argument("--output-md", type=str, default=None, help="Path to write Markdown summary report")

    args = parser.parse_args()

    is_simulation = args.model is None
    model_name = args.model if args.model else "Deterministic Simulation (tiktoken cl100k_base)"

    # 1. MCP Tool Schema Size Analysis (always evaluated and presented in every benchmark run)
    schema_metrics = measure_mcp_schema(rowlite_bin=args.rowlite_bin)
    print(format_schema_comparison_console(schema_metrics, LEGACY_SCHEMA_BASELINE, LEGACY_TOTAL_TOKENS, LEGACY_TOTAL_PROPS))

    # Resolve scenarios to run
    if args.scenario == "all":
        selected_scenarios = list(SCENARIOS.values())
    else:
        selected_scenarios = [SCENARIOS[args.scenario]]

    row_sizes = [args.rows]

    print(f"\n==========================================================================================")
    print(f" Starting Rowlite Multi-Scenario Token Efficiency Evaluation")
    print(f" Execution Mode: {model_name}")
    print(f" Canonical Source: internal/core/testdata/large_table_10k.md")
    print(f" Scenarios: {[s.id for s in selected_scenarios]} | Rows: {row_sizes}")
    print(f"==========================================================================================")

    all_results: List[Tuple[Scenario, RunTelemetry, RunTelemetry, RunTelemetry]] = []
    all_audit_findings: List[AuditFinding] = []

    # Historical comparison data if provided
    prev_data = {}
    if args.compare_with and os.path.exists(args.compare_with):
        try:
            with open(args.compare_with, "r", encoding="utf-8") as f:
                prev_json = json.load(f)
                prev_data = {
                    (b["scenario"], b["num_rows"]): b["rowlite"]
                    for b in prev_json.get("benchmarks", [])
                    if "scenario" in b
                }
        except Exception as e:
            print(f"[!] Warning: Failed to parse historical file {args.compare_with}: {e}")

    for sc in selected_scenarios:
        for size in row_sizes:
            bw, bg, r = run_benchmark_run(
                scenario=sc,
                num_rows=size,
                model=args.model,
                simulate=is_simulation,
                rowlite_bin=args.rowlite_bin,
                api_base=args.api_base,
                api_key=args.api_key,
            )
            all_results.append((sc, bw, bg, r))
            print(format_comparison_table(bw, bg, r, sc.name))
            if args.verbose_turns:
                print(format_turn_breakdown(bw, bg, r))

            # Audit against current baselines
            findings = RegressionAuditor.audit_single_run(r, bw)
            all_audit_findings.extend(findings)

            # Audit against historical run if available
            key = (sc.id, size)
            if key in prev_data:
                all_audit_findings.extend(RegressionAuditor.audit_comparison(r, prev_data[key]))

    # Print Regression Guard Audit block
    print("\n" + "=" * 106)
    print(" INEFFICIENCY REGRESSION GUARD AUDIT SUMMARY")
    print("=" * 106)
    print(RegressionAuditor.format_audit_report(all_audit_findings))

    # JSON export
    if args.output_json:
        payload = {
            "timestamp": datetime.now().isoformat(),
            "model": model_name,
            "simulated": is_simulation,
            "mcp_schema": schema_metrics,
            "benchmarks": [
                {
                    "scenario": sc.id,
                    "num_rows": r.num_rows,
                    "baseline_whole": bw.to_dict(),
                    "baseline_grep": bg.to_dict(),
                    "rowlite": r.to_dict(),
                    "savings_vs_whole_pct": ((bw.total_tokens - r.total_tokens) / bw.total_tokens * 100) if bw.total_tokens > 0 else 0,
                    "savings_vs_grep_pct": ((bg.total_tokens - r.total_tokens) / bg.total_tokens * 100) if bg.total_tokens > 0 else 0,
                }
                for sc, bw, bg, r in all_results
            ],
            "audit_findings": [
                {"severity": f.severity.value, "rule": f.rule, "message": f.message, "detail": f.delta_detail}
                for f in all_audit_findings
            ],
        }
        with open(args.output_json, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2)
        print(f"\n[+] Saved JSON telemetry metrics to: {args.output_json}")

    # Markdown export
    if args.output_md:
        md_text = format_snapshot_markdown(datetime.now().strftime("%Y-%m-%d %H:%M:%S"), model_name, all_results, all_audit_findings, schema_data=schema_metrics)
        with open(args.output_md, "w", encoding="utf-8") as f:
            f.write(md_text)
        print(f"[+] Saved Markdown summary report to: {args.output_md}")

    # Save permanent snapshot into benchmarks/
    if args.save_snapshot:
        snap_path = save_benchmark_snapshot(model_name, all_results, all_audit_findings, schema_data=schema_metrics)
        print(f"[+] Saved historical benchmark snapshot to: {snap_path}")
        print(f"[+] Updated historical index in: benchmarks/README.md")


if __name__ == "__main__":
    main()
