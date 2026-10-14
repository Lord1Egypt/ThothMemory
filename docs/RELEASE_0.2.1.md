# ThothMemory v0.2.1 — Mini backup correction (candidate, not published)

This corrective release follows v0.2.0, whose independent verification found that **Mini
0.1.0 could not complete `backup`** (`TLS/SSL error: certificate is not yet valid`), so its
backup and restore were never verified. v0.2.0 and Mini 0.1.0 are left as published.

- **Mini 0.1.1:** backup and restore no longer depend on a TLS certificate comparison over
  the container's private Unix socket. The TCP fallback keeps TLS. Complete backup,
  verified restore into a new database, activation and MCP reads on the restored copy are
  now required gates on amd64 and arm64 (arm64 emulated in CI) before publication.
- **Full 0.2.1 / UltraLite 0.1.0:** no behavior change; Full's version label follows the
  unified release.

If you installed Mini 0.1.0: take an independent copy of the `thothmemory-mini-data` volume,
then install 0.1.1 from this release (`thothmini.sh` and `MINI_IMAGE_DIGEST.txt`). Test
`thothmini.sh backup` and a restore into a new database before relying on it.

Android: the physical-device retest of the corrected digest has not been run; earlier
Samsung results apply to Mini 0.1.0 only. See docs/PUBLIC_RELEASE_VERIFICATION.md.

Checksums: download SHA256SUMS, IMAGE_DIGEST.txt, MINI_IMAGE_DIGEST.txt and PLATFORMS.md
and verify as in the v0.2.0 notes.
