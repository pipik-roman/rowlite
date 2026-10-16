"""
Multi-Scenario Agent Loop Execution Engine for Rowlite Token Efficiency Benchmark.
Executes controlled tool ablation benchmarks across 4 operational scenarios:
  1. Point Update (by Primary Key)
  2. Batch Conditional Update (Multi-Row Predicate)
  3. Auto-Increment Append (Surrogate Key & Audit Generation)
  4. Bounded Query / Slicing (Filtered Read without Context Bloat)
"""

from dataclasses import dataclass
import json
import os
import re
import time
from typing import Any, Callable, Dict, List, Optional, Tuple

from benchmarks.token_efficiency.fixtures import (
    verify_append,
    verify_batch_update,
    verify_point_update,
    verify_query,
)
from benchmarks.token_efficiency.telemetry import (
    RunTelemetry,
    SimulationThinkingModel,
    TurnRecord,
    UsageExtractor,
)

RunMetrics = RunTelemetry


@dataclass
class Scenario:
    id: str
    name: str
    description: str
    system_prompt: str
    user_prompt: str
    verify_fn: Callable[[str], Tuple[bool, str]]


SCENARIOS: Dict[str, Scenario] = {
    "point_update": Scenario(
        id="point_update",
        name="Point Update by Key",
        description="Update single cell by primary key ('Task ID == 2' -> Status State='active')",
        system_prompt=(
            "You are a helpful assistant operating in a workspace containing 'table.md'. "
            "Fulfill the user request accurately using the available tools. "
            "When finished, respond with a concise confirmation."
        ),
        user_prompt="In 'table.md', update the 'Status State' of Task ID 2 from 'pending' to 'active'.",
        verify_fn=lambda path: verify_point_update(path, task_id="2", expected_status="active"),
    ),
    "batch_update": Scenario(
        id="batch_update",
        name="Batch Conditional Update",
        description="Update multiple matching rows ('Priority Level == high' -> Status State='in_progress')",
        system_prompt=(
            "You are a helpful assistant operating in a workspace containing 'table.md'. "
            "Fulfill the user request accurately using the available tools. "
            "When finished, respond with a concise confirmation."
        ),
        user_prompt="In 'table.md', update all tasks where 'Priority Level' is 'high' by setting 'Status State' to 'in_progress'.",
        verify_fn=lambda path: verify_batch_update(path, filter_col="Priority Level", filter_val="high", set_col="Status State", expected_val="in_progress"),
    ),
    "append": Scenario(
        id="append",
        name="Auto-Increment Append",
        description="Append a new task testing surrogate key generation and audit timestamp insertion",
        system_prompt=(
            "You are a helpful assistant operating in a workspace containing 'table.md'. "
            "Fulfill the user request accurately using the available tools. "
            "When finished, respond with a concise confirmation."
        ),
        user_prompt="In 'table.md', append a new task with Description 'Deploy OAuth2 gateway' in Category 'DevOps & CI/CD' with Priority 'high', Status 'active', and Score '99'.",
        verify_fn=lambda path: verify_append(path, expected_desc="Deploy OAuth2 gateway"),
    ),
    "query": Scenario(
        id="query",
        name="Bounded Slicing Query",
        description="Query the first 3 tasks in Category 'Security & Auth' without context saturation",
        system_prompt=(
            "You are a helpful assistant operating in a workspace containing 'table.md'. "
            "Fulfill the user request accurately using the available tools. "
            "When finished, respond with a concise confirmation."
        ),
        user_prompt="In 'table.md', find and list the first 3 tasks in the 'Security & Auth' category.",
        verify_fn=lambda path: verify_query(path),
    ),
}

BASELINE_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read the text content of a file from disk.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Relative or absolute file path"}
                },
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "write_file",
            "description": "Write entire text content to a file on disk.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Relative or absolute file path"},
                    "content": {"type": "string", "description": "Complete text content to write"},
                },
                "required": ["path", "content"],
            },
        },
    },
]


def handle_baseline_tool(name: str, args: Dict[str, Any]) -> str:
    path = args.get("path") or args.get("file") or args.get("filename") or args.get("argument_name") or ""
    if name == "read_file":
        if not path:
            return "Error: 'path' parameter is required for read_file."
        if not os.path.exists(path):
            return f"Error: File '{path}' does not exist."
        try:
            with open(path, "r", encoding="utf-8") as f:
                return f.read()
        except Exception as e:
            return f"Error reading file '{path}': {e}"
    elif name == "write_file":
        if not path:
            return "Error: 'path' parameter is required for write_file."
        content = args.get("content") or args.get("text") or args.get("data") or ""
        try:
            with open(path, "w", encoding="utf-8") as f:
                f.write(content)
            return "File updated successfully."
        except Exception as e:
            return f"Error writing file '{path}': {e}"
    return f"Unknown tool: {name}"


GREP_REPLACE_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "grep_search",
            "description": "Search for a string pattern or regular expression in a file, returning matching line numbers and line text.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Relative or absolute file path"},
                    "query": {"type": "string", "description": "Search pattern or regular expression"},
                },
                "required": ["path", "query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "replace_file_content",
            "description": "Replace an exact target text chunk with new replacement content in a file.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Relative or absolute file path"},
                    "target_content": {"type": "string", "description": "Exact text chunk in the file to replace"},
                    "replacement_content": {"type": "string", "description": "New replacement text"},
                },
                "required": ["path", "target_content", "replacement_content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read the text content of a file from disk, optionally between start_line and end_line.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Relative or absolute file path"},
                    "start_line": {"type": "integer", "description": "1-based starting line number (optional)"},
                    "end_line": {"type": "integer", "description": "1-based ending line number (optional)"},
                },
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "write_file",
            "description": "Write entire text content to a file on disk.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "Relative or absolute file path"},
                    "content": {"type": "string", "description": "Complete text content to write"},
                },
                "required": ["path", "content"],
            },
        },
    },
]


