#!/usr/bin/env python3
"""Real MCP seed/read helper for Mini backup-and-restore gates (standard library only).

  mini-recovery-mcp.py seed --endpoint URL --token-file F --project ID
  mini-recovery-mcp.py read --endpoint URL --token-file F --project ID

`seed` adds evidence, a note and an idea to a project through the MCP endpoint.
`read` queries project context, tasks, handoff and search through the MCP endpoint and
prints one canonical JSON line whose "sha256" must be identical before backup and after
a restored database is activated. Ids and timestamps are preserved by a dump, so the
whole response is hashed, not just counts.
"""
import argparse, hashlib, json, sys, urllib.request, uuid

p = argparse.ArgumentParser()
p.add_argument("mode", choices=["seed", "read"])
p.add_argument("--endpoint", required=True)
p.add_argument("--token-file", required=True)
p.add_argument("--project", required=True)
a = p.parse_args()
if not a.endpoint.startswith(("http://127.0.0.1:", "http://localhost:", "http://[::1]:")):
    sys.exit("refusing a non-loopback endpoint")
token = open(a.token_file).read().strip()
HEAD = "a" * 40
seq = 0


def tool(name, args):
    global seq
    seq += 1
    body = json.dumps({"jsonrpc": "2.0", "id": seq, "method": "tools/call", "params": {"name": "memory_" + name, "arguments": args}}).encode()
    req = urllib.request.Request(a.endpoint, body, {"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "Authorization": "Bearer " + token})
    with urllib.request.urlopen(req, timeout=30) as r:
        v = json.load(r)
    if "error" in v or v["result"].get("isError"):
        sys.exit("MCP %s failed: %s" % (name, v.get("error") or v["result"]["content"][0]["text"]))
    return json.loads(v["result"]["content"][0]["text"])["data"]


if a.mode == "seed":
    tasks = tool("list_tasks", {"project_id": a.project})
    task = tasks[0]["id"]
    tool("attach_evidence", {"project_id": a.project, "task_id": task, "operation_id": "recovery-" + uuid.uuid4().hex[:16], "source_kind": "test", "locator": "test://recovery-fixture", "git_revision": HEAD, "outcome": "pass", "body": "Recovery fixture evidence"})
    tool("record_note", {"project_id": a.project, "operation_id": "recovery-" + uuid.uuid4().hex[:16], "title": "Recovery note", "body": "Backup and restore must preserve this note.", "git_revision": HEAD})
    tool("record_idea", {"project_id": a.project, "operation_id": "recovery-" + uuid.uuid4().hex[:16], "title": "Recovery idea", "body": "Restored databases are verified before activation.", "git_revision": HEAD})
    print("seeded evidence, note and idea")
else:
    out = {
        "context": tool("get_project_context", {"project_id": a.project, "git_revision": HEAD}),
        "tasks": tool("list_tasks", {"project_id": a.project}),
        "handoff": tool("get_latest_handoff", {"project_id": a.project}),
        "recent": tool("get_recent_activity", {"project_id": a.project}),
        "search": tool("search", {"project_id": a.project, "query": "Recovery"}),
    }
    blob = json.dumps(out, sort_keys=True, ensure_ascii=False)
    print(json.dumps({"sha256": hashlib.sha256(blob.encode()).hexdigest(), "tasks": len(out["tasks"]), "search_hits": len(out["search"])}))
