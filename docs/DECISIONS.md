# Architecture decisions

1. Use official Go MCP SDK and pgx; avoid custom protocol implementations.
2. PostgreSQL FTS first. Vectors and embeddings are deferred.
3. One service, embedded plain HTML/CSS/JS. No frontend runtime or polling loop.
4. Explicit repository identity, scoped local credentials. No automatic folder merging.
5. Separate reported evidence from owner attestation; preserve revision specificity.
6. Immutable events, optimistic task revisions, transaction-level idempotency.
7. Stdio bridge for credential-free client config; Streamable HTTP shared gateway.
8. Supported instructions with incremental checkpoints; no guarantee on termination.
9. Least-privilege runtime DB login separated from migration/schema owner.
10. New-database restore with recovery backup; never implicit destructive restore.