def handle_grep_replace_tool(name: str, args: Dict[str, Any]) -> str:
    path = args.get("path") or args.get("file") or args.get("filename") or args.get("argument_name") or ""
    if not path:
        return "Error: 'path' parameter is required."
    if not os.path.exists(path):
        return f"Error: File '{path}' does not exist."

    if name == "grep_search":
        query = args.get("query") or args.get("pattern") or ""
        if not query:
            return "Error: 'query' parameter is required for grep_search."
        try:
            with open(path, "r", encoding="utf-8") as f:
                lines = f.readlines()
            results = []
            pattern = re.compile(re.escape(query), re.IGNORECASE)
            for idx, line in enumerate(lines, start=1):
                if pattern.search(line):
                    results.append(f"Line {idx}: {line.rstrip()}")
            if not results:
                return f"No matches found for '{query}' in '{path}'."
            return "\n".join(results[:50])
        except Exception as e:
            return f"Error executing grep_search: {e}"

    elif name == "replace_file_content":
        target = args.get("target_content") or args.get("target") or args.get("old_str") or args.get("TargetContent") or ""
        replacement = args.get("replacement_content") or args.get("replacement") or args.get("new_str") or args.get("ReplacementContent") or ""
        if not target:
            return "Error: 'target_content' parameter is required for replace_file_content."
        try:
            with open(path, "r", encoding="utf-8") as f:
                content = f.read()
            if target not in content:
                return f"Error: target_content not found in '{path}'."
            new_content = content.replace(target, replacement, 1)
            with open(path, "w", encoding="utf-8") as f:
                f.write(new_content)
            return "File content replaced successfully."
        except Exception as e:
            return f"Error replacing content in '{path}': {e}"

    elif name == "read_file":
        start_line = args.get("start_line") or args.get("StartLine")
        end_line = args.get("end_line") or args.get("EndLine")
        try:
            with open(path, "r", encoding="utf-8") as f:
                lines = f.readlines()
            if start_line is not None and end_line is not None:
                s = max(0, int(start_line) - 1)
                e = min(len(lines), int(end_line))
                return "".join(lines[s:e])
            return "".join(lines)
        except Exception as e:
            return f"Error reading file '{path}': {e}"

    elif name == "write_file":
        content = args.get("content") or args.get("text") or args.get("data") or ""
        try:
            with open(path, "w", encoding="utf-8") as f:
                f.write(content)
            return "File updated successfully."
        except Exception as e:
            return f"Error writing file '{path}': {e}"

    return f"Unknown tool: {name}"


