#!/usr/bin/env python3
"""One MCP workload against any ThothMemory edition; standard library only.

  bench-editions.py --http http://127.0.0.1:7319/mcp --token-file TOKEN [options]
  bench-editions.py --stdio "thothmemory-ultralite --data DIR mcp-stdio --token-file TOKEN" [options]

Creates one clearly labelled local project, grows it to each --sizes total of
events and at each size samples get_project_context, search and append_event.
Prints JSON. Never deletes anything; use disposable installations.
"""
import argparse, json, shlex, statistics, subprocess, sys, time, urllib.request, uuid

ap = argparse.ArgumentParser()
g = ap.add_mutually_exclusive_group(required=True)
g.add_argument("--http")
g.add_argument("--stdio")
ap.add_argument("--token-file")
ap.add_argument("--sizes", default="200,2000,10000")
ap.add_argument("--samples", type=int, default=50)
ap.add_argument("--label", default="")
a = ap.parse_args()


class HTTP:
    def __init__(self, url, token):
        if not url.startswith("http://127.0.0.1:"):
            sys.exit("refusing a non-loopback endpoint")
        self.url, self.token, self.seq = url, token, 0

    def rpc(self, method, params):
        self.seq += 1
        req = urllib.request.Request(self.url, json.dumps({"jsonrpc": "2.0", "id": self.seq, "method": method, "params": params}).encode(),
                                     {"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "Authorization": "Bearer " + self.token})
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.load(r)

    def close(self):
        pass


class Stdio:
    """MCP stdio: newline-delimited JSON-RPC to a child process."""
    def __init__(self, command):
        self.p = subprocess.Popen(shlex.split(command), stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)
        self.seq = 0

    def rpc(self, method, params):
        self.seq += 1
        self.p.stdin.write(json.dumps({"jsonrpc": "2.0", "id": self.seq, "method": method, "params": params}) + "\n")
        self.p.stdin.flush()
        while True:
            line = self.p.stdout.readline()
            if not line:
                raise RuntimeError("server closed the stream")
            msg = json.loads(line)
            if msg.get("id") == self.seq:
                return msg

    def notify(self, method):
        self.p.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method}) + "\n")
        self.p.stdin.flush()

    def close(self):
        self.p.stdin.close()
        return self.p.wait(timeout=10)


def connect():
    t0 = time.perf_counter()
    c = HTTP(a.http, open(a.token_file).read().strip()) if a.http else Stdio(a.stdio)
    r = c.rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "bench", "version": "1"}})
    if isinstance(c, Stdio):
        c.notify("notifications/initialized")
    return c, (time.perf_counter() - t0) * 1000, r["result"]["serverInfo"]


def tool(c, name, args):
    r = c.rpc("tools/call", {"name": "memory_" + name, "arguments": args})["result"]
    if r.get("isError"):
        raise RuntimeError(r["content"][0]["text"])
    return json.loads(r["content"][0]["text"])["data"]


def stats(v):
    v = sorted(v)
    return {"n": len(v), "median_ms": round(statistics.median(v), 2), "p95_ms": round(v[max(0, int(len(v) * 0.95) - 1)], 2), "max_ms": round(v[-1], 2)}


c, first_ms, info = connect()
out = {"label": a.label, "server": info, "connect_and_initialize_ms": round(first_ms, 1), "sizes": []}
project = tool(c, "create_project", {"name": "BENCH " + a.label, "repository": "local:" + str(uuid.uuid4())})["id"]
words = ["migration", "rollback", "cache", "handoff", "تعارض", "المراجعات", "evidence", "revision", "checkpoint", "index"]
written = 0
for size in [int(x) for x in a.sizes.split(",")]:
    t0, filled = time.perf_counter(), 0
    while written < size:
        tool(c, "append_event", {"project_id": project, "operation_id": "bench-" + uuid.uuid4().hex, "body": f"note {written} {words[written % 10]} {words[(written * 7) % 10]} lorem ipsum dolor sit amet"})
        written += 1
        filled += 1
    fill = time.perf_counter() - t0
    samples = {"context": [], "search": [], "write": []}
    for i in range(a.samples):
        for kind, call in (("context", lambda: tool(c, "get_project_context", {"project_id": project})),
                           ("search", lambda: tool(c, "search", {"project_id": project, "query": words[i % 10]})),
                           ("write", lambda: tool(c, "append_event", {"project_id": project, "operation_id": "bench-" + uuid.uuid4().hex, "body": "sample write"}))):
            s = time.perf_counter()
            call()
            samples[kind].append((time.perf_counter() - s) * 1000)
    written += a.samples
    out["sizes"].append({"events": written, "fill_events_per_s": round(filled / fill, 1) if filled else None,
                         **{k: stats(v) for k, v in samples.items()}})
    print(json.dumps(out["sizes"][-1]), file=sys.stderr, flush=True)
code = c.close()
if isinstance(c, Stdio):
    out["stdio_exit_code_after_disconnect"] = code
out["project"] = project
print(json.dumps(out, indent=1))
