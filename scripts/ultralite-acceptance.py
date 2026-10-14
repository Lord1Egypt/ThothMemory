#!/usr/bin/env python3
"""Post-publication acceptance for a downloaded UltraLite executable (Linux; disposable data).

  ultralite-acceptance.py BINARY [--previous OLD_BINARY] [--sums SHA256SUMS --asset NAME]

No Docker, no model, no credentials. Semantic writes are deterministic MCP stdio calls, not
model behavior. Prints one PASS/FAIL line per gate and exits non-zero on any failure.
"""
import argparse, concurrent.futures, hashlib, json, os, stat, subprocess, sys, tempfile, time
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument("binary")
p.add_argument("--previous")
p.add_argument("--sums")
p.add_argument("--asset")
a = p.parse_args()
binary = str(Path(a.binary).resolve())
failures = []


def gate(name, fn):
    try:
        print("PASS ", name, "-", fn() or "")
    except Exception as e:  # noqa: BLE001 - report every failed gate
        failures.append(name)
        print("FAIL ", name, "-", repr(e))


def cli(exe, data, *args, cwd=None, check=True):
    r = subprocess.run([exe, "--data", str(data), *args], cwd=cwd, text=True, capture_output=True)
    if check and r.returncode:
        raise RuntimeError("%s failed: %s" % (args, r.stderr.strip()[:300]))
    return r.stdout


