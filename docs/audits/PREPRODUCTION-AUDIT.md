# Pre-production audit — 2026-10-12

## Assessment

**READY WITH LIMITATIONS for an owner-operated trial of the patch candidate.**
This is an isolated `0.1.2-rc.1` candidate, not a released or installed upgrade.
Do not interpret this assessment as approval to deploy automatically. OpenCode legacy
is tested; its normal 1.17.13 runtime rejects V2 keys. **Published 0.1.1 Docker arm64 is NOT READY:** its manifest contains an x86-64 binary.
The standalone arm64 release binary is correctly AArch64. Candidate architecture fixes
are validated separately; this audit does not overwrite released tags.
A genuine semantic handoff
between three independently hosted commercial models remains an owner acceptance step.

Release baseline: `v0.1.1`, source `76176f761f0ade77175d30a5d9add0ffdba5dfd8`.
Audit branch starts at reviewed main `88a4a3bf419dce13994b56063f5c925160ee63f2`;
main's subsequent documentation/workflow changes are expected, not source tampering.
Core fixes: `08d5d4c`; OpenCode failure-reporting fix: `dd63b5a`; Windows atomic ACL refinement: `87431ed`; populated upgrade regression: `e562df9`;
real architecture build/probe fix: `599999f`.
Candidate changes are reviewable through this branch's Git history. No existing tag,
Docker Hub image, production configuration, database or agent profile was replaced.

At inspection start, the reported production endpoint was unavailable and its two
containers were already stopped. They were deliberately left stopped. Testing used
unique Compose projects, loopback ports, named volumes, synthetic credentials and
isolated client homes. Production availability is an owner action, not a test PASS.

## Inspected scope

Reviewed `cmd/`, all `internal/store`, `internal/gateway`, `internal/cli`, embedded UI,
migrations 001–004 and new 005, all tests/scripts, Dockerfile, Compose, both installers,
Windows launcher, AGENTS.md, CLAUDE.md and all three GitHub workflows. Reconciled
README, ARCHITECTURE, DATABASE_SCHEMA, MCP_API, SECURITY, AGENT_MEMORY_PROTOCOL,
BACKUP_RESTORE, VALIDATION, PERFORMANCE, DELIVERY, DEMONSTRATION and TROUBLESHOOTING.
Historical delivery measurements remain historical, not new audit observations.

## Confirmed defects and fixes

| Finding | Risk / impact | Reproduction and minimal correction |
|---|---|---|
| Published arm64 container contains x86-64 executable | High: native ARM deployment can fail with exec format error or rely on unintended emulation | Direct child manifest extraction confirmed ELF machine 62, expected 183. Remove TARGETARCH default and verify ELF architecture before migrations/runtime smoke; add both-platform validation to PR CI. |
| Setup recreated removed default principals | High: revoked client regained authorization after setup | Red regression before fix; existing `tokens.json` is now authoritative. Narrowing and revocation survive setup and explicit app reload. |
| Windows Go file modes did not protect inherited ACLs | High when installing/backing up in shared directories: other local users could read secrets | Native baseline inherited Everyone read ACE. Explicit protected owner DACL at file creation for secrets, configuration backups and dump archives; private directory ACL before initialization. Native regression and full Windows setup pass. |
| Global instructions/hooks ignored client home overrides | Medium: wrong profile targeted during isolated/global onboarding | Regression with CODEX_HOME, CLAUDE_CONFIG_DIR and XDG_CONFIG_HOME; shared ClientHome resolver now applies consistently. |
| Search omitted body substrings, commit prefixes and equivalent Unicode | Medium: relevant engineering memory missed | Four fixture failures before, all ten pass after additive migration 005 and combined English/simple/NFC literal search. |
| Resume briefing omitted incremental checkpoints | Medium: recoverable progress hidden after a missing final hook | MCP disconnect without finish reproduced; briefing now includes four observations tied to sessions and explicit incomplete/finished status. |
| Briefing could split a UTF-8 character | Low: damaged multilingual context | Odd byte boundary regression; truncation now backs up to a rune boundary. |
| Windows bootstrap extracted the default image despite `.env` image selection | Medium: CLI/server mismatch on upgrade | Stub Docker regression failed on baseline; bootstrap now resolves image/port overrides consistently. Native repeat setup also passed. |
| OpenCode lifecycle write errors were silently discarded | Medium: user could miss unsynced metadata | Missing-binary callback regression failed before fix; now emits a sanitized warning through documented client logging (stderr fallback), without logging tokens, paths or subprocess errors. |
| V2 registration implied unproven runtime compatibility | Medium: first-run OpenCode failure | Actual 1.17.13 `run` rejects permissions/providers keys. V2 registration now fails before writing unless explicitly opted into unverified support. |
| Manual publication retry could overwrite an already released version | Medium: release immutability | Added draft-only checks before build and promotion, and serialized publication. No publication workflow was triggered by this audit. |

