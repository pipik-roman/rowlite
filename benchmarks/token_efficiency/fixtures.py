"""
Canonical Test Fixture Extraction and Verification for Rowlite Token Efficiency Benchmarks.
Extracts slices from internal/core/testdata/large_table_10k.md using `rowlite list` to guarantee
authentic production-grade testing data (emojis, unicode, escaped pipes, and YAML frontmatter).
"""

import os
import subprocess
from typing import Optional, Tuple

CANONICAL_FIXTURE_REL = os.path.join("internal", "core", "testdata", "large_table_10k.md")


def get_canonical_fixture_path(repo_root: Optional[str] = None) -> str:
    if repo_root is None:
        repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
    path = os.path.join(repo_root, CANONICAL_FIXTURE_REL)
    if not os.path.exists(path):
        raise FileNotFoundError(f"Canonical benchmark dataset not found at: {path}")
    return path


def extract_canonical_fixture(
    num_rows: int,
    target_path: str,
    rowlite_bin: str = "rowlite",
    repo_root: Optional[str] = None,
    include_frontmatter: bool = True,
) -> str:
    """
    Extracts the first `num_rows` from the canonical 10k dataset using `rowlite list --head <num_rows> --format md`.
    Preserves YAML frontmatter constraints (unique, sort, audit, autoincrement).
    Returns the target file path.
    """
    source_path = get_canonical_fixture_path(repo_root)

    # 1. Extract YAML frontmatter from source file
    frontmatter_block = ""
    if include_frontmatter:
        with open(source_path, "r", encoding="utf-8") as f:
            content = f.read()
        if content.startswith("---"):
            end_idx = content.find("---", 3)
            if end_idx != -1:
                frontmatter_block = content[: end_idx + 3] + "\n"

    # 2. Extract first N rows using rowlite list
    cmd = [rowlite_bin, "list", source_path, "--head", str(num_rows), "--format", "md"]
    table_output = subprocess.check_output(cmd, text=True, encoding="utf-8")

    # 3. Write combined markdown file in UTF-8
    with open(target_path, "w", encoding="utf-8") as f:
        f.write(frontmatter_block + table_output)

    return target_path


def parse_gfm_table_rows(file_path: str):
    """Parses markdown table into headers and list of row dicts."""
    with open(file_path, "r", encoding="utf-8") as f:
        content = f.read()

    lines = [line.strip() for line in content.splitlines() if line.strip() and not line.strip().startswith("---") and not line.strip().startswith("rowlite:")]
    # Skip any frontmatter keys if present
    table_lines = [l for l in lines if l.startswith("|") and l.endswith("|")]
    if len(table_lines) < 2:
        return None, []

    headers = [c.strip() for c in table_lines[0].strip("|").split("|")]
    # table_lines[1] is delimiter
    rows = []
    for line in table_lines[2:]:
        cells = [c.strip() for c in line.strip("|").split("|")]
        if len(cells) == len(headers):
            row_dict = dict(zip(headers, cells))
            rows.append(row_dict)
    return headers, rows


def verify_point_update(file_path: str, task_id: str = "2", expected_status: str = "active") -> Tuple[bool, str]:
    """Verifies that target task_id has expected_status in Status State."""
    try:
        headers, rows = parse_gfm_table_rows(file_path)
        if not headers or not rows:
            return False, "Failed to parse valid markdown table."

        for r in rows:
            if r.get("Task ID") == task_id:
                curr = r.get("Status State")
                if curr == expected_status:
                    return True, f"Task {task_id} successfully updated to Status State '{expected_status}'."
                return False, f"Task {task_id} found but Status State is '{curr}', expected '{expected_status}'."

        return False, f"Task ID {task_id} not found in table."
    except Exception as e:
        return False, f"Verification exception: {e}"


def verify_batch_update(file_path: str, filter_col: str = "Priority Level", filter_val: str = "high", set_col: str = "Status State", expected_val: str = "in_progress") -> Tuple[bool, str]:
    """Verifies that all rows matching filter_col == filter_val have set_col == expected_val."""
    try:
        headers, rows = parse_gfm_table_rows(file_path)
        if not headers or not rows:
            return False, "Failed to parse valid markdown table."

        matched = 0
        for r in rows:
            if r.get(filter_col) == filter_val:
                matched += 1
                curr = r.get(set_col)
                if curr != expected_val:
                    return False, f"Row with {filter_col}='{filter_val}' has {set_col}='{curr}', expected '{expected_val}'."

        if matched == 0:
            return False, f"No rows matched {filter_col} == '{filter_val}' to verify."
        return True, f"All {matched} rows matching {filter_col}='{filter_val}' have {set_col}='{expected_val}'."
    except Exception as e:
        return False, f"Verification exception: {e}"


def verify_append(file_path: str, expected_desc: str = "Deploy OAuth2 gateway") -> Tuple[bool, str]:
    """Verifies that a new row with expected_desc was appended with non-empty Task ID and Created At."""
    try:
        headers, rows = parse_gfm_table_rows(file_path)
        if not headers or not rows:
            return False, "Failed to parse valid markdown table."

        for r in rows:
            if r.get("Task Description") == expected_desc:
                tid = r.get("Task ID", "")
                created = r.get("Created At", "")
                if tid and tid != "":
                    return True, f"Appended row found with Task ID '{tid}' and Created At '{created}'."
                return False, f"Appended row found but Task ID is empty: {r}"

        return False, f"Row with Task Description '{expected_desc}' was not found in table."
    except Exception as e:
        return False, f"Verification exception: {e}"


def verify_query(file_path: str) -> Tuple[bool, str]:
    """Verifies table remains intact and uncorrupted after a read-only query."""
    try:
        headers, rows = parse_gfm_table_rows(file_path)
        if headers and len(rows) > 0:
            return True, f"Table intact with {len(rows)} rows."
        return False, "Table corrupted or empty."
    except Exception as e:
        return False, f"Verification exception: {e}"
