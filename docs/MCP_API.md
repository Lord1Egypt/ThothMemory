# MCP API

Endpoint: `http://127.0.0.1:7319/mcp`, authenticated Streamable HTTP using the official
Go SDK. Stateless request handling reconnects without an in-memory session dependency.
Agent sessions are durable database records, distinct from transport connections.
Stdio bridge exposes the same tools through SDK transports. Initialize, list tools,
then call a tool; `thothmemory doctor` performs a genuine SDK handshake and discovery.

Names use `memory_` for consistent client compatibility. Every response contains
`{"data": ...}` in both structuredContent and text content, with stable entity IDs.
Tool schemas reject unknown keys, invalid types, enums, missing fields and bounds.
No unrestricted query/SQL, shell, file access or administrative tools exist.

| Tools | Inputs / behavior |
|---|---|
| memory_list_projects | limit, offset; only authorized projects |
| memory_create_project | name, repository; canonical HTTPS/SSH remote or local UUID |
| memory_get_project_context / summary | project_id, optional current git_revision |
| memory_get_health | database/MCP readiness |
| memory_start_session | project_id, operation_id, agent, git_revision; optional branch |
| memory_checkpoint_session / finish_session | session_id, expected_revision, git_revision, body |
| memory_get_latest_handoff | project_id |
| memory_list_tasks / get_task | project_id; task_id/state/pagination where appropriate |
| memory_create_task | project_id, operation_id, title; optional depends_on UUID array |
| memory_claim_task / release_task | task_id, expected_revision; 30 minute lease |
| memory_update_task | task_id, expected_revision, state; revision/evidence gates |
| memory_append_event | body; optional task/session/agent/Git revision |
| memory_record_decision / issue / observation | title, body; evidence_id, supersedes, task/session |
| memory_attach_evidence | source_kind, locator, git_revision, outcome, body; task/digest |
| memory_record_test_result | same, fixed test source kind |
| memory_get_recent_activity | project_id, task filter and pagination |
| memory_search | query/exact_id, task/agent/evidence, after/before RFC3339, current revision |

All project writes except natural-key project creation require operation_id (8–100
characters). UUIDs are validated; Git revisions must be full 40/64 hex characters.
Body maximum 8,000, title 500, locator 2,000, query 500. Default limit 20, maximum
100; offset maximum 10,000. Context limits tasks to 12, decisions/issues and evidence
to 8 each, sessions to 4, handoff to 1, and truncates long bodies with retrieval IDs.
Search returns provenance, ranking, superseded status and stale=true for a different
recorded revision. Without a supplied current revision, stale is unknown (null).
For intentional cross-project searches, list authorized projects then query each.

Errors use MCP isError with predictable messages. Authentication errors use HTTP 401;
Host/Origin violations use 403. Tool discovery hides write tools for read credentials,
and the application independently checks write/project permissions. A conflict means
reload/reconcile, while a transport timeout means retry the same operation/payload.

Owner-only evidence review (never MCP):

```sh
docker compose run --rm migrate attest PROJECT_UUID EVIDENCE_UUID OPERATION_ID \
  'Reviewed the linked evidence and its exact revision'
```

Inspect the evidence before attesting. Agent inferences and reported output must
never be upgraded to independently verified facts by assumption.
