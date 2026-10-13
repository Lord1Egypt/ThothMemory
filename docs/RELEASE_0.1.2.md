# ThothMemory 0.1.2 (candidate): ARM64 container fix only

**Status: candidate. Not merged, tagged or published.** Source: `patch/0.1.2-arm64-fix`,
based on `main` at `88a4a3b` (v0.1.1 plus documentation). Commit and hashes below.

## The defect (reproduced 2026-10-13)

`lord1egypt/thoth-memory:0.1.1` (OCI index `sha256:0edd6990289b12099a5c4d44525e9e5bf03fc52934ba09dfb43f293ed4bf9e63`):

| Platform pulled by digest | `/usr/local/bin/thothmemory` ELF `e_machine` | SHA-256 |
|---|---|---|
| linux/amd64 | 62 (x86-64) | `28dc690b…b391d8` |
| linux/arm64 | **62 (x86-64), should be 183** | `28dc690b…b391d8`, byte-identical to amd64 |

Cause: `ARG TARGETOS=linux` / `ARG TARGETARCH=amd64` in the Dockerfile overrode BuildKit's automatic
values. The old smoke test passed on an x86 host because the mislabelled binary runs there.
Standalone release binaries were correct and are not affected.

## The fix (14 files, no migration, no behavior change)

- Dockerfile: `ARG TARGETOS` / `ARG TARGETARCH` without defaults (from PR #2 commit `599999f`).
- `scripts/smoke-platforms.sh`: reads each image's executable and fails unless the ELF
  `e_machine` is 62 (amd64) or 183 (arm64), then runs migration, MCP handshake, tool discovery,
  writes and a handoff; waits on the final PostgreSQL server (from `5c14c8d`, smoke part only).
- `.github/workflows/ci.yml`: builds and smoke-tests both platforms on every PR.
- Version strings and default image tag 0.1.1 → 0.1.2; changelog entry.
- Nothing from PR #2's search, credentials, migrations 005, Windows ACL or client changes, and
  nothing from the next-generation branch.

## Verification (this candidate, 2026-10-13)

| Check | Result |
|---|---|
| `scripts/check.sh` (format, vet, race tests) | pass |
| PostgreSQL integration suite (`scripts/test-integration.sh`) | pass (`cli`, `store`, `tests`) |
| `govulncheck ./...` | No vulnerabilities found |
| Image build, linux/amd64 (native) | built; ELF `e_machine` 62; real server smoke PASS |
| Image build, linux/arm64 (**QEMU-emulated**, not native ARM hardware) | built; ELF `e_machine` **183**; real server smoke PASS |
| Candidate executables | amd64 `d7b9c3fa…32633a9`, arm64 `573e4c1c…72d6af` (different, as required) |
| Local image IDs (not registry digests) | amd64 `sha256:39924a52…682de8`, arm64 `sha256:fbe8bd0c…f3a8ea` |

Registry digests, provenance attestations and SBOM exist only after the publication workflow
runs; they are not claimed here. The ARM binary was exercised under emulation only; a native
arm64 run is covered by the GitHub `ubuntu-24.04-arm` runner if you add it to the smoke job.

## Proposed release procedure (all steps need owner approval)

1. Review and merge this PR into `main` (independent of PR #2 and the next-generation PR).
   PR #2 also sets `0.1.2-rc.1`; renumber it to `0.1.3-rc.1`/`0.2.0-rc.1` after this merges.
2. Tag `v0.1.2` on the merge commit. The existing Release workflow validates and drafts.
3. Run the Docker publication workflow for `v0.1.2`; confirm both child manifests' ELF types by digest
   before promoting `latest`. Do **not** retag or modify `0.1.1`.
4. Users on ARM: `docker pull lord1egypt/thoth-memory:0.1.2`. Existing installs keep their data;
   there is no schema change, so rollback is pinning `THOTH_IMAGE` back to `0.1.1` (amd64 only).
5. Production (currently stopped): upgrade only after a verified backup; it is separate from this release.
