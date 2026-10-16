#!/usr/bin/env python3
"""
Generates publication-grade benchmark visualizations comparing:
  1. Baseline Whole-File Editing (read_file + write_file)
  2. Targeted Editing (grep_search + replace_file_content)
  3. Rowlite Native MCP Server (rowlite_update, rowlite_append, etc.)

Loads benchmark data directly from benchmark snapshot JSON files (e.g. benchmarks/latest_token_efficiency.json)
eliminating manual array editing.

Outputs:
  - docs/assets/token-efficiency-tokens.png   (Dedicated Token Footprint chart)
  - docs/assets/token-efficiency-latency.png  (Dedicated Wall-Clock Latency chart)
  - docs/assets/token-efficiency-benchmark.png (Combined 2-Panel overview chart)
"""

import argparse
import glob
import json
import os
import sys
from typing import Any, Dict, List, Optional
import matplotlib.pyplot as plt
import numpy as np


REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))


def setup_matplotlib():
    plt.rcParams["font.sans-serif"] = ["Segoe UI", "DejaVu Sans", "Helvetica", "Arial"]
    plt.rcParams["font.family"] = "sans-serif"


def load_benchmark_data(input_path: Optional[str] = None) -> Dict[str, Any]:
    """
    Loads benchmark payload from specified file or discovers the latest snapshot JSON.
    """
    resolved_path = None
    if input_path and os.path.exists(input_path):
        resolved_path = input_path
    else:
        latest_file = os.path.join(REPO_ROOT, "benchmarks", "latest_token_efficiency.json")
        if os.path.exists(latest_file):
            resolved_path = latest_file
        else:
            json_files = sorted(glob.glob(os.path.join(REPO_ROOT, "benchmarks", "*_token_efficiency.json")))
            if json_files:
                resolved_path = json_files[-1]

    if not resolved_path or not os.path.exists(resolved_path):
        raise FileNotFoundError("No benchmark JSON found in benchmarks/. Please run benchmark_tokens.py --save-snapshot first.")

    with open(resolved_path, "r", encoding="utf-8") as f:
        data = json.load(f)
    print(f"[*] Loaded benchmark data from: {resolved_path}")
    return data


def extract_chart_vectors(data: Dict[str, Any]):
    benchmarks = data.get("benchmarks", [])
    if not benchmarks:
        raise ValueError("Invalid benchmark JSON: 'benchmarks' list is empty.")

    scenarios = []
    tokens_whole = []
    tokens_grep = []
    tokens_rowlite = []

    latency_whole = []
    latency_grep = []
    latency_rowlite = []

    oom_whole = []
    savings_vs_grep = []

    name_map = {
        "point_update": "Point Update\n(by Key)",
        "batch_update": "Batch Update\n(Multi-Row)",
        "append": "Auto-Increment\nAppend",
        "query": "Bounded Slicing\nQuery",
    }

    for b in benchmarks:
        sc_id = b.get("scenario", "")
        sc_name = name_map.get(sc_id) or b.get("scenario_name") or sc_id.replace("_", " ").title()
        scenarios.append(sc_name)

        bw = b.get("baseline_whole", {})
        bg = b.get("baseline_grep", {})
        r = b.get("rowlite", {})

        tw = bw.get("total_tokens", 0)
        tg = bg.get("total_tokens", 0)
        tr = r.get("total_tokens", 0)

        tokens_whole.append(tw)
        tokens_grep.append(tg)
        tokens_rowlite.append(tr)

        lw = bw.get("wall_clock_seconds", 0.0)
        lg = bg.get("wall_clock_seconds", 0.0)
        lr = r.get("wall_clock_seconds", 0.0)

        latency_whole.append(round(lw, 1))
        latency_grep.append(round(lg, 1))
        latency_rowlite.append(round(lr, 1))

        is_oom = (not bw.get("verified", True)) or ("oom" in str(bw.get("final_output", "")).lower())
        oom_whole.append(is_oom)

        # Savings percentage vs grep
        if tg > 0:
            pct = (tg - tr) / tg * 100
            savings_vs_grep.append(f"-{pct:.1f}%")
        else:
            savings_vs_grep.append("0.0%")

    return {
        "model": data.get("model", "qwen3.8-27b"),
        "scenarios": scenarios,
        "tokens_whole": tokens_whole,
        "tokens_grep": tokens_grep,
        "tokens_rowlite": tokens_rowlite,
        "latency_whole": latency_whole,
        "latency_grep": latency_grep,
        "latency_rowlite": latency_rowlite,
        "oom_whole": oom_whole,
        "savings_vs_grep": savings_vs_grep,
    }