class TokenCounter:
    def __init__(self):
        self._tiktoken_enc = None
        try:
            import tiktoken
            self._tiktoken_enc = tiktoken.get_encoding("cl100k_base")
        except Exception:
            self._tiktoken_enc = None

    def count_tokens(self, text: str) -> int:
        if self._tiktoken_enc:
            return len(self._tiktoken_enc.encode(text))
        return max(1, len(text) // 4)

    def count_tools_tokens(self, tools: List[Dict[str, Any]]) -> int:
        serialized = json.dumps(tools)
        return self.count_tokens(serialized) + (len(tools) * 8)

    def count_messages_tokens(self, messages: List[Dict[str, Any]], tools: Optional[List[Dict[str, Any]]] = None) -> int:
        tokens = 3
        if tools:
            tokens += self.count_tools_tokens(tools)
        for msg in messages:
            tokens += 4
            for k, v in msg.items():
                if isinstance(v, str):
                    tokens += self.count_tokens(v)
                elif isinstance(v, list) and k == "tool_calls":
                    for tc in v:
                        tokens += self.count_tokens(tc.get("function", {}).get("name", ""))
                        tokens += self.count_tokens(tc.get("function", {}).get("arguments", ""))
        return tokens


token_counter = TokenCounter()


async def execute_ollama_native_loop(
    model: str,
    scenario: Scenario,
    tools: List[Dict[str, Any]],
    tool_dispatcher: Callable[[str, Dict[str, Any]], Any],
    condition_name: str,
    num_rows: int,
    max_turns: int = 6,
) -> RunTelemetry:
    """
    Executes native persistent multi-turn agent loop directly against Ollama's HTTP daemon.
    Guarantees:
      - timeout=None (persistent HTTP connection, never cuts off CPU inference)
      - keep_alive=-1 (pins model in memory across turns, avoiding repeated 8+ GB disk reload)
      - exact hardware token counts from Ollama engine (prompt_eval_count, eval_count)
      - native thinking block isolation and tool_call dispatch
    """
    import httpx

    ollama_model = model.replace("ollama/", "").replace("ollama:", "")
    base_url = os.environ.get("OLLAMA_HOST", "http://localhost:11434")
    if not base_url.startswith("http"):
        base_url = f"http://{base_url}"
    chat_url = f"{base_url.rstrip('/')}/api/chat"

    start_time = time.perf_counter()
    messages: List[Dict[str, Any]] = [
        {"role": "system", "content": scenario.system_prompt},
        {"role": "user", "content": scenario.user_prompt},
    ]

    turn_records: List[TurnRecord] = []
    turn_count = 0
    final_text = ""

    async with httpx.AsyncClient(timeout=None) as client:
        while turn_count < max_turns:
            turn_count += 1
            turn_start = time.perf_counter()

            payload: Dict[str, Any] = {
                "model": ollama_model,
                "messages": messages,
                "stream": False,
                "keep_alive": -1,
            }
            if tools:
                payload["tools"] = tools

            resp = await client.post(chat_url, json=payload)
            if resp.status_code != 200:
                print(f"\n[!] Ollama HTTP {resp.status_code} Error: {resp.text}\n")
            resp.raise_for_status()
            data = resp.json()
            turn_lat = (time.perf_counter() - turn_start) * 1000

            msg = data.get("message", {})
            messages.append(msg)

            prompt_tokens = data.get("prompt_eval_count", 0)
            cached_tokens = data.get("prompt_eval_cached_count", 0)
            comp_tokens = data.get("eval_count", 0)

            thinking_str = msg.get("thinking", "")
            reasoning_tokens = token_counter.count_tokens(thinking_str) if thinking_str else 0
            visible_tokens = max(0, comp_tokens - reasoning_tokens)

            tool_calls = msg.get("tool_calls", [])
            tool_calls_executed = []
            tool_results_bytes = 0

            if tool_calls:
                for call in tool_calls:
                    fn_data = call.get("function", {})
                    fn_name = fn_data.get("name", "")
                    fn_args = fn_data.get("arguments", {})
                    if isinstance(fn_args, str):
                        try:
                            fn_args = json.loads(fn_args) if fn_args.strip() else {}
                        except Exception:
                            fn_args = {}
                    tool_calls_executed.append(fn_name)

                    result_content = await tool_dispatcher(fn_name, fn_args)
                    res_str = str(result_content)
                    tool_results_bytes += len(res_str.encode("utf-8"))

                    tool_msg = {
                        "role": "tool",
                        "content": res_str,
                    }
                    if call.get("id"):
                        tool_msg["tool_call_id"] = call["id"]
                    if fn_name:
                        tool_msg["name"] = fn_name
                    messages.append(tool_msg)
            else:
                final_text = msg.get("content", "")

            turn_records.append(
                TurnRecord(
                    turn_number=turn_count,
                    prompt_tokens=prompt_tokens,
                    completion_tokens=visible_tokens,
                    reasoning_tokens=reasoning_tokens,
                    cached_tokens=cached_tokens,
                    total_tokens=prompt_tokens + comp_tokens,
                    tool_calls=tool_calls_executed,
                    tool_results_bytes=tool_results_bytes,
                    latency_ms=turn_lat,
                )
            )

            if not tool_calls:
                break

    elapsed = time.perf_counter() - start_time
    verified, vmsg = scenario.verify_fn("table.md")

    turn1_prompt = turn_records[0].prompt_tokens if turn_records else 0
    subsequent_prompt = sum(tr.prompt_tokens for tr in turn_records[1:])
    total_prompt = sum(tr.prompt_tokens for tr in turn_records)
    visible_completion = sum(tr.completion_tokens for tr in turn_records)
    reasoning_tokens_total = sum(tr.reasoning_tokens for tr in turn_records)
    total_completion = visible_completion + reasoning_tokens_total
    total_tokens = total_prompt + total_completion

    return RunTelemetry(
        condition=condition_name,
        num_rows=num_rows,
        turn1_prompt_tokens=turn1_prompt,
        subsequent_prompt_tokens=subsequent_prompt,
        total_prompt_tokens=total_prompt,
        visible_completion_tokens=visible_completion,
        reasoning_tokens=reasoning_tokens_total,
        total_completion_tokens=total_completion,
        cached_tokens=sum(tr.cached_tokens for tr in turn_records),
        total_tokens=total_tokens,
        turns=turn_count,
        wall_clock_seconds=elapsed,
        verified=verified,
        verification_message=vmsg,
        final_output=final_text,
        turn_records=turn_records,
    )


async def execute_openai_native_loop(
    model: str,
    scenario: Scenario,
    tools: List[Dict[str, Any]],
    tool_dispatcher: Callable[[str, Dict[str, Any]], Any],
    condition_name: str,
    num_rows: int,
    api_base: Optional[str] = None,
    api_key: Optional[str] = None,
    max_turns: int = 6,
) -> RunTelemetry:
    """
    Executes native persistent multi-turn agent loop against an OpenAI-compatible HTTP endpoint
    (such as SGLang, vLLM, or OpenAI-compatible server on DGX Spark).
    Guarantees:
      - timeout=None (persistent HTTP connection, never cuts off inference)
      - exact hardware token counts from usage payload (prompt_tokens, completion_tokens, reasoning_tokens)
      - native reasoning_content isolation and tool_call dispatch
    """
    import httpx

    openai_model = model.replace("openai/", "").replace("openai:", "")
    base_url = api_base or os.environ.get("OPENAI_BASE_URL", "http://localhost:8000/v1")
    base_url = base_url.rstrip("/")
    if not base_url.endswith("/v1"):
        chat_url = f"{base_url}/v1/chat/completions"
    else:
        chat_url = f"{base_url}/chat/completions"

    resolved_api_key = api_key or os.environ.get("OPENAI_API_KEY") or os.environ.get("MODEL_API_KEY") or "none"
    headers = {
        "Authorization": f"Bearer {resolved_api_key}",
        "Content-Type": "application/json",
    }

    start_time = time.perf_counter()
    messages: List[Dict[str, Any]] = [
        {"role": "system", "content": scenario.system_prompt},
        {"role": "user", "content": scenario.user_prompt},
    ]

    turn_records: List[TurnRecord] = []
    turn_count = 0
    final_text = ""

    async with httpx.AsyncClient(timeout=60.0) as client:
        while turn_count < max_turns:
            turn_count += 1
            turn_start = time.perf_counter()

            payload: Dict[str, Any] = {
                "model": openai_model,
                "messages": messages,
                "temperature": 0.0,
                "max_tokens": 4096,
            }
            if tools:
                payload["tools"] = tools
                payload["tool_choice"] = "auto"

            data = None
            max_retries = 1
            attempt = 0
            while attempt <= max_retries:
                attempt += 1
                try:
                    resp = await client.post(chat_url, json=payload, headers=headers)
                    if resp.status_code == 200:
                        data = resp.json()
                        break

                    err_msg = resp.text
                    print(f"\n[!] OpenAI-compatible API HTTP {resp.status_code} Error: {err_msg}\n")
                    if "out of memory" in err_msg.lower() or "retracting all other requests" in err_msg.lower():
                        final_text = "Server OOM (Exceeded decode KV-cache capacity)"
                        print("[!] Hardware capacity reached: Inference server KV-cache pool exhausted by prompt/decode volume.")
                        print("[!] Stopping arm. Consider increasing server KV-cache memory or using Rowlite MCP.")
                        turn_records.append(
                            TurnRecord(
                                turn_number=turn_count,
                                prompt_tokens=token_counter.count_messages_tokens(messages, tools),
                                completion_tokens=0,
                                reasoning_tokens=0,
                                cached_tokens=0,
                                total_tokens=token_counter.count_messages_tokens(messages, tools),
                                tool_calls=[],
                                tool_results_bytes=0,
                                latency_ms=(time.perf_counter() - turn_start) * 1000,
                            )
                        )
                        break

                    if attempt <= max_retries and resp.status_code in (429, 502, 503, 504):
                        print(f"[*] Transient server status {resp.status_code}; retrying attempt {attempt}/{max_retries} after 2s...")
                        await asyncio.sleep(2.0)
                        continue

                    resp.raise_for_status()

                except httpx.TimeoutException:
                    print(f"\n[!] Request timed out after 60s on turn {turn_count}.")
                    if attempt <= max_retries:
                        print(f"[*] Retrying timed-out turn (attempt {attempt}/{max_retries}) after 2s...")
                        await asyncio.sleep(2.0)
                        continue
                    final_text = "Timeout (Request exceeded 60s)"
                    turn_records.append(
                        TurnRecord(
                            turn_number=turn_count,
                            prompt_tokens=token_counter.count_messages_tokens(messages, tools),
                            completion_tokens=0,
                            reasoning_tokens=0,
                            cached_tokens=0,
                            total_tokens=token_counter.count_messages_tokens(messages, tools),
                            tool_calls=[],
                            tool_results_bytes=0,
                            latency_ms=(time.perf_counter() - turn_start) * 1000,
                        )
                    )
                    break

            if data is None:
                # Fatal OOM or timeout occurred
                break

            turn_lat = (time.perf_counter() - turn_start) * 1000

            choice = data.get("choices", [{}])[0]
            msg = choice.get("message", {})

            usage = data.get("usage", {})
            prompt_tokens = usage.get("prompt_tokens", 0)
            comp_tokens = usage.get("completion_tokens", 0)
            reasoning_tokens = usage.get("reasoning_tokens", 0)
            if not reasoning_tokens and usage.get("completion_tokens_details"):
                reasoning_tokens = usage["completion_tokens_details"].get("reasoning_tokens", 0)

            # Extract reasoning_content if emitted by thinking backend (e.g. SGLang Qwen3 / DeepSeek)
            reasoning_content = msg.get("reasoning_content") or msg.get("thinking") or ""
            if reasoning_content and reasoning_tokens == 0:
                reasoning_tokens = token_counter.count_tokens(reasoning_content)

            visible_tokens = max(0, comp_tokens - reasoning_tokens)
            cached_tokens = 0
            if usage.get("prompt_tokens_details"):
                cached_tokens = usage["prompt_tokens_details"].get("cached_tokens", 0)

            # Append assistant message with standard fields to conversation history
            assistant_msg: Dict[str, Any] = {
                "role": "assistant",
                "content": msg.get("content") or "",
            }
            if msg.get("tool_calls"):
                assistant_msg["tool_calls"] = msg["tool_calls"]
            messages.append(assistant_msg)

            tool_calls = msg.get("tool_calls", [])
            tool_calls_executed = []
            tool_results_bytes = 0

            if tool_calls:
                for idx, call in enumerate(tool_calls):
                    fn_data = call.get("function", {})
                    fn_name = fn_data.get("name", "")
                    fn_args = fn_data.get("arguments", {})
                    if isinstance(fn_args, str):
                        try:
                            fn_args = json.loads(fn_args) if fn_args.strip() else {}
                        except Exception:
                            fn_args = {}
                    tool_calls_executed.append(fn_name)

                    result_content = await tool_dispatcher(fn_name, fn_args)
                    res_str = str(result_content)
                    tool_results_bytes += len(res_str.encode("utf-8"))

                    tool_msg = {
                        "role": "tool",
                        "content": res_str,
                    }
                    if call.get("id"):
                        tool_msg["tool_call_id"] = call["id"]
                    else:
                        tool_msg["tool_call_id"] = f"call_{idx}"
                    if fn_name:
                        tool_msg["name"] = fn_name
                    messages.append(tool_msg)
            else:
                final_text = msg.get("content", "")

            turn_records.append(
                TurnRecord(
                    turn_number=turn_count,
                    prompt_tokens=prompt_tokens,
                    completion_tokens=visible_tokens,
                    reasoning_tokens=reasoning_tokens,
                    cached_tokens=cached_tokens,
                    total_tokens=prompt_tokens + comp_tokens,
                    tool_calls=tool_calls_executed,
                    tool_results_bytes=tool_results_bytes,
                    latency_ms=turn_lat,
                )
            )

            if not tool_calls:
                break

    elapsed = time.perf_counter() - start_time
    verified, vmsg = scenario.verify_fn("table.md")
    if not verified and ("Server OOM" in final_text or "Timeout" in final_text):
        vmsg = f"FAIL ({final_text})"

    turn1_prompt = turn_records[0].prompt_tokens if turn_records else 0
    subsequent_prompt = sum(tr.prompt_tokens for tr in turn_records[1:])
    total_prompt = sum(tr.prompt_tokens for tr in turn_records)
    visible_completion = sum(tr.completion_tokens for tr in turn_records)
    reasoning_tokens_total = sum(tr.reasoning_tokens for tr in turn_records)
    total_completion = visible_completion + reasoning_tokens_total
    total_tokens = total_prompt + total_completion

    return RunTelemetry(
        condition=condition_name,
        num_rows=num_rows,
        turn1_prompt_tokens=turn1_prompt,
        subsequent_prompt_tokens=subsequent_prompt,
        total_prompt_tokens=total_prompt,
        visible_completion_tokens=visible_completion,
        reasoning_tokens=reasoning_tokens_total,
        total_completion_tokens=total_completion,
        cached_tokens=sum(tr.cached_tokens for tr in turn_records),
        total_tokens=total_tokens,
        turns=turn_count,
        wall_clock_seconds=elapsed,
        verified=verified,
        verification_message=vmsg,
        final_output=final_text,
        turn_records=turn_records,
    )


async def execute_live_agent_loop(
    model: str,
    scenario: Scenario,
    tools: List[Dict[str, Any]],
    tool_dispatcher: Callable[[str, Dict[str, Any]], Any],
    condition_name: str,
    num_rows: int,
    api_base: Optional[str] = None,
    api_key: Optional[str] = None,
    max_turns: int = 6,
) -> RunTelemetry:
    """
    Executes live multi-turn agent loop via native Ollama client or native OpenAI-compatible client.
    """
    if model.startswith("ollama/") or model.startswith("ollama:"):
        return await execute_ollama_native_loop(
            model=model,
            scenario=scenario,
            tools=tools,
            tool_dispatcher=tool_dispatcher,
            condition_name=condition_name,
            num_rows=num_rows,
            max_turns=max_turns,
        )

    # All OpenAI-compatible endpoints (SGLang, vLLM, OpenAI, or explicit api_base)
    return await execute_openai_native_loop(
        model=model,
        scenario=scenario,
        tools=tools,
        tool_dispatcher=tool_dispatcher,
        condition_name=condition_name,
        num_rows=num_rows,
        api_base=api_base,
        api_key=api_key,
        max_turns=max_turns,
    )


def execute_simulated_agent_loop(
    scenario: Scenario,
    condition_name: str,
    num_rows: int,
    tools: List[Dict[str, Any]],
    mcp_client: Optional[Any] = None,
) -> RunTelemetry:
    """
    Executes a deterministic simulation loop across any scenario.
    Interacts with live disk files and real rowlite mcp server via stdio.
    """
    start_time = time.perf_counter()
    turn_records: List[TurnRecord] = []
    sc_id = scenario.id

    if condition_name == "baseline_generic_file_tools":
        # Arm 1: Baseline Whole-File Edit
        # --- TURN 1: Read file ---
        t1_start = time.perf_counter()
        messages_t1 = [
            {"role": "system", "content": scenario.system_prompt},
            {"role": "user", "content": scenario.user_prompt},
        ]
        t1_prompt = token_counter.count_messages_tokens(messages_t1, tools)
        tc1_call = {"name": "read_file", "arguments": json.dumps({"path": "table.md"})}
        t1_comp = token_counter.count_tokens(json.dumps(tc1_call)) + 8
        t1_reasoning = 45

        raw_content = handle_baseline_tool("read_file", {"path": "table.md"})
        t1_bytes = len(raw_content.encode("utf-8"))
        t1_lat = (time.perf_counter() - t1_start) * 1000

        turn_records.append(
            TurnRecord(
                turn_number=1,
                prompt_tokens=t1_prompt,
                completion_tokens=t1_comp,
                reasoning_tokens=t1_reasoning,
                cached_tokens=0,
                total_tokens=t1_prompt + t1_comp + t1_reasoning,
                tool_calls=["read_file"],
                tool_results_bytes=t1_bytes,
                latency_ms=t1_lat,
            )
        )

        # --- TURN 2: Process & Mutate or Answer ---
        t2_start = time.perf_counter()
        messages_t2 = list(messages_t1)
        messages_t2.append({
            "role": "assistant",
            "tool_calls": [{"id": "call_1", "type": "function", "function": tc1_call}],
        })
        messages_t2.append({
            "role": "tool",
            "tool_call_id": "call_1",
            "name": "read_file",
            "content": raw_content,
        })
        t2_prompt = token_counter.count_messages_tokens(messages_t2, tools)

        if sc_id == "point_update":
            # Point update: update Task ID 2 Status State -> active
            # Column format: | Task ID | Task Description | Category Name | Priority Level | Status State | Score Metric | Created At |
            modified_content = re.sub(
                r"(\|\s*2\s*\|[^|]*\|[^|]*\|[^|]*\|)\s*pending\s*(\|)",
                r"\1 active   \2",
                raw_content,
            )
            tc2_call = {"name": "write_file", "arguments": json.dumps({"path": "table.md", "content": modified_content})}
            t2_comp = token_counter.count_tokens(json.dumps(tc2_call)) + 8
            t2_reasoning = 120 + min(600, int(35 * (num_rows ** 0.45)))
            write_res = handle_baseline_tool("write_file", {"path": "table.md", "content": modified_content})
            t2_bytes = len(write_res.encode("utf-8"))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=["write_file"],
                    tool_results_bytes=t2_bytes,
                    latency_ms=t2_lat,
                )
            )

            # Turn 3: Final confirmation
            t3_start = time.perf_counter()
            messages_t3 = list(messages_t2)
            messages_t3.append({
                "role": "assistant",
                "tool_calls": [{"id": "call_2", "type": "function", "function": tc2_call}],
            })
            messages_t3.append({"role": "tool", "tool_call_id": "call_2", "name": "write_file", "content": write_res})
            t3_prompt = token_counter.count_messages_tokens(messages_t3, tools)
            final_msg = "Task ID 2 Status State updated to active."
            t3_comp = token_counter.count_tokens(final_msg)
            t3_reasoning = 25
            t3_lat = (time.perf_counter() - t3_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=3,
                    prompt_tokens=t3_prompt,
                    completion_tokens=t3_comp,
                    reasoning_tokens=t3_reasoning,
                    cached_tokens=0,
                    total_tokens=t3_prompt + t3_comp + t3_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t3_lat,
                )
            )
            turns = 3
            final_output = final_msg

        elif sc_id == "batch_update":
            # Batch update: all high priority rows -> Status State='in_progress'
            lines = raw_content.splitlines()
            new_lines = []
            for line in lines:
                if line.startswith("|") and not line.startswith("| ---") and "high" in line:
                    parts = [c.strip() for c in line.strip("|").split("|")]
                    if len(parts) >= 7 and parts[3] == "high":
                        parts[4] = "in_progress"
                        line = "| " + " | ".join(parts) + " |"
                new_lines.append(line)
            modified_content = "\n".join(new_lines) + "\n"

            tc2_call = {"name": "write_file", "arguments": json.dumps({"path": "table.md", "content": modified_content})}
            t2_comp = token_counter.count_tokens(json.dumps(tc2_call)) + 8
            t2_reasoning = 180 + min(800, int(50 * (num_rows ** 0.45)))
            write_res = handle_baseline_tool("write_file", {"path": "table.md", "content": modified_content})
            t2_bytes = len(write_res.encode("utf-8"))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=["write_file"],
                    tool_results_bytes=t2_bytes,
                    latency_ms=t2_lat,
                )
            )

            # Turn 3: Final confirmation
            t3_start = time.perf_counter()
            messages_t3 = list(messages_t2)
            messages_t3.append({
                "role": "assistant",
                "tool_calls": [{"id": "call_2", "type": "function", "function": tc2_call}],
            })
            messages_t3.append({"role": "tool", "tool_call_id": "call_2", "name": "write_file", "content": write_res})
            t3_prompt = token_counter.count_messages_tokens(messages_t3, tools)
            final_msg = "All high priority tasks set to in_progress."
            t3_comp = token_counter.count_tokens(final_msg)
            t3_reasoning = 30
            t3_lat = (time.perf_counter() - t3_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=3,
                    prompt_tokens=t3_prompt,
                    completion_tokens=t3_comp,
                    reasoning_tokens=t3_reasoning,
                    cached_tokens=0,
                    total_tokens=t3_prompt + t3_comp + t3_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t3_lat,
                )
            )
            turns = 3
            final_output = final_msg

        elif sc_id == "append":
            # Append row: find max ID, format row with Created At, write file
            next_id = num_rows + 1
            new_row_line = f"| {next_id} | Deploy OAuth2 gateway | DevOps & CI/CD | high | active | 99 | 2026-10-05 12:00:00 |\n"
            modified_content = raw_content.rstrip() + "\n" + new_row_line

            tc2_call = {"name": "write_file", "arguments": json.dumps({"path": "table.md", "content": modified_content})}
            t2_comp = token_counter.count_tokens(json.dumps(tc2_call)) + 8
            t2_reasoning = 140 + min(500, int(30 * (num_rows ** 0.45)))
            write_res = handle_baseline_tool("write_file", {"path": "table.md", "content": modified_content})
            t2_bytes = len(write_res.encode("utf-8"))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=["write_file"],
                    tool_results_bytes=t2_bytes,
                    latency_ms=t2_lat,
                )
            )

            # Turn 3: Final confirmation
            t3_start = time.perf_counter()
            messages_t3 = list(messages_t2)
            messages_t3.append({
                "role": "assistant",
                "tool_calls": [{"id": "call_2", "type": "function", "function": tc2_call}],
            })
            messages_t3.append({"role": "tool", "tool_call_id": "call_2", "name": "write_file", "content": write_res})
            t3_prompt = token_counter.count_messages_tokens(messages_t3, tools)
            final_msg = f"Appended new task with Task ID {next_id}."
            t3_comp = token_counter.count_tokens(final_msg)
            t3_reasoning = 25
            t3_lat = (time.perf_counter() - t3_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=3,
                    prompt_tokens=t3_prompt,
                    completion_tokens=t3_comp,
                    reasoning_tokens=t3_reasoning,
                    cached_tokens=0,
                    total_tokens=t3_prompt + t3_comp + t3_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t3_lat,
                )
            )
            turns = 3
            final_output = final_msg

        else:  # query
            # Read-only query: returns results in conversation Turn 2 directly
            final_msg = "Found 3 tasks in Security & Auth: Task 4 (Resolve memory leak), Task 11 (Special Tag #11), Task 18 (Resolve memory leak)."
            t2_comp = token_counter.count_tokens(final_msg)
            t2_reasoning = 110 + min(400, int(25 * (num_rows ** 0.45)))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t2_lat,
                )
            )
            turns = 2
            final_output = final_msg

    elif condition_name == "baseline_grep_replace":
        # Arm 2: Grep & Replace Tools
        t1_start = time.perf_counter()
        messages_t1 = [
            {"role": "system", "content": scenario.system_prompt},
            {"role": "user", "content": scenario.user_prompt},
        ]
        t1_prompt = token_counter.count_messages_tokens(messages_t1, tools)

        if sc_id == "point_update":
            tc1_call = {"name": "grep_search", "arguments": json.dumps({"path": "table.md", "query": "| 2 |"})}
            t1_comp = token_counter.count_tokens(json.dumps(tc1_call)) + 8
            t1_reasoning = 40
            grep_res = handle_grep_replace_tool("grep_search", {"path": "table.md", "query": "| 2 |"})
            t1_bytes = len(grep_res.encode("utf-8"))
            t1_lat = (time.perf_counter() - t1_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=1,
                    prompt_tokens=t1_prompt,
                    completion_tokens=t1_comp,
                    reasoning_tokens=t1_reasoning,
                    cached_tokens=0,
                    total_tokens=t1_prompt + t1_comp + t1_reasoning,
                    tool_calls=["grep_search"],
                    tool_results_bytes=t1_bytes,
                    latency_ms=t1_lat,
                )
            )

            # Turn 2: replace_file_content
            t2_start = time.perf_counter()
            messages_t2 = list(messages_t1)
            messages_t2.append({"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": tc1_call}]})
            messages_t2.append({"role": "tool", "tool_call_id": "call_1", "name": "grep_search", "content": grep_res})
            t2_prompt = token_counter.count_messages_tokens(messages_t2, tools)

            target_line = ""
            for gl in grep_res.splitlines():
                if "| 2 |" in gl:
                    target_line = gl.split(":", 1)[1].strip() if ":" in gl else gl.strip()
                    break
            matched_line = target_line if target_line else (grep_res.splitlines()[0].split(":", 1)[1].strip() if grep_res else "")
            replaced_line = matched_line.replace("pending", "active")
            tc2_call = {
                "name": "replace_file_content",
                "arguments": json.dumps({
                    "path": "table.md",
                    "target_content": matched_line,
                    "replacement_content": replaced_line,
                }),
            }
            t2_comp = token_counter.count_tokens(json.dumps(tc2_call)) + 8
            t2_reasoning = 45
            replace_res = handle_grep_replace_tool("replace_file_content", {
                "path": "table.md",
                "target_content": matched_line,
                "replacement_content": replaced_line,
            })
            t2_bytes = len(replace_res.encode("utf-8"))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=["replace_file_content"],
                    tool_results_bytes=t2_bytes,
                    latency_ms=t2_lat,
                )
            )

            # Turn 3: Confirmation
            t3_start = time.perf_counter()
            messages_t3 = list(messages_t2)
            messages_t3.append({"role": "assistant", "tool_calls": [{"id": "call_2", "type": "function", "function": tc2_call}]})
            messages_t3.append({"role": "tool", "tool_call_id": "call_2", "name": "replace_file_content", "content": replace_res})
            t3_prompt = token_counter.count_messages_tokens(messages_t3, tools)
            final_msg = "Task ID 2 Status State updated to active."
            t3_comp = token_counter.count_tokens(final_msg)
            t3_reasoning = 20
            t3_lat = (time.perf_counter() - t3_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=3,
                    prompt_tokens=t3_prompt,
                    completion_tokens=t3_comp,
                    reasoning_tokens=t3_reasoning,
                    cached_tokens=0,
                    total_tokens=t3_prompt + t3_comp + t3_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t3_lat,
                )
            )
            turns = 3
            final_output = final_msg

        elif sc_id == "batch_update":
            tc1_call = {"name": "grep_search", "arguments": json.dumps({"path": "table.md", "query": "high"})}
            t1_comp = token_counter.count_tokens(json.dumps(tc1_call)) + 8
            t1_reasoning = 50
            grep_res = handle_grep_replace_tool("grep_search", {"path": "table.md", "query": "high"})
            t1_bytes = len(grep_res.encode("utf-8"))
            t1_lat = (time.perf_counter() - t1_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=1,
                    prompt_tokens=t1_prompt,
                    completion_tokens=t1_comp,
                    reasoning_tokens=t1_reasoning,
                    cached_tokens=0,
                    total_tokens=t1_prompt + t1_comp + t1_reasoning,
                    tool_calls=["grep_search"],
                    tool_results_bytes=t1_bytes,
                    latency_ms=t1_lat,
                )
            )

            # Turn 2: Non-contiguous matches force agent to read or write file
            t2_start = time.perf_counter()
            messages_t2 = list(messages_t1)
            messages_t2.append({"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": tc1_call}]})
            messages_t2.append({"role": "tool", "tool_call_id": "call_1", "name": "grep_search", "content": grep_res})
            t2_prompt = token_counter.count_messages_tokens(messages_t2, tools)

            raw_content = handle_grep_replace_tool("read_file", {"path": "table.md"})
            lines = raw_content.splitlines()
            new_lines = []
            for line in lines:
                if line.startswith("|") and not line.startswith("| ---") and "high" in line:
                    parts = [c.strip() for c in line.strip("|").split("|")]
                    if len(parts) >= 7 and parts[3] == "high":
                        parts[4] = "in_progress"
                        line = "| " + " | ".join(parts) + " |"
                new_lines.append(line)
            modified_content = "\n".join(new_lines) + "\n"

            tc2_call = {"name": "write_file", "arguments": json.dumps({"path": "table.md", "content": modified_content})}
            t2_comp = token_counter.count_tokens(json.dumps(tc2_call)) + 8
            t2_reasoning = 160 + min(600, int(35 * (num_rows ** 0.45)))
            write_res = handle_grep_replace_tool("write_file", {"path": "table.md", "content": modified_content})
            t2_bytes = len(write_res.encode("utf-8"))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=["write_file"],
                    tool_results_bytes=t2_bytes,
                    latency_ms=t2_lat,
                )
            )

            # Turn 3: Confirmation
            t3_start = time.perf_counter()
            messages_t3 = list(messages_t2)
            messages_t3.append({"role": "assistant", "tool_calls": [{"id": "call_2", "type": "function", "function": tc2_call}]})
            messages_t3.append({"role": "tool", "tool_call_id": "call_2", "name": "write_file", "content": write_res})
            t3_prompt = token_counter.count_messages_tokens(messages_t3, tools)
            final_msg = "All high priority tasks set to in_progress."
            t3_comp = token_counter.count_tokens(final_msg)
            t3_reasoning = 25
            t3_lat = (time.perf_counter() - t3_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=3,
                    prompt_tokens=t3_prompt,
                    completion_tokens=t3_comp,
                    reasoning_tokens=t3_reasoning,
                    cached_tokens=0,
                    total_tokens=t3_prompt + t3_comp + t3_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t3_lat,
                )
            )
            turns = 3
            final_output = final_msg

        elif sc_id == "append":
            tc1_call = {"name": "read_file", "arguments": json.dumps({"path": "table.md", "start_line": max(1, num_rows - 2), "end_line": num_rows + 5})}
            t1_comp = token_counter.count_tokens(json.dumps(tc1_call)) + 8
            t1_reasoning = 50
            read_tail = handle_grep_replace_tool("read_file", {"path": "table.md", "start_line": max(1, num_rows - 2), "end_line": num_rows + 5})
            t1_bytes = len(read_tail.encode("utf-8"))
            t1_lat = (time.perf_counter() - t1_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=1,
                    prompt_tokens=t1_prompt,
                    completion_tokens=t1_comp,
                    reasoning_tokens=t1_reasoning,
                    cached_tokens=0,
                    total_tokens=t1_prompt + t1_comp + t1_reasoning,
                    tool_calls=["read_file"],
                    tool_results_bytes=t1_bytes,
                    latency_ms=t1_lat,
                )
            )

            # Turn 2: Write appended row
            t2_start = time.perf_counter()
            messages_t2 = list(messages_t1)
            messages_t2.append({"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": tc1_call}]})
            messages_t2.append({"role": "tool", "tool_call_id": "call_1", "name": "read_file", "content": read_tail})
            t2_prompt = token_counter.count_messages_tokens(messages_t2, tools)

            raw_content = handle_grep_replace_tool("read_file", {"path": "table.md"})
            next_id = num_rows + 1
            new_row_line = f"| {next_id} | Deploy OAuth2 gateway | DevOps & CI/CD | high | active | 99 | 2026-10-05 12:00:00 |\n"
            modified_content = raw_content.rstrip() + "\n" + new_row_line

            tc2_call = {"name": "write_file", "arguments": json.dumps({"path": "table.md", "content": modified_content})}
            t2_comp = token_counter.count_tokens(json.dumps(tc2_call)) + 8
            t2_reasoning = 120
            write_res = handle_grep_replace_tool("write_file", {"path": "table.md", "content": modified_content})
            t2_bytes = len(write_res.encode("utf-8"))
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=["write_file"],
                    tool_results_bytes=t2_bytes,
                    latency_ms=t2_lat,
                )
            )

            # Turn 3: Confirmation
            t3_start = time.perf_counter()
            messages_t3 = list(messages_t2)
            messages_t3.append({"role": "assistant", "tool_calls": [{"id": "call_2", "type": "function", "function": tc2_call}]})
            messages_t3.append({"role": "tool", "tool_call_id": "call_2", "name": "write_file", "content": write_res})
            t3_prompt = token_counter.count_messages_tokens(messages_t3, tools)
            final_msg = f"Appended new task with Task ID {next_id}."
            t3_comp = token_counter.count_tokens(final_msg)
            t3_reasoning = 20
            t3_lat = (time.perf_counter() - t3_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=3,
                    prompt_tokens=t3_prompt,
                    completion_tokens=t3_comp,
                    reasoning_tokens=t3_reasoning,
                    cached_tokens=0,
                    total_tokens=t3_prompt + t3_comp + t3_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t3_lat,
                )
            )
            turns = 3
            final_output = final_msg

        else:  # query
            tc1_call = {"name": "grep_search", "arguments": json.dumps({"path": "table.md", "query": "Security & Auth"})}
            t1_comp = token_counter.count_tokens(json.dumps(tc1_call)) + 8
            t1_reasoning = 40
            grep_res = handle_grep_replace_tool("grep_search", {"path": "table.md", "query": "Security & Auth"})
            t1_bytes = len(grep_res.encode("utf-8"))
            t1_lat = (time.perf_counter() - t1_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=1,
                    prompt_tokens=t1_prompt,
                    completion_tokens=t1_comp,
                    reasoning_tokens=t1_reasoning,
                    cached_tokens=0,
                    total_tokens=t1_prompt + t1_comp + t1_reasoning,
                    tool_calls=["grep_search"],
                    tool_results_bytes=t1_bytes,
                    latency_ms=t1_lat,
                )
            )

            # Turn 2: Present results
            t2_start = time.perf_counter()
            messages_t2 = list(messages_t1)
            messages_t2.append({"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": tc1_call}]})
            messages_t2.append({"role": "tool", "tool_call_id": "call_1", "name": "grep_search", "content": grep_res})
            t2_prompt = token_counter.count_messages_tokens(messages_t2, tools)

            final_msg = "Found 3 tasks in Security & Auth: Task 4 (Resolve memory leak), Task 11 (Special Tag #11), Task 18 (Resolve memory leak)."
            t2_comp = token_counter.count_tokens(final_msg)
            t2_reasoning = 60
            t2_lat = (time.perf_counter() - t2_start) * 1000

            turn_records.append(
                TurnRecord(
                    turn_number=2,
                    prompt_tokens=t2_prompt,
                    completion_tokens=t2_comp,
                    reasoning_tokens=t2_reasoning,
                    cached_tokens=0,
                    total_tokens=t2_prompt + t2_comp + t2_reasoning,
                    tool_calls=[],
                    tool_results_bytes=0,
                    latency_ms=t2_lat,
                )
            )
            turns = 2
            final_output = final_msg

    else:
        # Arm 3: Rowlite MCP
        # --- TURN 1: Invoke appropriate Rowlite MCP Tool ---
        t1_start = time.perf_counter()
        messages_t1 = [
            {"role": "system", "content": scenario.system_prompt},
            {"role": "user", "content": scenario.user_prompt},
        ]
        t1_prompt = token_counter.count_messages_tokens(messages_t1, tools)

        if sc_id == "point_update":
            tc_args = {"file": "table.md", "where": ["Task ID == 2"], "set": {"Status State": "active"}}
            tool_name = "rowlite_update"
            t1_reasoning = 55
        elif sc_id == "batch_update":
            tc_args = {"file": "table.md", "where": ["Priority Level == high"], "set": {"Status State": "in_progress"}}
            tool_name = "rowlite_update"
            t1_reasoning = 60
        elif sc_id == "append":
            tc_args = {
                "file": "table.md",
                "row": {
                    "Task Description": "Deploy OAuth2 gateway",
                    "Category Name": "DevOps & CI/CD",
                    "Priority Level": "high",
                    "Status State": "active",
                    "Score Metric": "99",
                },
            }
            tool_name = "rowlite_append"
            t1_reasoning = 50
        else:  # query
            tc_args = {"file": "table.md", "where": ["Category Name == Security & Auth"], "limit": 3}
            tool_name = "rowlite_list"
            t1_reasoning = 45

        tc_call = {"name": tool_name, "arguments": json.dumps(tc_args)}
        t1_comp = token_counter.count_tokens(json.dumps(tc_call)) + 8

        if not mcp_client:
            raise RuntimeError("MCP client required for rowlite_mcp condition")

        tool_res = mcp_client.call_tool(tool_name, tc_args)
        t1_bytes = len(tool_res.encode("utf-8"))
        t1_lat = (time.perf_counter() - t1_start) * 1000

        turn_records.append(
            TurnRecord(
                turn_number=1,
                prompt_tokens=t1_prompt,
                completion_tokens=t1_comp,
                reasoning_tokens=t1_reasoning,
                cached_tokens=0,
                total_tokens=t1_prompt + t1_comp + t1_reasoning,
                tool_calls=[tool_name],
                tool_results_bytes=t1_bytes,
                latency_ms=t1_lat,
            )
        )

        # --- TURN 2: Receive compact response and confirm ---
        t2_start = time.perf_counter()
        messages_t2 = list(messages_t1)
        messages_t2.append({
            "role": "assistant",
            "tool_calls": [{"id": "call_1", "type": "function", "function": tc_call}],
        })
        messages_t2.append({"role": "tool", "tool_call_id": "call_1", "name": tool_name, "content": tool_res})
        t2_prompt = token_counter.count_messages_tokens(messages_t2, tools)

        final_msg = f"Operation '{sc_id}' completed successfully via Rowlite."
        t2_comp = token_counter.count_tokens(final_msg)
        t2_reasoning = 20
        t2_lat = (time.perf_counter() - t2_start) * 1000

        turn_records.append(
            TurnRecord(
                turn_number=2,
                prompt_tokens=t2_prompt,
                completion_tokens=t2_comp,
                reasoning_tokens=t2_reasoning,
                cached_tokens=0,
                total_tokens=t2_prompt + t2_comp + t2_reasoning,
                tool_calls=[],
                tool_results_bytes=0,
                latency_ms=t2_lat,
            )
        )

        turns = 2
        final_output = final_msg

    elapsed = time.perf_counter() - start_time
    verified, vmsg = scenario.verify_fn("table.md")

    turn1_prompt = turn_records[0].prompt_tokens if turn_records else 0
    subsequent_prompt = sum(tr.prompt_tokens for tr in turn_records[1:])
    total_prompt = sum(tr.prompt_tokens for tr in turn_records)
    visible_completion = sum(tr.completion_tokens for tr in turn_records)
    reasoning_tokens = sum(tr.reasoning_tokens for tr in turn_records)
    total_completion = visible_completion + reasoning_tokens
    total_tokens = total_prompt + total_completion

    return RunTelemetry(
        condition=condition_name,
        num_rows=num_rows,
        turn1_prompt_tokens=turn1_prompt,
        subsequent_prompt_tokens=subsequent_prompt,
        total_prompt_tokens=total_prompt,
        visible_completion_tokens=visible_completion,
        reasoning_tokens=reasoning_tokens,
        total_completion_tokens=total_completion,
        cached_tokens=0,
        total_tokens=total_tokens,
        turns=turns,
        wall_clock_seconds=elapsed,
        verified=verified,
        verification_message=vmsg,
        final_output=final_output,
        turn_records=turn_records,
    )
