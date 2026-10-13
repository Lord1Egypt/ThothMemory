# Sequential release plan (prepared 2026-10-13)

State when written: `main` = `88a4a3b` (v0.1.1 plus docs), unprotected, no rulesets. PR #9
`patch/0.1.2-arm64-fix` (head `ef4867f`), PR #2 `audit/preproduction-0.1.2` (head `ecb8415`), PR #8
`nextgen/full-and-mini`. All Draft or unmerged. Production containers and volume untouched; verified cold
backup at `~/ThothMemory-production-backup-20261013/`. Nothing in this plan has been executed.

## The first approval gate

> **Gate 1: merge PR #9 into `main`** (merge commit, no squash, so the commit that CI tested is
> in history). It changes 15 files (14 plus its notes), no migration, no dashboard, no SQL, no
> dependency. A merge **does not publish anything**: `ci.yml` runs on push to `main` and
> publishes nothing; publication starts only from a `vX.Y.Z` tag (Gate 2, separate approval).
> Rollback: `git revert -m 1 <merge commit>` (a new commit, no history rewrite).

## 1. PR #9 review: safe to merge

| Check | Result |
|---|---|
| Base | `main` `88a4a3b`, unmoved; merge-base equals main; GitHub: mergeable, state clean |
| Scope | 15 files: Dockerfile ARG fix, ELF-checking smoke test, CI step, version strings 0.1.1 → 0.1.2, compose/setup/env defaults, changelog, release notes. No `migrations/`, `internal/web`, `go.mod/go.sum`, SQL or third-party changes |
| Architecture check | `scripts/smoke-platforms.sh` reads each image's executable and fails unless ELF `e_machine` is 62 (amd64) or 183 (arm64); locally verified (arm64 under QEMU), and CI step "Verify executable architectures and exercise both candidate servers" is green |
| CI | green on [run 38441187422](https://github.com/Lord1Egypt/ThothMemory/actions/runs/38441187422): `test`, `windows`, GitGuardian |
| Tags | `v0.1.0`, `v0.1.1` unchanged locally and on origin |
| Compatibility | Runtime behavior identical; only version strings and default image tag change |

Residual risks: the arm64 smoke in CI runs under QEMU, not on ARM hardware; `compose.yaml` and setup
scripts default to image tag `0.1.2`, which does not exist until the release publishes, so merging
without releasing leaves `main`'s installers pointing at an unpublished tag (use `THOTH_IMAGE` or release
promptly).

## 2. v0.1.2 release checklist

What the existing workflows do, so nothing surprises you: **pushing tag `v0.1.2` is the publish action.**
`release.yml` (on tag) runs CI, builds the 4 native binaries and creates a *draft* GitHub release;
when it succeeds, `docker.yml` (workflow_run) builds `linux/amd64,linux/arm64`, **pushes
`lord1egypt/thoth-memory:candidate-<sha>`**, pulls it by digest, runs `scripts/smoke-platforms.sh` and
`scripts/acceptance.sh`, and only then tags `0.1.2` and **`latest`** and publishes the release.

Before Gate 2 (tag):
- [ ] Gate 1 done; fresh CI on the merge commit in `main` is green.
- [ ] Docker Hub secrets exist (`DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`: present).
- [ ] Record the rollback target: index `sha256:0edd6990289b12099a5c4d44525e9e5bf03fc52934ba09dfb43f293ed4bf9e63` (v0.1.1, amd64 content in both child manifests).
- [ ] `git tag -a v0.1.2 -m "ThothMemory 0.1.2: ARM64 container fix"` on the merge commit (annotated).

After the workflow (verify before announcing; each command is read-only):
- [ ] `docker buildx imagetools inspect lord1egypt/thoth-memory:0.1.2` shows `linux/amd64` and `linux/arm64`; record the index digest from `IMAGE_DIGEST.txt`.
- [ ] For each platform: pull **by digest**, `docker create`, copy `/usr/local/bin/thothmemory`, check ELF bytes 18-19: `3e 00` (62) for amd64, `b7 00` (183) for arm64, and different SHA-256s.
- [ ] `latest` resolves to the same digest as `0.1.2`; `0.1.1` still resolves to `0edd6990…`.
- [ ] Release assets: 4 binaries + `SHA256SUMS` + `IMAGE_DIGEST.txt`; checksums verify; provenance attestations present (`imagetools inspect --format '{{json .Provenance}}'`).
- [ ] On a native ARM64 host if available: run the smoke test against the digest (CI does it under emulation).

