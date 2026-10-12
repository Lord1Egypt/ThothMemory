-- Bound automatic growth while retaining immutable history. Owner can raise the
-- per-project budget explicitly after reviewing disk usage and backups.
ALTER TABLE projects ADD COLUMN event_count bigint NOT NULL DEFAULT 0 CHECK(event_count>=0);
ALTER TABLE projects ADD COLUMN event_budget bigint NOT NULL DEFAULT 100000 CHECK(event_budget BETWEEN 100 AND 10000000);
UPDATE projects p SET event_count=(SELECT count(*) FROM events e WHERE e.project_id=p.id);