class Session:
    def __init__(self, exe, data, token="agent_token"):
        self.proc = subprocess.Popen([exe, "mcp-stdio", "--data", str(data), "--token-file", str(Path(data) / ".secrets" / token)],
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.seq = 0
        self.rpc("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "ultralite-acceptance", "version": "1"}})
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
        self.proc.stdin.flush()

    def rpc(self, method, params):
        self.seq += 1
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": self.seq, "method": method, "params": params}) + "\n")
        self.proc.stdin.flush()
        r = json.loads(self.proc.stdout.readline())
        if "error" in r:
            raise RuntimeError(r["error"])
        return r["result"]

    def tool(self, name, args):
        r = self.rpc("tools/call", {"name": "memory_" + name, "arguments": args})
        if r.get("isError"):
            raise RuntimeError(r["content"][0]["text"])
        return json.loads(r["content"][0]["text"])["data"]

    def close(self):
        self.proc.stdin.close()
        return self.proc.wait(timeout=30)


def listeners(pid):
    inodes = set()
    for fd in Path("/proc/%d/fd" % pid).iterdir():
        try:
            t = os.readlink(fd)
        except OSError:
            continue
        if t.startswith("socket:["):
            inodes.add(t[8:-1])
    found = []
    for f, tcp in (("tcp", 1), ("tcp6", 1), ("udp", 0), ("udp6", 0)):
        for line in Path("/proc/net/" + f).read_text().splitlines()[1:]:
            c = line.split()
            if c[9] in inodes and (not tcp or c[3] == "0A"):
                found.append((f, c[1]))
    return found


HEAD = "a" * 40
REPO = "https://github.com/thothmemory-example/ultralite-acceptance"
with tempfile.TemporaryDirectory(prefix="thoth-ul-accept-") as tmp:
    root = Path(tmp)
    data = root / "memory"
    proj = root / "project"
    proj.mkdir()
    subprocess.run(["git", "init", "-q", str(proj)], check=True)
    subprocess.run(["git", "-C", str(proj), "remote", "add", "origin", REPO], check=True)
    state = {}

    def checksum():
        if not (a.sums and a.asset):
            return "skipped: no --sums"
        want = {l.split()[1].lstrip("*"): l.split()[0] for l in Path(a.sums).read_text().splitlines() if l.strip()}[a.asset]
        got = hashlib.sha256(Path(binary).read_bytes()).hexdigest()
        assert got == want, (got, want)
        return got[:16]

    def install():
        cli(binary, data, "init", cwd=proj)
        cli(binary, data, "init", cwd=proj)  # idempotent
        return "version " + subprocess.run([binary, "version"], text=True, capture_output=True).stdout.strip()

    def perms():
        bad = []
        for path in [data, data / ".secrets"] + [x for x in data.rglob("*") if x.is_dir()]:
            if stat.S_IMODE(path.stat().st_mode) & 0o077:
                bad.append(str(path))
        for path in data.rglob("*"):
            if path.is_file() and stat.S_IMODE(path.stat().st_mode) & 0o077:
                bad.append(str(path))
        assert not bad, bad
        return "no group/other access under the data directory"

    def doctor():
        out = cli(binary, data, "doctor", cwd=proj)
        return out.strip().splitlines()[-1][:80] if out.strip() else "ok"

    def memory():
        s = Session(binary, data)
        tools = [t["name"] for t in s.rpc("tools/list", {})["tools"]]
        assert "memory_record_idea" in tools and "memory_search" in tools, tools
        pid = s.proc.pid
        lis = listeners(pid)
        assert not lis, lis
        pr = s.tool("create_project", {"name": "Acceptance", "repository": REPO})["id"]
        state["p"] = pr
        t = s.tool("create_task", {"project_id": pr, "operation_id": "ul-task-0001", "title": "Acceptance task", "git_revision": HEAD})["id"]
        s.tool("record_idea", {"project_id": pr, "operation_id": "ul-idea-0001", "title": "Idea", "body": "Small idea: ultralite persistence marker alpha."})
        s.tool("record_note", {"project_id": pr, "operation_id": "ul-note-0001", "title": "Note", "body": "Note: ultralite persistence marker bravo.", "git_revision": HEAD})
        assert s.tool("search", {"project_id": pr, "query": "alpha"}), "search found nothing"
        rc = s.close()
        assert rc == 0, rc
        assert not Path("/proc/%d" % pid).exists()
        return "%d tools; no listening socket; idea, note, task written; exit 0" % len(tools)

    def reopen():
        s = Session(binary, data)
        hits = s.tool("search", {"project_id": state["p"], "query": "bravo"})
        assert hits, hits
        ctx = s.tool("get_project_context", {"project_id": state["p"], "git_revision": HEAD})
        assert ctx is not None
        s.close()
        return "new process retrieved the earlier note"

    def concurrent_writes():
        def worker(n):
            s = Session(binary, data)
            for i in range(15):
                s.tool("record_note", {"project_id": state["p"], "operation_id": "ul-c%d-%02d-xxxx" % (n, i), "title": "c%d" % n, "body": "concurrent writer %d note %d" % (n, i), "git_revision": HEAD})
            return s.close()
        with concurrent.futures.ThreadPoolExecutor(3) as ex:
            assert all(rc == 0 for rc in ex.map(worker, range(3)))
        s = Session(binary, data)
        n = len(s.tool("search", {"project_id": state["p"], "query": "concurrent"}))
        s.close()
        assert n > 0, n  # search output is capped; every writer exited 0 above and doctor verifies integrity
        cli(binary, data, "doctor", cwd=proj)
        return "3 concurrent MCP processes x 15 writes, no error, doctor ok (search returned %d)" % n

    def backup_restore():
        snap = root / "snap.db"
        cli(binary, data, "backup", str(snap))
        assert stat.S_IMODE(snap.stat().st_mode) & 0o077 == 0, oct(snap.stat().st_mode)
        s = Session(binary, data)
        s.tool("record_note", {"project_id": state["p"], "operation_id": "ul-after-backup", "title": "post", "body": "written after backup: zulu", "git_revision": HEAD})
        s.close()
        cli(binary, data, "restore", str(snap), "--confirm")
        s = Session(binary, data)
        assert s.tool("search", {"project_id": state["p"], "query": "bravo"})
        assert not s.tool("search", {"project_id": state["p"], "query": "zulu"}), "restore kept post-backup data"
        s.close()
        cli(binary, data, "doctor", cwd=proj)
        return "restored snapshot; later write rolled back; doctor ok"

    def upgrade():
        if not a.previous:
            return "skipped: no --previous"
        prev = str(Path(a.previous).resolve())
        d2 = root / "upgrade"
        cli(prev, d2, "init", cwd=proj)
        s = Session(prev, d2)
        pr = s.tool("create_project", {"name": "Upgrade", "repository": REPO + "-upgrade"})["id"]
        s.tool("record_note", {"project_id": pr, "operation_id": "ul-upg-0001", "title": "kept", "body": "survives executable upgrade: yankee", "git_revision": HEAD})
        s.close()
        s = Session(binary, d2)
        assert s.tool("search", {"project_id": pr, "query": "yankee"})
        s.close()
        cli(binary, d2, "doctor", cwd=proj)
        return "data written by %s read by %s" % (prev.split("/")[-1], binary.split("/")[-1])

    def no_daemon():
        r = subprocess.run(["pgrep", "-f", str(data)], text=True, capture_output=True)
        assert not r.stdout.strip(), r.stdout
        return "no process left referencing the data directory"

    for name, fn in [("checksum", checksum), ("fresh init (twice)", install), ("owner-only permissions", perms), ("doctor integrity", doctor),
                     ("MCP handshake, discovery, writes, no listener, clean exit", memory), ("cross-session retrieval", reopen),
                     ("concurrent access", concurrent_writes), ("backup and restore", backup_restore),
                     ("upgrade preserves data", upgrade), ("no background daemon", no_daemon)]:
        gate(name, fn)

print("%d failed" % len(failures) if failures else "all gates passed")
sys.exit(1 if failures else 0)
