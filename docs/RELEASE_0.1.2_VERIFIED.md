# ThothMemory v0.1.2 — published and independently verified

Published 2026-10-13T19:49:49Z:
https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.1.2

The owner explicitly approved PR #9's merge and, conditionally on successful
post-merge CI, the annotated tag/publication/latest promotion. Both conditions
were met. No other PR merge, release or production operation was authorized or
performed in this release sequence.

## Source and workflows

- PR #9 head: `ef4867f2d3fc642dd91cee41c77177fcbda2ba94`, all CI and GitGuardian
  checks green; base unchanged at `88a4a3bf419dce13994b56063f5c925160ee63f2`.
- Merge commit: `c7d2f8dd69a8284058f8e2d69619140ddc2d32e1`, parents base and
  approved head. Its tree exactly equals the approved patch head.
- Annotated v0.1.2 tag object: `7338870aa5dacb4040c5a5a2b29b64621b9f6506`.
- Post-merge CI **SUCCESS**, test and Windows:
  https://github.com/Lord1Egypt/ThothMemory/actions/runs/38456555296
- Release **SUCCESS**, validate/test, validate/Windows, artifacts:
  https://github.com/Lord1Egypt/ThothMemory/actions/runs/38457029107
- Docker publication **SUCCESS**, publish job:
  https://github.com/Lord1Egypt/ThothMemory/actions/runs/38457634410

The workflow validated candidate images by digest before promoting 0.1.2/latest
and publishing the draft. No manual retry, forced tag update or check bypass.

## Public image identities

Repository: https://hub.docker.com/r/lord1egypt/thoth-memory

| Manifest | SHA-256 digest |
|---|---|
| OCI index, both 0.1.2 and latest | `sha256:c6c7d7e0803afe1e8703f1fd6d8cf80f7427cf2d3447a07316ac378afdafd245` |
| linux/amd64 child | `sha256:94572bbe23accd8b8485d59b0ed512bab90dcaef573c8506101b9c33afba8178` |
| linux/arm64 child | `sha256:58dc1ab2605c7051a60064b18397e1594c2010d09b8a9464a3aca09967f37e27` |

Anonymous registry API retrieval verified each manifest's SHA-256 against its
response body. IMAGE_DIGEST.txt matches the index. Both platforms have fetched
SLSA provenance and SBOM attestations; provenance names this repository and the
exact merge commit. Attestation presence is not a claim of independent signing.

Each child was pulled by its immutable digest. Copied image executables are ELF
x86-64 (amd64) and AArch64 (arm64), byte-identical to the release clients below.
Independent post-publication `scripts/smoke-platforms.sh` **passed on both**:
startup, PostgreSQL migration/connectivity, MCP initialization/discovery, writes,
four-agent-session handoff. amd64 ran natively; arm64 ran under **QEMU**.
All fixtures were disposable and cleaned up. No physical Android result is
claimed for these Full images.

## Public native artifacts

All four files downloaded anonymously after publication, matched the initially
downloaded artifacts byte-for-byte, and passed the public SHA256SUMS manifest.
Release also contains SHA256SUMS and IMAGE_DIGEST.txt. Release is public,
non-draft/non-prerelease, titled "ThothMemory v0.1.2"; tag resolves to the merge.

| Asset | SHA-256 | Runtime verification |
|---|---|---|
| thothmemory-linux-amd64 | `d7b9c3fafb8fe9a308dc50172e3cb4a31f95f53493ec7a34155e6cb5432633a9` | native CLI; native published server smoke |
| thothmemory-linux-arm64 | `573e4c1cef9c1ae24d8bf2b970762fe6cc3e2ac7abd1925932db0fe19572d6af` | emulated CLI and published server smoke |
| thothmemory-windows-amd64.exe | `076cd1105a6b1846385d9f0b086e8e7604111d3cb3453893b18ecaa12272132c` | native Windows CI at release source; downloaded PE architecture/checksum verified |
| thothmemory-windows-arm64.exe | `9b32195dee1c2014933c448cf5e2095dccbdd24025814bd62e38247229b66c4c` | PE AArch64/checksum verified; runtime untested |

Local launch of the downloaded Windows amd64 executable was blocked by WSL's
`UtilBindVsockAnyPort: socket failed 1`; this does not establish native execution
of that downloaded asset. No native Windows ARM64 claim. No macOS package here.

## Historical preservation and recovery

v0.1.0/v0.1.1 annotated tag objects remain `4839a2e1d1049ce5daf9df123b23b598488b46fe`
and `854cb758489dfa4fd0dd2ba3b6d129773f8d39a2` respectively. Docker 0.1.1 remains
`sha256:0edd6990289b12099a5c4d44525e9e5bf03fc52934ba09dfb43f293ed4bf9e63`.
The original ARM64 defect is preserved in that historical image, not propagated
to this release. No historical tags or assets were overwritten.

Production containers remain stopped at 2026-10-13T13:37:28Z; original volume,
credentials and cold backup were untouched. There is no production deployment
or migration. This patch adds no schema migration or database dependency.

For later releases, this verified v0.1.2 digest is an architecture-correct
recovery baseline. Any production rollback still requires explicit approval.
For a newly discovered v0.1.2 defect, preserve data and evidence and ship a new
corrective version; do not mutate semantic-version tags or restore the broken
v0.1.1 ARM64 image. See RELEASE_PLAN.md.

Local evidence/assets are retained under the patch worktree's
`bin/release-v0.1.2/`: registry-verification.json, provenance.json, sbom.json,
independent-public-smoke.log, all three workflow logs, downloaded clients and
checksums. Resume verification:

```sh
cd /home/lordegypt/DataBase/ThothMemory-arm64patch
python3 bin/release-v0.1.2/verify-registry.py
(cd bin/release-v0.1.2 && sha256sum -c SHA256SUMS)
gh release view v0.1.2
```

Remaining gates: integrate PR #2 after the now-merged patch and run fresh CI;
owner approval before its merge/release. PR #8 remains unmerged and needs final
integration, GitGuardian incident 38171534 resolution, and separate edition
release approvals. Final Mini device retest requires phone connectivity and
approved distribution. Production upgrade remains separately gated.