Local `permissions` CLI provides explicit project allowlists, read-only access and
revocation with backups/consent. It does not rotate tokens or reload the live service.
The default remains documented single-owner trusted-workspace access, with distinct
agent tokens. Wildcard credentials can intentionally access every project; they
cannot prevent an authorized agent choosing the wrong project UUID. Scoped credentials
are the mitigation when that trust is inappropriate. A scoped principal cannot create
new projects; bootstrap them with a trusted local credential before narrowing access.

## Verification evidence

| Gate | Result / evidence |
|---|---|
| Formatting, modules, vet, unit/race | [Local quality output](evidence/quality.txt); integration evidence below includes executed DB tests. |
| Dependency analysis | [govulncheck](evidence/govulncheck.txt): no vulnerabilities found at audit time. Container scan is separately gated by PR CI. |
| PostgreSQL and MCP | [Executed race integration suite](evidence/integration-tests.txt): clean/repeat migrations, upgrade preservation/checksum mismatch, partial migration rollback, retry/concurrency/claims/budget rollback, immutable history, evidence gates, real SDK handshake and calls. |
| Authorization | [Policy reload test](evidence/authorization.json); every project-scoped tool and cross-project task/session/evidence associations exercised in `TestAuditProjectAssociationsAndExpiredClaims`. Read-only mutation rejection and revoked 401 verified. |
| Recovery | [Full table count/content comparison](evidence/recovery.json): separate DB restore, activation, authenticated MCP read/write, second backup contains activated sentinel, source retained, corruption rejected before DB creation. Four persisted sessions and handoffs included. |
| Lifecycle / clients | [Compatibility matrix](AGENT-COMPATIBILITY-MATRIX.md): real clients distinguished from simulated engineering actors and directly invoked callbacks. |
| Search | [Before/after fixture](MULTILINGUAL-SEARCH.md), including Arabic and technical identifiers. |
| Deployment | Native Windows CMD/PowerShell clean build, repeat credential hash, actual MCP demo and dump/restore passed; Linux/WSL published-image clean install and [candidate repeat setup/restart/port rejection](evidence/resilience.json) passed. Chromium unlock/read/search and mobile viewport passed. |
| Performance | [Controlled workloads and limitations](PERFORMANCE-REGRESSION.md). No new persistent service or polling worker. |
| Registry | [OCI index](evidence/release-index.json), [platform attestation fields](evidence/attestations.json). Anonymous pull and amd64 deployment passed. **Published arm64 binary verification failed**: [binary hashes/architecture](evidence/published-binaries.json). [Fixed candidate](evidence/candidate-platform-smoke.txt) passes ELF checks plus native amd64/emulated arm64 MCP execution. |

`go test` without THOTH_TEST_DATABASE_URL skips database tests; it is not sufficient
evidence alone. The integration log explicitly shows those tests executed. The
four-session test models engineering reasoning; PostgreSQL, authentication, MCP and
revision handling are real. It does not prove a model will always obey instructions.

