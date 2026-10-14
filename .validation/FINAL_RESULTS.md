# Final results at c9b852adbcf9820eb8d6a5ecff0c1edefb84846a

## Latest: v0.2.0 published, independent Mini recovery FAILED

Released source be30dfe67c9094f197d8810add7b77fb3abcb5d8. All CI/security and
publication workflows passed; 16 public assets and checksums independently
verified. Full public amd64/emulated-arm64 smoke and native Linux/Windows
UltraLite checks passed. Public Mini passed both architectures' 11 MCP gates
and seeded restart/crash/volume recovery, but backup failed with MariaDB TLS
certificate-not-yet-valid error. Restore could not complete. Stop corrective
publication pending owner direction; do not mutate existing tags or bypass TLS.
Exact digests/evidence/resume state: docs/PUBLIC_RELEASE_VERIFICATION.md.
Production untouched. Older in-progress and blocked entries are historical.

## v0.2.0 release in progress — 2026-10-14

Owner reviewed incident 38171534. GitHub check-run rerequest for 115527343001
accepted; fresh GitGuardian 115653901254 at f94a63b passed (29 commits, no
secrets). Final head 5ca6a8391d90e0887301a0096f9878d6dbf54b06 passed all six
CI jobs in 38503392283 plus GitGuardian. Mini lifecycle QA now seeds real MCP
records and compares portable payload hashes; local and CI gates passed 14/14.

PR #8 merged as be30dfe67c9094f197d8810add7b77fb3abcb5d8, tree identical to
approved head. Main CI 38503866569 SUCCESS before tag creation. Annotated
v0.2.0 object 3f1cdb82918c5bd62ba10c6dad6a7cc55ea8691d pushed; Release run
38504405851 queued. Follow it and its automatic Docker publication chain; do
not dispatch duplicate publication. Public verification still outstanding.
No production changes. Evidence and verification helpers: bin/release-v0.2.0/.

## Unified launch follow-up at f94a63b0a3dc3e23d6392b66d00a3c9dc631a4f1

Final CI 38464367577 completed SUCCESS, all six jobs, including native Windows,
Linux amd64/ARM64 beginner flows with actual Codex CLI registration and real
MCP stdio persistence/backup/restore. GitGuardian is separately FAILURE on
incident 38171534. Owner-only false-positive review requested but unanswered.
PR #2 merged at c6a78e5412bc7ddc569382bcd7299a5b80511216 after green integrated
CI/GitGuardian; main CI 38462178511 passed. PR #8 targets main and remains Draft.
No v0.2.0 tag, primary release, new public image or public UltraLite asset exists.

Unified packaging, guarded draft resumption, Full/Mini image staging/scanning,
complete asset verification and beginner English/Arabic docs implemented.
OpenSSL CVE-2026-84782 found by new Mini scan, fixed with pinned libssl3t64 3.16;
patched image scan passes. No scanner suppression or bypass.

Local nine-binary package at bin/launch-dist/ rebuilt at f94a63b; all checksums
verified. Real public v0.1.2 staging refusal tested, no assets changed.
Local Full source-package install/doctor/backup/restore and Mini local-registry
install/clients/doctor/backup/restart/uninstall passed. All disposable test
containers/volumes removed. Historical clone original fields/counts preserved
through 007; 12-project / 84-route dashboard passed again. Production containers
retain original stop timestamps; original volume/credentials/cold backup untouched.
Final Mini phone retest blocked: ADB empty. Existing physical evidence retained.

Resume from docs/LAUNCH_STATUS.md and docs/FINAL_HANDOFF.md, then PR #8's exact
checks and updated description. No new release authorization needed once all
the owner's conditional gates pass. Finalize release-facing pending notices
before merge/tag and verify every public artifact after the workflow chain.

## Release completion (supersedes the pending-approval notes below)

Explicit owner approvals received and executed. PR #9 merged at
c7d2f8dd69a8284058f8e2d69619140ddc2d32e1; post-merge CI 38456555296 passed.
Annotated v0.1.2 pushed only afterward; Release 38457029107 and Docker publication
38457634410 both succeeded. Public GitHub release and four anonymous binary
downloads/checksums verified. Index/latest digest:
sha256:c6c7d7e0803afe1e8703f1fd6d8cf80f7427cf2d3447a07316ac378afdafd245
Both child manifests were pulled by digest, ELF checked, and image executables
match published client binaries. Independent real server/MCP smoke passed on
amd64 native and arm64 QEMU. SBOM/provenance fetched for both platforms; correct
merge commit recorded. Historical v0.1.1 image and Git tags unchanged.
Production untouched. Exact durable evidence: docs/RELEASE_0.1.2_VERIFIED.md.
Evidence files: ../ThothMemory-arm64patch/bin/release-v0.1.2/ from worktree parent.
No PR #2/#8 merge or other product publication authorized/performed.

Release-execution follow-up: tracked checkpoint updated in docs-only commit
57e70e8 on nextgen/full-and-mini. PR #9 remains at ef4867f with CLEAN/MERGEABLE
status and all checks green; v0.1.2 remains absent. Await separate owner approvals
for (1) merge PR #9 and (2) tag/publication/latest promotion after green merge CI.
No release, merge or production change was performed. No unchanged artifact or
successful suite was repeated. Corrected recovery procedure avoids the broken
v0.1.1 ARM64 image. See docs/FINAL_HANDOFF.md for resume commands and release matrix.

GitHub CI https://github.com/Lord1Egypt/ThothMemory/actions/runs/38452588196
completed SUCCESS, six of six jobs. Separate GitGuardian remains FAILURE on
synthetic fixture incident 38171534 in earlier commit 1aeacc34. PR #8 stays Draft
and stacked on audit/preproduction-0.1.2. No production changes or publication.

Mini glibc ARM64 rebuilt and tested locally under QEMU: ELF AArch64,
MariaDB 11.4.12 AArch64, 11/11 MCP acceptance gates, zero skipped.
Image local index: sha256:4b59c6e561536e58dc4e3c848bf2a889c05f415ae310f2f8b9e3a48312afe0da
Archive: bin/rc/thothmemory-mini-glibc-arm64-candidate.tar
SHA256: 1d99835c869a4430d499148e3f2425456197a4edcb730db1d0a1c3284a7470c5
Executable SHA256: b938cc5b058850ce38490b0756bb0cf137c6b62ce1b57897f39a3494b7eb71ee
Not published; Android retest needs device connectivity and approved distribution.

All five UltraLite artifact checksums match the committed manifest. Patch archive
and immutable local image IDs are documented in docs/FINAL_HANDOFF.md.

Local logs: final-check.log, final-integration.log, final-acceptance.log,
final-skipped.log, final-patch-smoke.log, final-mini-build.log,
final-mini-arm64-acceptance.log. Historical dashboard: 12 projects, 84 routes,
zero JS errors/overflow; private screenshots in dashboard/. Do not publish them.

Disposable dashboard DB/container/volume and Mini container removed. Services on
27401 and 27402 stopped; production original containers remain stopped at their
original timestamps. Cold backup checksum reverified and original preserved.

Persistent upstream handoff: PR #8 description records these final CI and artifact
results without causing a redundant docs-only CI rerun. Tracked engineering
checkpoint is docs/FINAL_HANDOFF.md.