Rollback:
- Before tag: revert the merge (new commit). Nothing published.
- After tag, image problem: re-point tags to the previous digest (needs your Docker Hub credentials):
  `docker buildx imagetools create -t lord1egypt/thoth-memory:latest lord1egypt/thoth-memory@sha256:0edd6990…e63`
  (this restores the arm64 defect for `latest` users) and `gh release edit v0.1.2 --draft`.
  Do **not** delete or move the `v0.1.2` tag or overwrite `0.1.1`; ship `0.1.3` instead.
- Users: pin `THOTH_IMAGE=lord1egypt/thoth-memory:0.1.1` (amd64 only). No schema change, so data is unaffected either way.

## 3. What PR #2 needs after PR #9 merges

Trial merge of #9 into #2's branch (not committed, discarded): **8 conflicts, all mechanical**, no logic.

| File | Conflict | Resolution |
|---|---|---|
| `Dockerfile` | `LABEL …version` (0.1.2-rc.1 vs 0.1.2) | set the new number (below) |
| `cmd/thothmemory/main.go`, `internal/cli/client.go`, `internal/gateway/http.go`, `internal/gateway/tools.go`, `internal/store/operations.go` | version string literals | set the new number (9 files in total carry `0.1.2-rc.1`, incl. 4 docs/evidence) |
| `CHANGELOG.md` | both add a 0.1.2 section | keep #9's section as 0.1.2; rename #2's "0.1.2 candidate" to the new number; **drop #2's duplicated ARM64 bullet** |
| `setup.ps1` | #2 rewrote the image/port logic, #9 only changed the default tag | keep #2's block, with the new default tag |

Duplicated fixes that merge cleanly and need no action: the Dockerfile `ARG TARGETOS/TARGETARCH`
change, `scripts/smoke-platforms.sh` (final-server readiness and ELF checks), and the `ci.yml`
architecture step. After rebasing, #2's diff shrinks to its real content (credentials, search
migration 005, Windows ACLs, client fixes). **Proposed numbering**: #9 = `0.1.2`; #2 = `0.1.3`
(additive migration 005, no breaking change); next-generation Full = `0.2.0`.
Order: merge #9, then rebase or merge `main` into #2, resolve the 8 files, **run fresh CI**, then Gate 3.

## 4. GitGuardian finding on PR #8

Correction from the GitHub check-run API: incident **38171534** is a **Generic Password**
in `pkg/memorycore/memorycore_test.go:66`, introduced by `1aeacc34`. It is an explicitly
synthetic secret-rejection fixture, still present at `d391f0b`; the current correction
constructs it at runtime and retains the rejection/error-sanitization assertions.
The earlier AWS attribution below was a hypothesis, not the detector result.
The earlier scan of added lines reported:

- **One AWS-access-key-shaped string**, and it is exactly AWS's published *documentation example* key (verified by
  hash equality, not by reading it). It is in the test fixtures for credential refusal
  (`internal/contract/contract.go`), added in `e41115e`. This was not the reported trigger.
- Other candidates are synthetic: a credential-shaped password and token built by repeating a short sequence
  in test code; 64-hex strings that are SHA-256 digests (image digests, binary checksums).
- **No real secret is involved.** All 2,132 distinct long tokens in the added lines were compared by SHA-256 to
  the values in the production and audit `.secrets`, `.env` and `client*.json` files: 2 matches, both
  low-entropy configuration text (a path-like name and a short identifier, 2.9 to 3.5 bits per character versus
  4 or more for random tokens). A separate check of all 7 real credential files (tokens and passwords) found
  **none appears anywhere in any branch's history**. No credential files are tracked.
- Current code builds these fixtures at runtime, so the tip has no secret-shaped literals. The history still does.

Recommendation: review incident **38171534** as a **false positive** (synthetic password test fixture) in the GitGuardian
dashboard. A history rewrite is not needed and I have not done one. Needs you (dashboard access). The same check
passes on PR #9 (and passed on PR #2) because they contain no such fixtures.

## 5. Integration plan for PR #8 (after #2)