## Rejected hypotheses / retained design

- English text search did retrieve whole Arabic tokens; the defect was incomplete
  substring/identifier/Unicode coverage, not inability to store or find all Arabic.
- No cross-project bypass, read-only write bypass or MCP attestation path was found.
  Composite FKs and application authorization both remain necessary.
- A disconnected transport is not proof that a durable agent session completed or
  crashed. Incomplete is deliberately honest; no permanent polling was introduced.
- SHA-256 detects accidental corruption, not malicious archive replacement. Dumps
  remain unencrypted: use protected/encrypted storage and trusted transfer.
- The previous arm64 smoke PASS was insufficient: on an x86 host a mislabeled x86
  binary ran natively. Real MCP success did not establish instruction-set correctness.
  The strengthened smoke test inspects ELF before executing either architecture.

## Remaining limits and next action

Owner must review the PR and execute [first-use acceptance](FIRST-RUN-ACCEPTANCE.md).
No new release, latest promotion, merge or live migration is authorized here.
Native Linux Engine (outside WSL2/Docker Desktop), native Windows arm64, long-duration
soak behavior, real provider quota exhaustion and V2 normal runtime remain untested
or unsupported. Session-final hooks cannot guarantee semantic handoffs after termination.
Large-project substring scans and migration 005 table locks require further measurement
before installations much larger than the tested dataset. Migration 005 is additive;
old images are not a supported in-place schema downgrade. Keep a pre-upgrade backup.

## Protected installation observation

Production secret/config files and both original stopped container identities were
retained. No audit registration appeared in the normal Claude profile. Its preference
file changed concurrently; nearby client backups differ only in runtime tip/queue
counters. The audit did not restore or edit those counters, and does not claim a
byte-identical snapshot for that independently changing file. Protected production
credentials remained byte-identical; test-only temporary authenticated material was
removed during cleanup. [Protection check](evidence/protection.json) records these distinctions.

## Publication integrity finding

Both registry tags still resolve to the recorded OCI digest and contain expected
platform/attestation entries. SLSA source matches the peeled annotated release tag,
and all four standalone release checksums pass. The amd64 image executable is
byte-identical to its release artifact. The arm64 image executable is instead
byte-identical to that amd64 executable; direct inspection of the arm64 child
manifest confirmed the mismatch. This is a build configuration defect, not evidence
of source tampering. The old smoke output is retained as explicitly inadequate
evidence. Candidate CI now includes ELF validation and real MCP smoke for both
architectures. A new authorized release is required to distribute the correction;
existing v0.1.1 remains immutable. [Release verification](evidence/publication.json).

## Cleanup and remote regression follow-up

[Selective cleanup](evidence/cleanup.json) removed 27 audit containers, nine named
volumes, nine networks and six candidate image tags. No audit Compose containers
remain. Original stopped production containers retained identical status/finish
timestamps; all four unrelated running workloads remain running. No global prune
was used. Nine stopped containers, three volumes and three networks from earlier
temporary ThothMemory validation installations were also removed after confirming
their exact project labels and temporary-directory configuration. Total: 36 test
containers, 12 named test volumes, 12 test networks and six candidate image tags.
Further container acceptance runs execute on disposable CI runners.

The first PR CI run exposed PostgreSQL startup timing: socket readiness could
observe the temporary initialization server, then `createdb` raced its shutdown.
Smoke tests, the integration helper and Compose now wait on TCP readiness from the final server, and the
smoke timeout fails explicitly. Smoke cleanup also removes its database's anonymous
volume and temporary directory. A Windows hosted-runner test compared textual SID
representations and rejected Windows' valid local Administrator alias (`LA`); both
expected and actual descriptors are now canonicalized before comparison. This is
a test portability correction, without weakening the owner-only DACL.