def generate_tokens_chart(vectors: Dict[str, Any], output_path: str):
    scenarios = vectors["scenarios"]
    tokens_whole = vectors["tokens_whole"]
    tokens_grep = vectors["tokens_grep"]
    tokens_rowlite = vectors["tokens_rowlite"]
    oom_whole = vectors["oom_whole"]
    savings_vs_grep = vectors["savings_vs_grep"]

    fig, ax = plt.subplots(figsize=(9, 5.5), dpi=300)
    x = np.arange(len(scenarios))
    width = 0.25

    c_whole = "#f43f5e"     # Coral Red
    c_grep = "#64748b"      # Slate Gray
    c_rowlite = "#059669"   # Emerald Green

    ax.bar(x - width, tokens_whole, width, label="Whole-File (read/write)", color=c_whole, alpha=0.85, edgecolor="#be123c", linewidth=1.2)
    ax.bar(x, tokens_grep, width, label="Targeted (grep/replace)", color=c_grep, alpha=0.9, edgecolor="#334155", linewidth=1.2)
    ax.bar(x + width, tokens_rowlite, width, label="Rowlite Native MCP", color=c_rowlite, alpha=0.95, edgecolor="#047857", linewidth=1.2)

    # Annotate Whole-File (OOM vs normal)
    for i in range(len(scenarios)):
        if oom_whole[i]:
            ax.text(x[i] - width, tokens_whole[i] + 250, "OOM\nCrash", ha="center", va="bottom", fontsize=8.5, color="#be123c", fontweight="bold")
        else:
            ax.text(x[i] - width, tokens_whole[i] + 150, f"{tokens_whole[i]:,}", ha="center", va="bottom", fontsize=8.5, color="#334155")

    # Annotate Grep values
    for i in range(len(scenarios)):
        ax.text(x[i], tokens_grep[i] + 150, f"{tokens_grep[i]:,}", ha="center", va="bottom", fontsize=8.5, color="#334155")

    # Annotate Rowlite values & Savings badges
    for i in range(len(scenarios)):
        ax.text(x[i] + width, tokens_rowlite[i] + 150, f"{tokens_rowlite[i]:,}", ha="center", va="bottom", fontsize=8.5, fontweight="bold", color="#047857")
        if i < len(scenarios) - 1:
            ax.text(x[i] + width, tokens_rowlite[i] / 2, f"{savings_vs_grep[i]}\nvs grep", ha="center", va="center", fontsize=8, fontweight="bold", color="white")

    max_tok = max(max(tokens_whole), max(tokens_grep), max(tokens_rowlite))
    ax.set_ylabel("Total Trajectory Tokens (Prompt + Completion)", fontsize=11, fontweight="600", color="#1e293b")
    ax.set_title(f"AI Agent Token Efficiency: Markdown Table Manipulation\nTotal Trajectory Tokens on 50 Rows ({vectors['model']} on NVIDIA DGX Spark)", fontsize=12, fontweight="bold", color="#0f172a", pad=14)
    ax.set_xticks(x)
    ax.set_xticklabels(scenarios, fontsize=10, color="#1e293b")
    ax.set_ylim(0, max_tok * 1.20)
    ax.legend(loc="upper right", frameon=True, facecolor="#f8fafc", edgecolor="#cbd5e1", fontsize=9.5)
    ax.grid(axis="y", linestyle="--", alpha=0.3, color="#94a3b8")
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)

    plt.tight_layout()
    plt.savefig(output_path, dpi=300)
    plt.close()
    print(f"[+] Saved tokens chart: {output_path}")


