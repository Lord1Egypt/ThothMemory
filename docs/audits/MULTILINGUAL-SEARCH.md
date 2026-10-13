# Multilingual search audit

Fixture: synthetic English, Egyptian Arabic, formal Arabic, mixed notes, technical
symbols/paths, SQL errors, full Git SHA, Unicode composed text and similar repeated
observations. Requests use real MCP against PostgreSQL; exact ID lookup tested too.

| Query class | v0.1.1 | Candidate |
|---|---|---|
| English stem `migrate` | Found | Found |
| Egyptian Arabic `اتحلت` | Found | Found |
| Formal Arabic `تعارض` | Found | Found |
| Mixed AND `PostgreSQL اختبار` | Found | Found |
| Symbol substring `Checkpoint` in HandleCheckpointHTTP | Missed target | Found |
| Arabic body substring `إعداد` in إعدادات | Missed | Found |
| File path `internal/store/operations.go` | Found | Found |
| PostgreSQL SQLSTATE `23505` | Found | Found |
| Commit prefix `9a3f8c1275e4` | Missed | Found |
| Decomposed `café` matching composed café | Missed | Found |
| Exact record UUID | Found | Found |

Evidence: [before](evidence/search-before.json), [after](evidence/search-after.json),
regression `TestAuditSearchAndCheckpointRecovery` in [executed suite](evidence/integration-tests.txt).
A result is counted only when the expected record ID is returned, not simply when
any similar event matches. Rankings use English/simple full-text rank and recency;
no semantic relevance claim or Arabic morphological stemming is made.

## Decision

Add migration `005_multilingual_search.sql`. Keep immutable released migrations and
original English vectors. Add NFC-normalized search text and simple tsvector with
GIN indexes to records/events/evidence/tasks; extend the existing search view. Search
combines English stemming, simple tokens and literal case-insensitive substring
matching of normalized text, including full revision and evidence locator/digest.
Existing records/history/provenance are preserved. Exact IDs and authorization
filters remain parameterized. Literal `%` and `_` are not SQL LIKE wildcards.

No trigram extension, language service, embeddings or second database is required.
NFC handles canonically equivalent Unicode, not Arabic dialect synonyms, diacritic
removal, hamza/alef folding or translations. Mixed full-text terms use PostgreSQL's
websearch syntax; substring fallback is the literal whole query, not arbitrary regex.
For precise identities use exact IDs/revisions and inspect returned provenance.

An initial repeated substring boost in ranking caused a measured regression and was
removed. [Performance comparison](PERFORMANCE-REGRESSION.md) reports the final query.
Substring predicates can scan project candidates and are not covered by the GIN
indexes. Large datasets beyond the tested size remain a scaling limitation, rather
than a reason to add mandatory infrastructure prematurely.
