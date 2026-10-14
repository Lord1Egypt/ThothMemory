#!/usr/bin/env python3
"""Run newcomer commands against a native UltraLite artifact, with disposable data.

No model or provider credential is used. --client verifies an installed CLI's
registration; semantic writes are real MCP stdio calls, not model behavior.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser()
p.add_argument("binary")
p.add_argument("--client", choices=["codex", "claude", "opencode"])
args = p.parse_args()
binary = str(Path(args.binary).resolve())
with tempfile.TemporaryDirectory(prefix="thoth-quickstart-") as temp:
    root = Path(temp)
    data = root / "memory"
    project = root / "project"
    project.mkdir()
    subprocess.run(["git", "init", "-q", str(project)], check=True)
    subprocess.run(["git", "-C", str(project), "remote", "add", "origin", "https://github.com/thothmemory-example/quickstart"], check=True)

    def cli(*a):
        return subprocess.run([binary, "--data", str(data), *a], cwd=project,
                              check=True, text=True, capture_output=True).stdout

    cli("init")
    cli("init")
    cli("doctor")
    cli("instructions", "--yes")
    assert "THOTHMEMORY_AGENT_PROTOCOL.md" in (project / "AGENTS.md").read_text()
    if args.client:
        registration = cli("connect", args.client, "--yes")
        assert "not installed" not in registration, registration
        config = {"codex": ".codex/config.toml", "claude": ".mcp.json", "opencode": "opencode.json"}[args.client]
        assert (project / config).is_file()
    token = data / ".secrets" / "agent_token"
    proc = subprocess.Popen([binary, "mcp-stdio", "--data", str(data), "--token-file", str(token)],
                            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    seq = 0

    def rpc(method, params):
        global seq
        seq += 1
        proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": seq, "method": method, "params": params}) + "\n")
        proc.stdin.flush()
        response = json.loads(proc.stdout.readline())
        assert "error" not in response, response
        return response["result"]

    def tool(name, params):
        result = rpc("tools/call", {"name": "memory_" + name, "arguments": params})
        assert not result.get("isError"), result
        return json.loads(result["content"][0]["text"])["data"]

    rpc("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "quickstart-smoke", "version": "1"}})
    proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
    proc.stdin.flush()
    identity = tool("create_project", {"name": "Quickstart", "repository": "https://github.com/thothmemory-example/quickstart"})["id"]
    tool("record_idea", {"project_id": identity, "operation_id": "quickstart-idea", "title": "PocketClaw SQLite", "body": "Maybe add SQLite support to PocketClaw. Proposed, not approved."})
    proc.stdin.close()
    assert proc.wait(timeout=20) == 0
    assert "PocketClaw" in cli("search", identity, "PocketClaw")
    assert identity in cli("projects")
    cli("backup", str(root / "snapshot.db"))
    cli("restore", str(root / "snapshot.db"), "--confirm")
    assert "PocketClaw" in cli("search", identity, "PocketClaw")
    cli("doctor")
    print("PASS native quickstart: init, instructions, MCP idea write, process exit, CLI read, backup/restore" + (", real " + args.client + " registration" if args.client else ""))