def generate_latency_chart(vectors: Dict[str, Any], output_path: str):
    scenarios = vectors["scenarios"]
    latency_whole = vectors["latency_whole"]
    latency_grep = vectors["latency_grep"]
    latency_rowlite = vectors["latency_rowlite"]
    oom_whole = vectors["oom_whole"]

    fig, ax = plt.subplots(figsize=(9, 5.5), dpi=300)
    x = np.arange(len(scenarios))
    width = 0.25

    c_whole = "#f43f5e"     # Coral Red
    c_grep = "#64748b"      # Slate Gray
    c_rowlite = "#059669"   # Emerald Green

    ax.bar(x - width, latency_whole, width, label="Whole-File (read/write)", color=c_whole, alpha=0.85, edgecolor="#be123c", linewidth=1.2)
    ax.bar(x, latency_grep, width, label="Targeted (grep/replace)", color=c_grep, alpha=0.9, edgecolor="#334155", linewidth=1.2)
    ax.bar(x + width, latency_rowlite, width, label="Rowlite Native MCP", color=c_rowlite, alpha=0.95, edgecolor="#047857", linewidth=1.2)

    for i in range(len(scenarios)):
        if oom_whole[i]:
            ax.text(x[i] - width, latency_whole[i] + 0.6, "30s Hang\n(OOM)", ha="center", va="bottom", fontsize=8.5, color="#be123c", fontweight="bold")
        else:
            ax.text(x[i] - width, latency_whole[i] + 0.6, f"{latency_whole[i]:.1f}s", ha="center", va="bottom", fontsize=8.5, color="#334155")
        ax.text(x[i], latency_grep[i] + 0.6, f"{latency_grep[i]:.1f}s", ha="center", va="bottom", fontsize=8.5, color="#334155")
        ax.text(x[i] + width, latency_rowlite[i] + 0.6, f"{latency_rowlite[i]:.1f}s", ha="center", va="bottom", fontsize=8.5, fontweight="bold", color="#047857")

    # Dynamic Speedup badge on batch update (if scenario index 1 exists)
    if len(scenarios) > 1 and latency_rowlite[1] > 0 and latency_grep[1] > latency_rowlite[1]:
        speedup = latency_grep[1] / latency_rowlite[1]
        ax.text(x[1] + width, latency_rowlite[1] + 3.0, f"{speedup:.1f}x Faster\nvs Grep", ha="center", va="bottom", fontsize=8.5, fontweight="bold", color="#047857",
                bbox=dict(boxstyle="round,pad=0.25", facecolor="#d1fae5", edgecolor="#10b981", alpha=0.9))

    max_lat = max(max(latency_whole), max(latency_grep), max(latency_rowlite))
    ax.set_ylabel("Execution Latency (Seconds)", fontsize=11, fontweight="600", color="#1e293b")
    ax.set_title(f"AI Agent Execution Latency: Markdown Table Manipulation\nWall-Clock Turn Latency on 50 Rows ({vectors['model']} on NVIDIA DGX Spark)", fontsize=12, fontweight="bold", color="#0f172a", pad=14)
    ax.set_xticks(x)
    ax.set_xticklabels(scenarios, fontsize=10, color="#1e293b")
    ax.set_ylim(0, max_lat * 1.25)
    ax.legend(loc="upper right", frameon=True, facecolor="#f8fafc", edgecolor="#cbd5e1", fontsize=9.5)
    ax.grid(axis="y", linestyle="--", alpha=0.3, color="#94a3b8")
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)

    plt.tight_layout()
    plt.savefig(output_path, dpi=300)
    plt.close()
    print(f"[+] Saved latency chart: {output_path}")


