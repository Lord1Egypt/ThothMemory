#!/usr/bin/env python3
"""Black-box acceptance gates for a running ThothMemory (Mini or Full).

Drives the real MCP endpoint with real credentials and prints one PASS/FAIL
line per gate, then optional latency samples. Standard library only, so it
runs unchanged in ThothTerm's Debian guest on the phone.

  mini-acceptance.py --endpoint http://127.0.0.1:7319/mcp \
      --token-file agent_token [--reader-token-file read_token] [--bench 50]

It creates one clearly labelled project per run and never deletes anything.
"""
import argparse, concurrent.futures, json, statistics, sys, time, urllib.error, urllib.request, uuid

p = argparse.ArgumentParser()
p.add_argument("--endpoint", required=True)
p.add_argument("--token-file", required=True)
p.add_argument("--reader-token-file")
p.add_argument("--bench", type=int, default=0)
p.add_argument("--json", help="write results to this file")
args = p.parse_args()
if not args.endpoint.startswith(("http://127.0.0.1:", "http://localhost:", "http://[::1]:")):
    sys.exit("refusing a non-loopback endpoint")
TOKEN = open(args.token_file).read().strip()
READER = open(args.reader_token_file).read().strip() if args.reader_token_file else None
seq = 0


def rpc(method, params, token=TOKEN):
    global seq
    seq += 1
    req = urllib.request.Request(args.endpoint, json.dumps({"jsonrpc": "2.0", "id": seq, "method": method, "params": params}).encode(),
                                 {"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "Authorization": "Bearer " + token})
    with urllib.request.urlopen(req, timeout=30) as r:
        v = json.load(r)
    if "error" in v:
        raise RuntimeError(v["error"])
    return v["result"]


class ToolError(Exception):
    pass


def tool(name, a, token=TOKEN):
    r = rpc("tools/call", {"name": "memory_" + name, "arguments": a}, token)
    if r.get("isError"):
        raise ToolError(r["content"][0]["text"])
    return json.loads(r["content"][0]["text"])["data"]


def op():
    return "acceptance-" + uuid.uuid4().hex[:16]


results = []


def gate(name, fn):
    t0 = time.perf_counter()
    try:
        detail = fn() or ""
        results.append({"gate": name, "result": "PASS", "detail": detail, "ms": round((time.perf_counter() - t0) * 1000, 1)})
    except Exception as e:  # report every gate, never stop at the first failure
        results.append({"gate": name, "result": "FAIL", "detail": f"{type(e).__name__}: {e}"})
    r = results[-1]
    print(f"{r['result']}  {name}  {r['detail']}", flush=True)


HEAD, OTHER = "ab" * 20, "cd" * 20
ctx = {}


def health():
    base = args.endpoint.rsplit("/mcp", 1)[0]
    with urllib.request.urlopen(base + "/healthz", timeout=10) as r:
        v = json.load(r)
    assert v["status"] == "ready", v
    return f"{v['edition']} {v['version']}"


def handshake():
    init = rpc("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "acceptance", "version": "1"}})
    tools = rpc("tools/list", {})["tools"]
    assert len(tools) == 25 and not any(t["name"] == "memory_attest_evidence" for t in tools), len(tools)
    return f"{init['serverInfo']['name']} {init['serverInfo']['version']}, {len(tools)} tools"


def unauthenticated():
    try:
        rpc("tools/list", {}, token="x" * 64)
    except urllib.error.HTTPError as e:
        assert e.code == 401, e.code
        return "HTTP 401"
    raise AssertionError("invalid token accepted")


def crud():
    p = tool("create_project", {"name": "ACCEPTANCE " + time.strftime("%Y-%m-%d %H:%M:%S"), "repository": "local:" + str(uuid.uuid4())})
    ctx["p"] = p["id"]
    ctx["t"] = tool("create_task", {"project_id": p["id"], "operation_id": op(), "title": "Acceptance task", "git_revision": HEAD})["id"]
    ctx["s"] = tool("start_session", {"project_id": p["id"], "operation_id": op(), "agent": "acceptance", "git_revision": HEAD, "branch": "main"})["id"]
    tool("record_decision", {"project_id": p["id"], "operation_id": op(), "title": "Acceptance decision", "body": "قرار الاختبار: keep transactions short", "git_revision": HEAD, "session_id": ctx["s"]})
    got = tool("get_task", {"project_id": p["id"], "task_id": ctx["t"]})
    assert got[0]["revision"] == 1, got
    c = tool("get_project_context", {"project_id": p["id"], "git_revision": HEAD})
    assert len(c["decisions_and_issues"]) == 1 and c["sessions"][0]["status"] == "incomplete", c
    return "project, task, session, decision written and read back"


def rollback():
    other = tool("create_project", {"name": "ACCEPTANCE other", "repository": "local:" + str(uuid.uuid4())})["id"]
    before = len(tool("get_recent_activity", {"project_id": other, "limit": 100}))
    try:
        tool("attach_evidence", {"project_id": other, "task_id": ctx["t"], "operation_id": op(), "source_kind": "test", "locator": "x", "git_revision": HEAD, "outcome": "pass", "body": "cross-project"})
        raise AssertionError("cross-project evidence accepted")
    except ToolError:
        pass
    after = len(tool("get_recent_activity", {"project_id": other, "limit": 100}))
    assert before == after, (before, after)
    return "foreign-key failure rolled back event and operation"


def concurrency():
    def attempt(i):
        try:
            tool("update_task", {"project_id": ctx["p"], "task_id": ctx["t"], "operation_id": op(), "expected_revision": 1, "state": "implemented"})
            return True
        except ToolError:
            return False
    with concurrent.futures.ThreadPoolExecutor(8) as ex:
        wins = sum(ex.map(attempt, range(8)))
    assert wins == 1, f"{wins} writers succeeded"
    return "8 concurrent writers, exactly 1 succeeded"


def idempotency():
    a = {"project_id": ctx["p"], "operation_id": op(), "body": "idempotent note"}
    first, again = tool("append_event", a), tool("append_event", a)
    assert first == again
    try:
        tool("append_event", {**a, "body": "changed"})
        raise AssertionError("changed payload accepted")
    except ToolError as e:
        assert "conflict" in str(e), e
    return "replay identical, changed payload refused"


def gates():
    tool("record_test_result", {"project_id": ctx["p"], "task_id": ctx["t"], "operation_id": op(), "locator": "acceptance://test", "git_revision": HEAD, "outcome": "pass", "body": "reported"})
    try:
        tool("update_task", {"project_id": ctx["p"], "task_id": ctx["t"], "operation_id": op(), "expected_revision": 2, "state": "verified", "git_revision": HEAD})
        raise AssertionError("verified without owner attestation")
    except ToolError:
        pass
    tool("update_task", {"project_id": ctx["p"], "task_id": ctx["t"], "operation_id": op(), "expected_revision": 2, "state": "tested", "git_revision": HEAD})
    return "tested allowed, verified refused without owner attestation"


def search():
    hits = tool("search", {"project_id": ctx["p"], "query": "الاختبار", "git_revision": OTHER})
    assert any("قرار" in h["body"] for h in hits) and all(h["stale"] for h in hits if h["git_revision"]), hits
    assert tool("search", {"project_id": ctx["p"], "query": HEAD[:12]}), "commit prefix not found"
    return "Arabic substring and commit prefix found; stale flags set"


def read_only():
    if not READER:
        return "skipped: no reader token supplied"
    try:
        tool("append_event", {"project_id": ctx["p"], "operation_id": op(), "body": "x"}, token=READER)
        raise AssertionError("reader wrote")
    except ToolError:
        return "write refused for read credential"
    except RuntimeError as e:
        assert "unknown tool" in str(e), e
        return "write tool not even discoverable by read credential"


def finish():
    tool("finish_session", {"project_id": ctx["p"], "session_id": ctx["s"], "operation_id": op(), "expected_revision": 1, "git_revision": HEAD, "body": "Acceptance run finished."})
    assert tool("get_latest_handoff", {"project_id": ctx["p"]})[0]["body"] == "Acceptance run finished."
    return "handoff recorded and retrieved"


for name, fn in [("health", health), ("mcp handshake and discovery", handshake), ("authentication rejects bad token", unauthenticated),
                 ("crud and read-back", crud), ("transaction rollback", rollback), ("optimistic concurrency", concurrency),
                 ("idempotency", idempotency), ("evidence gates", gates), ("multilingual search", search),
                 ("read-only credential", read_only), ("session handoff", finish)]:
    gate(name, fn)

bench = {}
if args.bench and "p" in ctx:
    samples = {"context": [], "search": [], "write": []}
    for i in range(args.bench):
        for kind, call in (("context", lambda: tool("get_project_context", {"project_id": ctx["p"], "git_revision": HEAD})),
                           ("search", lambda: tool("search", {"project_id": ctx["p"], "query": "acceptance"})),
                           ("write", lambda: tool("append_event", {"project_id": ctx["p"], "operation_id": op(), "body": f"bench {i}"}))):
            t0 = time.perf_counter()
            call()
            samples[kind].append((time.perf_counter() - t0) * 1000)
    for kind, v in samples.items():
        v.sort()
        bench[kind] = {"n": len(v), "median_ms": round(statistics.median(v), 2), "p95_ms": round(v[int(len(v) * 0.95) - 1], 2), "max_ms": round(v[-1], 2)}
        print(f"BENCH {kind}: n={len(v)} median={bench[kind]['median_ms']} ms p95={bench[kind]['p95_ms']} ms max={bench[kind]['max_ms']} ms")

if args.json:
    with open(args.json, "w") as f:
        json.dump({"endpoint": args.endpoint, "results": results, "bench": bench, "project": ctx.get("p")}, f, indent=1)
failed = [r for r in results if r["result"] != "PASS"]
print(f"{len(results) - len(failed)}/{len(results)} gates passed")
sys.exit(1 if failed else 0)
