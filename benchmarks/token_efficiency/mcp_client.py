"""
Native stdio Model Context Protocol (MCP) client for Rowlite.
Communicates with `rowlite mcp` via JSON-RPC 2.0 over standard I/O.
"""

import json
import os
import subprocess
from typing import Any, Dict, List, Optional


class StdioMCPClient:
    def __init__(self, command: str = "rowlite", args: Optional[List[str]] = None, cwd: Optional[str] = None):
        self.command = command
        self.args = args if args is not None else ["mcp"]
        self.cwd = cwd
        self.process: Optional[subprocess.Popen] = None
        self._request_id = 0

    def start(self):
        cmd = [self.command] + self.args
        self.process = subprocess.Popen(
            cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            cwd=self.cwd,
            text=True,
            encoding="utf-8",
            bufsize=1,
        )
        self._initialize()

    def _next_id(self) -> int:
        self._request_id += 1
        return self._request_id

    def _send(self, payload: Dict[str, Any]):
        if not self.process or not self.process.stdin:
            raise RuntimeError("MCP process is not running.")
        line = json.dumps(payload) + "\n"
        self.process.stdin.write(line)
        self.process.stdin.flush()

    def _read_response(self) -> Dict[str, Any]:
        if not self.process or not self.process.stdout:
            raise RuntimeError("MCP process is not running.")
        line = self.process.stdout.readline()
        if not line:
            stderr = self.process.stderr.read() if self.process.stderr else ""
            raise RuntimeError(f"MCP server closed stdout unexpectedly. Stderr: {stderr}")
        return json.loads(line)

    def _initialize(self):
        req = {
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {
                    "name": "rowlite-token-benchmark",
                    "version": "1.0.0",
                },
            },
        }
        self._send(req)
        resp = self._read_response()
        if "error" in resp:
            raise RuntimeError(f"MCP initialize failed: {resp['error']}")

        self.instructions = resp.get("result", {}).get("instructions", "")

        # Send initialized notification
        notify = {
            "jsonrpc": "2.0",
            "method": "notifications/initialized",
            "params": {},
        }
        self._send(notify)

    def get_instructions(self) -> str:
        return getattr(self, "instructions", "")

    def list_tools(self) -> List[Dict[str, Any]]:
        """
        Queries tools/list from Rowlite MCP server and converts them into OpenAI function calling format.
        """
        req = {
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "tools/list",
            "params": {},
        }
        self._send(req)
        resp = self._read_response()
        if "error" in resp:
            raise RuntimeError(f"tools/list failed: {resp['error']}")

        tools_raw = resp.get("result", {}).get("tools", [])
        converted = []
        for t in tools_raw:
            params = t.get("inputSchema", {})
            props = params.get("properties", {})
            if isinstance(props, dict):
                for k, v in list(props.items()):
                    if isinstance(v, bool):
                        props[k] = {"description": "0-based index or range (e.g. 0, '2..5')"}
            converted.append({
                "type": "function",
                "function": {
                    "name": t.get("name"),
                    "description": t.get("description", ""),
                    "parameters": params,
                },
            })
        return converted

    def call_tool(self, name: str, arguments: Dict[str, Any]) -> str:
        """
        Invokes a tool via tools/call and returns text content.
        """
        req = {
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "tools/call",
            "params": {
                "name": name,
                "arguments": arguments,
            },
        }
        self._send(req)
        resp = self._read_response()
        if "error" in resp:
            return f"Error: {json.dumps(resp['error'])}"

        result = resp.get("result", {})
        if result.get("isError"):
            contents = [c.get("text", "") for c in result.get("content", []) if c.get("type") == "text"]
            return f"Tool Error: {' '.join(contents)}"

        contents = [c.get("text", "") for c in result.get("content", []) if c.get("type") == "text"]
        return "\n".join(contents)

    def close(self):
        if self.process:
            try:
                self.process.terminate()
                self.process.wait(timeout=2)
            except Exception:
                self.process.kill()
            self.process = None

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