def generate_combined_chart(vectors: Dict[str, Any], output_path: str):
    scenarios = vectors["scenarios"]
    tokens_whole = vectors["tokens_whole"]
    tokens_grep = vectors["tokens_grep"]
    tokens_rowlite = vectors["tokens_rowlite"]
    latency_whole = vectors["latency_whole"]
    latency_grep = vectors["latency_grep"]
    latency_rowlite = vectors["latency_rowlite"]
    oom_whole = vectors["oom_whole"]
    savings_vs_grep = vectors["savings_vs_grep"]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 6.5), dpi=300)
    x = np.arange(len(scenarios))
    width = 0.26

    c_whole = "#f43f5e"     # Coral Red
    c_grep = "#64748b"      # Slate Gray
    c_rowlite = "#059669"   # Emerald Green

    # Panel 1: Tokens
    ax1.bar(x - width, tokens_whole, width, label="Whole-File (read/write)", color=c_whole, alpha=0.85, edgecolor="#be123c", linewidth=1.2)
    ax1.bar(x, tokens_grep, width, label="Targeted (grep/replace)", color=c_grep, alpha=0.9, edgecolor="#334155", linewidth=1.2)
    ax1.bar(x + width, tokens_rowlite, width, label="Rowlite Native MCP", color=c_rowlite, alpha=0.95, edgecolor="#047857", linewidth=1.2)

    for i in range(len(scenarios)):
        if oom_whole[i]:
            ax1.text(x[i] - width, tokens_whole[i] + 250, "OOM\nCrash", ha="center", va="bottom", fontsize=8.5, color="#be123c", fontweight="bold")
        else:
            ax1.text(x[i] - width, tokens_whole[i] + 150, f"{tokens_whole[i]:,}", ha="center", va="bottom", fontsize=8.5, color="#334155")
        ax1.text(x[i], tokens_grep[i] + 150, f"{tokens_grep[i]:,}", ha="center", va="bottom", fontsize=8.5, color="#334155")
        ax1.text(x[i] + width, tokens_rowlite[i] + 150, f"{tokens_rowlite[i]:,}", ha="center", va="bottom", fontsize=8.5, fontweight="bold", color="#047857")
        if i < len(scenarios) - 1:
            ax1.text(x[i] + width, tokens_rowlite[i] / 2, f"{savings_vs_grep[i]}\nvs grep", ha="center", va="center", fontsize=8, fontweight="bold", color="white")

    max_tok = max(max(tokens_whole), max(tokens_grep), max(tokens_rowlite))
    ax1.set_ylabel("Total Trajectory Tokens (Prompt + Completion)", fontsize=11, fontweight="600", color="#1e293b")
    ax1.set_title("A. Context Token Consumption (50 Rows)", fontsize=13, fontweight="bold", color="#0f172a", pad=12)
    ax1.set_xticks(x)
    ax1.set_xticklabels(scenarios, fontsize=10, color="#1e293b")
    ax1.set_ylim(0, max_tok * 1.20)
    ax1.legend(loc="upper right", frameon=True, facecolor="#f8fafc", edgecolor="#cbd5e1", fontsize=9.5)
    ax1.grid(axis="y", linestyle="--", alpha=0.3, color="#94a3b8")
    ax1.spines["top"].set_visible(False)
    ax1.spines["right"].set_visible(False)

    # Panel 2: Latency
    ax2.bar(x - width, latency_whole, width, label="Whole-File (read/write)", color=c_whole, alpha=0.85, edgecolor="#be123c", linewidth=1.2)
    ax2.bar(x, latency_grep, width, label="Targeted (grep/replace)", color=c_grep, alpha=0.9, edgecolor="#334155", linewidth=1.2)
    ax2.bar(x + width, latency_rowlite, width, label="Rowlite Native MCP", color=c_rowlite, alpha=0.95, edgecolor="#047857", linewidth=1.2)

    for i in range(len(scenarios)):
        if oom_whole[i]:
            ax2.text(x[i] - width, latency_whole[i] + 0.6, "30s Hang\n(OOM)", ha="center", va="bottom", fontsize=8.5, color="#be123c", fontweight="bold")
        else:
            ax2.text(x[i] - width, latency_whole[i] + 0.6, f"{latency_whole[i]:.1f}s", ha="center", va="bottom", fontsize=8.5, color="#334155")
        ax2.text(x[i], latency_grep[i] + 0.6, f"{latency_grep[i]:.1f}s", ha="center", va="bottom", fontsize=8.5, color="#334155")
        ax2.text(x[i] + width, latency_rowlite[i] + 0.6, f"{latency_rowlite[i]:.1f}s", ha="center", va="bottom", fontsize=8.5, fontweight="bold", color="#047857")

    if len(scenarios) > 1 and latency_rowlite[1] > 0 and latency_grep[1] > latency_rowlite[1]:
        speedup = latency_grep[1] / latency_rowlite[1]
        ax2.text(x[1] + width, latency_rowlite[1] + 3.0, f"{speedup:.1f}x Faster\nvs Grep", ha="center", va="bottom", fontsize=8.5, fontweight="bold", color="#047857",
                bbox=dict(boxstyle="round,pad=0.25", facecolor="#d1fae5", edgecolor="#10b981", alpha=0.9))

    max_lat = max(max(latency_whole), max(latency_grep), max(latency_rowlite))
    ax2.set_ylabel("Execution Latency (Seconds)", fontsize=11, fontweight="600", color="#1e293b")
    ax2.set_title("B. Agent Turn Latency (Seconds)", fontsize=13, fontweight="bold", color="#0f172a", pad=12)
    ax2.set_xticks(x)
    ax2.set_xticklabels(scenarios, fontsize=10, color="#1e293b")
    ax2.set_ylim(0, max_lat * 1.25)
    ax2.legend(loc="upper right", frameon=True, facecolor="#f8fafc", edgecolor="#cbd5e1", fontsize=9.5)
    ax2.grid(axis="y", linestyle="--", alpha=0.3, color="#94a3b8")
    ax2.spines["top"].set_visible(False)
    ax2.spines["right"].set_visible(False)

    plt.suptitle(f"Rowlite vs Filesystem Baselines: AI Agent Efficiency on Markdown Tables\nEmpirically evaluated on 50 rows using {vectors['model']} (NVIDIA DGX Spark)", fontsize=14, fontweight="bold", color="#0f172a", y=0.99)
    plt.tight_layout()
    plt.subplots_adjust(top=0.88)

    plt.savefig(output_path, dpi=300)
    plt.close()
    print(f"[+] Saved combined chart: {output_path}")


def generate_all_charts(data: Dict[str, Any], output_dir: str):
    setup_matplotlib()
    os.makedirs(output_dir, exist_ok=True)
    vectors = extract_chart_vectors(data)
    generate_tokens_chart(vectors, os.path.join(output_dir, "token-efficiency-tokens.png"))
    generate_latency_chart(vectors, os.path.join(output_dir, "token-efficiency-latency.png"))
    generate_combined_chart(vectors, os.path.join(output_dir, "token-efficiency-benchmark.png"))


def main():
    parser = argparse.ArgumentParser(description="Generate publication-grade benchmark visualization charts")
    parser.add_argument("--input", type=str, default=None, help="Path to benchmark JSON snapshot file")
    parser.add_argument("--output-dir", type=str, default=None, help="Directory to save generated chart PNGs")
    args = parser.parse_args()

    data = load_benchmark_data(args.input)
    output_dir = args.output_dir if args.output_dir else os.path.join(REPO_ROOT, "docs", "assets")
    generate_all_charts(data, output_dir)


if __name__ == "__main__":
    main()
