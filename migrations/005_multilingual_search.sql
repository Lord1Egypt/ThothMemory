-- Additive: retain released English vectors and immutable engineering history.
-- NFC preserves equivalent Unicode spellings; simple search does not stem Arabic.
ALTER TABLE records ADD COLUMN search_text text GENERATED ALWAYS AS (normalize(title || ' ' || body || ' ' || git_revision)) STORED;
ALTER TABLE records ADD COLUMN search_simple tsvector GENERATED ALWAYS AS (to_tsvector('simple',normalize(title || ' ' || body || ' ' || git_revision))) STORED;
ALTER TABLE events ADD COLUMN search_text text GENERATED ALWAYS AS (normalize(kind || ' ' || body || ' ' || git_revision)) STORED;
ALTER TABLE events ADD COLUMN search_simple tsvector GENERATED ALWAYS AS (to_tsvector('simple',normalize(kind || ' ' || body || ' ' || git_revision))) STORED;
ALTER TABLE evidence ADD COLUMN search_text text GENERATED ALWAYS AS (normalize(locator || ' ' || summary || ' ' || git_revision || ' ' || artifact_digest)) STORED;
ALTER TABLE evidence ADD COLUMN search_simple tsvector GENERATED ALWAYS AS (to_tsvector('simple',normalize(locator || ' ' || summary || ' ' || git_revision || ' ' || artifact_digest))) STORED;
ALTER TABLE tasks ADD COLUMN search_text text GENERATED ALWAYS AS (normalize(title || ' ' || git_revision)) STORED;
ALTER TABLE tasks ADD COLUMN search_simple tsvector GENERATED ALWAYS AS (to_tsvector('simple',normalize(title || ' ' || git_revision))) STORED;
CREATE INDEX records_search_simple ON records USING gin(search_simple);
CREATE INDEX events_search_simple ON events USING gin(search_simple);
CREATE INDEX evidence_search_simple ON evidence USING gin(search_simple);
CREATE INDEX tasks_search_simple ON tasks USING gin(search_simple);
CREATE OR REPLACE VIEW memory_search AS
 SELECT id,project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes,created_at,search_vector,search_text,search_simple FROM records
 UNION ALL SELECT id,project_id,task_id,session_id,'event',kind,body,git_revision,principal_id,agent_id,NULL::uuid,NULL::uuid,created_at,search_vector,search_text,search_simple FROM events
 UNION ALL SELECT id,project_id,task_id,NULL::uuid,'evidence',locator,summary,git_revision,reported_by,'',id,NULL::uuid,created_at,search_vector,search_text,search_simple FROM evidence
 UNION ALL SELECT id,project_id,id,NULL::uuid,'task',title,state,git_revision,COALESCE(claimed_by,''),'',NULL::uuid,NULL::uuid,updated_at,search_vector,search_text,search_simple FROM tasks;
