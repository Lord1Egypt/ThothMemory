#!/usr/bin/env bash
# Real-client handoff for UltraLite: Codex records an idea, Claude Code retrieves it.
# Uses your installed, already-authenticated clients and a DISPOSABLE data directory
# and scratch repository; it edits no client configuration. It spends a small amount
# of model usage (two short sessions). Run it only when you intend that.
#   scripts/real-client-handoff.sh PATH_TO_thothmemory-ultralite
set -euo pipefail
U=$(readlink -f "${1:?path to thothmemory-ultralite}")
for c in codex claude; do command -v "$c" >/dev/null || { echo "$c not installed" >&2; exit 2; }; done
R=$(mktemp -d); trap 'rm -rf "$R"' EXIT; chmod 700 "$R"
mkdir "$R/proj" "$R/data"; cd "$R/proj"
git init -q -b main; git -c user.email=a@b.c -c user.name=n commit -q --allow-empty -m init
git remote add origin https://github.com/example-owner/pocketclaw-notes.git
echo demo > README.md; git add .; git -c user.email=a@b.c -c user.name=n commit -q -m readme
"$U" --data "$R/data" init >/dev/null
"$U" --data "$R/data" instructions --dir "$R/proj" --yes >/dev/null
echo "== Codex: tell it one small idea"
timeout 280 codex exec --skip-git-repo-check -s read-only -C "$R/proj" \
  -c "mcp_servers.thothmemory.command=\"$U\"" \
  -c "mcp_servers.thothmemory.args=[\"mcp-stdio\",\"--data\",\"$R/data\",\"--token-file\",\"$R/data/.secrets/codex_token\"]" \
  "Maybe we can make a SQLite version for PocketClaw." 2>&1 | grep -E '^mcp:|^codex$' | head -20
echo "== Claude Code: a different agent retrieves it"
cat > "$R/mcp.json" <<JSON
{"mcpServers":{"thothmemory":{"type":"stdio","command":"$U","args":["mcp-stdio","--data","$R/data","--token-file","$R/data/.secrets/claude_token"]}}}
JSON
timeout 280 claude -p --strict-mcp-config --mcp-config "$R/mcp.json" --allowedTools 'mcp__thothmemory__*' --max-budget-usd 0.60 \
  "Follow THOTHMEMORY_AGENT_PROTOCOL.md. Using ThothMemory only, say what ideas are recorded and who recorded them." </dev/null
echo "== Stored in memory"
"$U" --data "$R/data" search "$("$U" --data "$R/data" call list_projects | python3 -c 'import json,sys;print(json.load(sys.stdin)["data"][0]["id"])')" PocketClaw