PR #8 is stacked on #2's branch, so it contains #2's commits. Simulated merge of #8 into `main + #9`:
**8 conflicts, the same mechanical version-string files** (plus `gateway/tools.go`, which #8 moved to
`internal/mcptools/tools.go`); no logic conflict.

1. Gate 1: merge #9 (v0.1.2 released via Gate 2, ideally first).
2. Gate 3: bring #2 onto `main` as in section 3 (version 0.1.3); fresh CI; merge; release 0.1.3 or hold it for 0.2.0.
3. Retarget PR #8 to `main` (GitHub offers it automatically when #2's branch is deleted, so keep that branch until
   #8 is retargeted). Rebase #8 onto `main`; resolve the version files (Full `0.2.0-rc.1` is already in
   `internal/memory/domain.go`; I just aligned its Dockerfile label and CLI client to it). Migrations stay
   `001-005` (#2) then `006`, `007` (#8); both are additive; run the 006/007 upgrade on a clone of the
   production volume again, **never on production**.
4. After integration, fresh evidence per edition (not carried over):
   - Full: `scripts/test-integration.sh` (PostgreSQL + MariaDB + SQLite), upgrade test from a v0.1.1 clone through 007, dashboard check on a populated clone.
   - Mini: images build for amd64 and arm64; `mini-acceptance.py` and `mini-lifecycle.sh` on host; **repeat the phone run** with a digest-pinned image in a registry the phone trusts.
   - UltraLite: native CI on Linux, Windows and arm64; rebuild twice for reproducibility; re-record SHA-256s.
   - Automatic memory: `TestAutomaticMemoryEndToEnd`, `scripts/real-client-handoff.sh`, re-run real Claude hooks; Codex hooks only after you trust a project and approve them in Codex.
5. Releases are separate decisions: Full 0.2.0 image, Mini 0.1.0 image (new repository name and tags, e.g. `thoth-memory-mini`, not `latest` of the existing repo), UltraLite 0.1.0 binaries (consider signing).

## 6. Android Mini acceptance: reconfirmed

Every figure in `docs/ANDROID_GATE.md` was cross-checked against the raw logs in
`docs/audits/evidence/android/` on 2026-10-13: 11/11 MCP gates, 14 passed 0 failed lifecycle, 51.4 s
install, 1,609 MiB virtual size, abstract-socket transport, ThothDock 0.2.1, SIGKILL recovery, bench medians.
One mistake was found and fixed: the idle-tree PSS had been written in KiB-as-MiB; correct values are 121.5
MiB before and 184.5 MiB with Mini (the measured delta also includes the owner's own app varying).

Remaining runtime limitations (unchanged and not hidden):
1. Short run only: no multi-hour soak, no Doze or background-kill observation, no memory-pressure test.
2. Process budget: Mini adds 3 processes; ThothDock's Process Guard warned at 21 processes; Android's device-wide
   cap is not readable by apps. Co-running several containers can end the engine.
3. The image was pulled through a temporary tunnel (closed). No public digest-pinned image exists yet.
4. SELinux audit logs unreadable as shell: "no denials" is not claimed. Battery not measured.
5. Alpine arm64 image never ran on the phone; glibc is the validated variant.
6. TCP fallback mode (if a runtime cannot use abstract sockets) opens a loopback database port reachable by any
   local app (password-protected); the default abstract-socket mode does not.
7. ThothDock isolation is functional, not a security boundary: same-UID containers can reach each other and the
   device loopback; no cgroup limits, so a runaway MariaDB is not capped by the engine.
8. Latency from PRoot is 2 to 4 times UltraLite's native numbers; fsync cost depends on the phone's flash.

## Exact remaining steps, in order

| # | Action | Who | Publishes? |
|---|---|---|---|
| **1** | **Approve merging PR #9 into `main`** | you | no |
| 2 | Approve tag `v0.1.2` (starts image publication, `latest`, release) | you | **yes** |
| 3 | Mark GitGuardian finding a false positive | you (dashboard) | no |
| 4 | I resolve PR #2's 8 conflicts, renumber to 0.1.3, fresh CI, then you approve its merge | me, then you | no |
| 5 | I retarget and integrate PR #8, re-run all three editions and the Android gate, then you approve | me, then you | no |
| 6 | Approve each further release (Full, Mini image to a trusted registry, UltraLite) | you | yes |
