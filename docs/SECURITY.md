# Security design and operational limits

## Default boundary

Only 127.0.0.1:7319 is published. PostgreSQL has no host port. The dashboard shell and
minimal readiness status are public to local browsers; all memory reads require a
bearer token. Host and exact Origin checks reject DNS rebinding and cross-origin
requests. HTTP bodies/headers and request times are bounded. UI uses textContent,
CSP and no remote scripts. Tokens are never placed in URLs, images or logs.

Setup generates 256-bit tokens and independent database passwords with crypto/rand.
Files live in an owner-only .secrets directory (Windows ACL / Unix mode 700); mounted
secret files are readable by the non-root container UID. Protect the directory and
backups. Docker administrators can inspect all container data and are trusted.
The application is non-root, read-only, drops capabilities and uses no Docker socket.
Compose's database logins separate schema migration owner from runtime DML.

## Authorization

.secrets/tokens.json defines id, token, write and projects (UUID allowlist, or '*').
Initial local-agent, codex, claude and opencode credentials explicitly cover all
projects; local-reader is read-only. To isolate clients, narrow their project lists,
then restart the application. Read-only credentials discover only read tools, and
writes are also checked in the core. Owner=true is rejected on the HTTP service.
Use distinct principal IDs for independent claim ownership. Possession of a token
establishes identity; an agent-supplied name is a reported label, not authentication.
No endpoint provides SQL, command execution, filesystem reads, DB admin or secrets.

## Evidence and history

Evidence from MCP is always reported. A claimed human observation is still reported.
Only local privileged CLI attestation can establish owner trust. Verified tasks need
passing, owner-attested test/CI evidence for the exact task and full revision;
released tasks need similarly attested release evidence. These checks do not execute
external tests or validate remote Git state. The next agent must compare HEAD.
Historical tables reject updates/deletes. Composite foreign keys prohibit linking
another project's tasks/sessions/evidence. Every write records the principal.

## Stored instructions

Memory is untrusted data. Tool descriptions, server guidance, agent instructions
and the dashboard identify that boundary. No server component executes stored
content. An LLM may still mishandle hostile text: prompt instructions cannot provide
a complete prompt-injection defense. Agent host permissions remain essential.

## Capacity and recovery

Each project defaults to 100,000 events. Each mutation consumes one event slot in
the same transaction; replays consume none. History is never silently pruned. At the
budget limit writes fail until the owner reviews disk space/backups and explicitly
raises the project's event_budget through administrative PostgreSQL access. Queries
are paginated and bounded; Compose logs rotate at 10MB × 3 files per service.

Do not expose this HTTP service through a tunnel or bind public ports. Future remote
use needs TLS, suitable authorization lifecycle and a separate security review.
Backups are sensitive and unencrypted: use OS disk encryption or encrypt them before
moving to untrusted storage. SHA-256 detects accidental corruption, not a malicious
replacement of both archive and checksum. Restore only trusted archives.
