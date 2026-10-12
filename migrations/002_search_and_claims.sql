ALTER TABLE tasks ADD COLUMN claim_expires_at timestamptz;
ALTER TABLE events ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english',kind || ' ' || body)) STORED;
ALTER TABLE evidence ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english',source_kind || ' ' || summary)) STORED;
ALTER TABLE tasks ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english',title)) STORED;
CREATE INDEX events_search ON events USING gin(search_vector);
CREATE INDEX evidence_search ON evidence USING gin(search_vector);
CREATE INDEX tasks_search ON tasks USING gin(search_vector);
CREATE VIEW memory_search AS
 SELECT id,project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes,created_at,search_vector FROM records
 UNION ALL SELECT id,project_id,task_id,session_id,'event',kind,body,git_revision,principal_id,agent_id,NULL::uuid,NULL::uuid,created_at,search_vector FROM events
 UNION ALL SELECT id,project_id,task_id,NULL::uuid,'evidence',locator,summary,git_revision,reported_by,'',id,NULL::uuid,created_at,search_vector FROM evidence
 UNION ALL SELECT id,project_id,id,NULL::uuid,'task',title,state,git_revision,COALESCE(claimed_by,''),'',NULL::uuid,NULL::uuid,updated_at,search_vector FROM tasks;
