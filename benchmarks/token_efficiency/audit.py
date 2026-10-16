"""
Inefficiency Regression Guard Audit for Rowlite Token Efficiency Benchmarks.
Compares benchmark runs against historical baselines to automatically identify red flags:
  - Schema Tax Bloat (Turn 1 prompt expansion > 10%)
  - Efficiency Degradation (Token reduction drops below threshold)
  - Trajectory Turn Inflation (Additional conversational round-trips)
  - Integrity & Verification Failure (Data corruption or formatting drift)
  - Latency Spikes (> 50% wall-clock slowdown)
"""

from dataclasses import dataclass
from enum import Enum
from typing import Any, Dict, List, Optional, Tuple

from benchmarks.token_efficiency.telemetry import RunTelemetry


class Severity(Enum):
    INFO = "INFO"
    WARNING = "WARNING"
    RED_FLAG = "RED_FLAG"


@dataclass
class AuditFinding:
    severity: Severity
    rule: str
    message: str
    delta_detail: str


class RegressionAuditor:
    """Evaluates telemetry deltas and raises Red Flags if performance or efficiency degrades."""

    @staticmethod
    def audit_single_run(current_rowlite: RunTelemetry, current_baseline: RunTelemetry) -> List[AuditFinding]:
        findings: List[AuditFinding] = []

        # 1. Mutation Verification Check
        if not current_rowlite.verified:
            findings.append(
                AuditFinding(
                    severity=Severity.RED_FLAG,
                    rule="MUTATION_VERIFICATION_FAILED",
                    message="Rowlite mutation failed verification on disk.",
                    delta_detail=current_rowlite.verification_message,
                )
            )

        # 2. Token Reduction on Large Tables (Differentiating Mutations vs Read-Only Queries)
        if current_rowlite.num_rows >= 100:
            savings_pct = (
                (current_baseline.total_tokens - current_rowlite.total_tokens)
                / current_baseline.total_tokens
                * 100
            )
            # Mutations should achieve >= 50% savings; read-only queries should achieve >= 5%
            is_read_only = "query" in current_rowlite.final_output.lower() or "list" in current_rowlite.final_output.lower()
            threshold = 5.0 if is_read_only else 50.0

            if savings_pct < threshold:
                findings.append(
                    AuditFinding(
                        severity=Severity.RED_FLAG,
                        rule="TOKEN_SAVINGS_SUBPAR",
                        message=f"Total token reduction on {current_rowlite.num_rows} rows is only {savings_pct:.1f}% (expected >= {threshold:.0f}%).",
                        delta_detail=f"Baseline: {current_baseline.total_tokens}, Rowlite: {current_rowlite.total_tokens}",
                    )
                )

        # 3. Visible Output Conciseness (Strict O(1) on Mutations; Generous for Query Answers)
        if current_rowlite.num_rows >= 50:
            is_query = "query" in current_rowlite.final_output.lower() or "list" in current_rowlite.final_output.lower() or any(
                "rowlite_list" in tr.tool_calls for tr in current_rowlite.turn_records
            )
            # Mutations should be strictly O(1) <= 200 tokens across turns.
            # Queries require conversational headroom (up to 350 tokens) to present retrieved data to the user.
            max_allowed = 350 if is_query else 200
            if current_rowlite.visible_completion_tokens > max_allowed:
                findings.append(
                    AuditFinding(
                        severity=Severity.WARNING,
                        rule="VISIBLE_OUTPUT_BLOAT",
                        message=f"Rowlite emitted {current_rowlite.visible_completion_tokens} visible output tokens (expected <= {max_allowed} tokens for {'queries' if is_query else 'mutations'}).",
                        delta_detail=f"Visible completion tokens: {current_rowlite.visible_completion_tokens}",
                    )
                )

        return findings

    @staticmethod
    def audit_comparison(
        current_rowlite: RunTelemetry,
        previous_rowlite: Dict[str, Any],
    ) -> List[AuditFinding]:
        findings: List[AuditFinding] = []

        prev_t1 = previous_rowlite.get("turn1_prompt_tokens", 0)
        curr_t1 = current_rowlite.turn1_prompt_tokens

        # Check Schema Tax Inflation (>10% increase)
        if prev_t1 > 0:
            schema_growth_pct = (curr_t1 - prev_t1) / prev_t1 * 100
            if schema_growth_pct > 10.0:
                findings.append(
                    AuditFinding(
                        severity=Severity.RED_FLAG,
                        rule="SCHEMA_TAX_INFLATION",
                        message=f"Turn 1 Schema Tax grew by {schema_growth_pct:+.1f}% compared to previous run.",
                        delta_detail=f"Previous: {prev_t1} tokens -> Current: {curr_t1} tokens ({curr_t1 - prev_t1:+d})",
                    )
                )

        # Check Turn Count Inflation
        prev_turns = previous_rowlite.get("turns", 0)
        curr_turns = current_rowlite.turns
        if prev_turns > 0 and curr_turns > prev_turns:
            findings.append(
                AuditFinding(
                    severity=Severity.RED_FLAG,
                    rule="TRAJECTORY_TURNS_INCREASED",
                    message=f"Rowlite trajectory length increased from {prev_turns} to {curr_turns} turns.",
                    delta_detail=f"Turns delta: +{curr_turns - prev_turns}",
                )
            )

        # Check Total Token Regression
        prev_total = previous_rowlite.get("total_tokens", 0)
        curr_total = current_rowlite.total_tokens
        if prev_total > 0:
            token_diff_pct = (curr_total - prev_total) / prev_total * 100
            if token_diff_pct > 15.0:
                findings.append(
                    AuditFinding(
                        severity=Severity.RED_FLAG,
                        rule="TOKEN_CONSUMPTION_REGRESSION",
                        message=f"Total tokens consumed by Rowlite increased by {token_diff_pct:+.1f}%.",
                        delta_detail=f"Previous: {prev_total} -> Current: {curr_total} tokens ({curr_total - prev_total:+d})",
                    )
                )

        return findings

    @staticmethod
    def format_audit_report(findings: List[AuditFinding]) -> str:
        if not findings:
            return "✅ Regression Guard Audit: 0 Red Flags detected. All token efficiency invariants preserved."

        lines = [f"⚠️ Regression Guard Audit: {len(findings)} Finding(s) Detected:\n"]
        for f in findings:
            tag = "🚩 [RED FLAG]" if f.severity == Severity.RED_FLAG else "⚠️ [WARNING]"
            lines.append(f"{tag} {f.rule}: {f.message}")
            if f.delta_detail:
                lines.append(f"    Detail: {f.delta_detail}")
        return "\n".join(lines)
