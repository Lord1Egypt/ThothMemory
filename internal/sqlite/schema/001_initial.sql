-- ThothMemory UltraLite schema 1 (SQLite, STRICT tables).
-- Identifiers compare with SQLite's default BINARY collation, matching
-- PostgreSQL. Timestamps are fixed-width UTC text (2006-01-02T15:04:05.000000Z),
-- so text order is chronological. search_text is NFC-lowercased by the
-- application, as for MariaDB. Applied inside one transaction.
CREATE TABLE principals(id TEXT NOT NULL PRIMARY KEY CHECK(length(id) BETWEEN 1 AND 200), created_at TEXT NOT NULL) STRICT;
CREATE TABLE projects(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 200), repository TEXT NOT NULL UNIQUE CHECK(length(repository) BETWEEN 1 AND 1000), created_at TEXT NOT NULL, event_count INTEGER NOT NULL DEFAULT 0 CHECK(event_count>=0), event_budget INTEGER NOT NULL DEFAULT 100000 CHECK(event_budget BETWEEN 100 AND 10000000)) STRICT;
CREATE TABLE agents(id TEXT NOT NULL PRIMARY KEY CHECK(length(id) BETWEEN 1 AND 100)) STRICT;
CREATE TABLE sessions(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL REFERENCES projects(id), agent_id TEXT NOT NULL REFERENCES agents(id), principal_id TEXT NOT NULL REFERENCES principals(id), branch TEXT NOT NULL CHECK(length(branch)<=500), git_revision TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT, revision INTEGER NOT NULL DEFAULT 1, UNIQUE(project_id,id)) STRICT;
CREATE INDEX sessions_project_time ON sessions(project_id,started_at);
CREATE TABLE tasks(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL REFERENCES projects(id), title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 500), state TEXT NOT NULL DEFAULT 'planned' CHECK(state IN ('planned','active','blocked','implemented','committed','tested','verified','released')), revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>0), git_revision TEXT NOT NULL DEFAULT '', claimed_by TEXT REFERENCES principals(id), claim_expires_at TEXT, updated_at TEXT NOT NULL, search_text TEXT NOT NULL, UNIQUE(project_id,id)) STRICT;
CREATE INDEX tasks_project_state ON tasks(project_id,state,updated_at);
CREATE INDEX tasks_project_time ON tasks(project_id,updated_at);
CREATE TABLE task_dependencies(project_id TEXT NOT NULL, task_id TEXT NOT NULL, depends_on TEXT NOT NULL, PRIMARY KEY(project_id,task_id,depends_on), FOREIGN KEY(project_id,task_id) REFERENCES tasks(project_id,id), FOREIGN KEY(project_id,depends_on) REFERENCES tasks(project_id,id), CHECK(task_id<>depends_on)) STRICT;
CREATE INDEX task_dependencies_reverse ON task_dependencies(project_id,depends_on);
CREATE TABLE evidence(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL REFERENCES projects(id), task_id TEXT, source_kind TEXT NOT NULL CHECK(source_kind IN ('git_commit','pull_request','build','test','ci','log','human','inference','release')), locator TEXT NOT NULL CHECK(length(locator) BETWEEN 1 AND 2000), git_revision TEXT NOT NULL, artifact_digest TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL CHECK(outcome IN ('pass','fail','unknown')), summary TEXT NOT NULL CHECK(length(summary) BETWEEN 1 AND 8000), reported_by TEXT NOT NULL REFERENCES principals(id), created_at TEXT NOT NULL, search_text TEXT NOT NULL, UNIQUE(project_id,id), FOREIGN KEY(project_id,task_id) REFERENCES tasks(project_id,id)) STRICT;
CREATE INDEX evidence_task_revision ON evidence(project_id,task_id,git_revision);
CREATE INDEX evidence_project_time ON evidence(project_id,created_at);
CREATE TABLE attestations(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL, evidence_id TEXT NOT NULL UNIQUE, principal_id TEXT NOT NULL REFERENCES principals(id), statement TEXT NOT NULL CHECK(length(statement) BETWEEN 1 AND 8000), created_at TEXT NOT NULL, FOREIGN KEY(project_id,evidence_id) REFERENCES evidence(project_id,id)) STRICT;
CREATE INDEX attestations_project ON attestations(project_id);
CREATE TABLE events(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL REFERENCES projects(id), task_id TEXT, session_id TEXT, kind TEXT NOT NULL CHECK(length(kind) BETWEEN 1 AND 100), body TEXT NOT NULL CHECK(length(body) BETWEEN 1 AND 8000), principal_id TEXT NOT NULL REFERENCES principals(id), agent_id TEXT NOT NULL DEFAULT '', git_revision TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, details TEXT NOT NULL CHECK(json_valid(details)), search_text TEXT NOT NULL, FOREIGN KEY(project_id,task_id) REFERENCES tasks(project_id,id), FOREIGN KEY(project_id,session_id) REFERENCES sessions(project_id,id)) STRICT;
CREATE INDEX events_project_time ON events(project_id,created_at,id);
CREATE INDEX events_task_time ON events(project_id,task_id,created_at);
CREATE INDEX events_session ON events(project_id,session_id);
CREATE TABLE records(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL REFERENCES projects(id), task_id TEXT, session_id TEXT, kind TEXT NOT NULL CHECK(kind IN ('decision','issue','observation','handoff','idea','note')), title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 500), body TEXT NOT NULL CHECK(length(body) BETWEEN 1 AND 8000), git_revision TEXT NOT NULL DEFAULT '', principal_id TEXT NOT NULL REFERENCES principals(id), agent_id TEXT NOT NULL DEFAULT '', evidence_id TEXT, supersedes TEXT UNIQUE, created_at TEXT NOT NULL, search_text TEXT NOT NULL, UNIQUE(project_id,id), FOREIGN KEY(project_id,task_id) REFERENCES tasks(project_id,id), FOREIGN KEY(project_id,session_id) REFERENCES sessions(project_id,id), FOREIGN KEY(project_id,evidence_id) REFERENCES evidence(project_id,id), FOREIGN KEY(project_id,supersedes) REFERENCES records(project_id,id)) STRICT;
CREATE INDEX records_project_time ON records(project_id,created_at,id);
CREATE INDEX records_project_kind ON records(project_id,kind,created_at);
CREATE INDEX records_session ON records(project_id,session_id);
CREATE TABLE operations(project_id TEXT NOT NULL REFERENCES projects(id), operation_id TEXT NOT NULL CHECK(length(operation_id) BETWEEN 8 AND 100), principal_id TEXT NOT NULL REFERENCES principals(id), payload_hash TEXT NOT NULL CHECK(length(payload_hash)=64), response TEXT NOT NULL CHECK(json_valid(response)), created_at TEXT NOT NULL, PRIMARY KEY(project_id,principal_id,operation_id)) STRICT;
CREATE TABLE imports(id TEXT NOT NULL PRIMARY KEY CHECK(length(id)=36), project_id TEXT NOT NULL REFERENCES projects(id), export_id TEXT NOT NULL, source_edition TEXT NOT NULL CHECK(source_edition IN ('full','mini','ultralite')), source_version TEXT NOT NULL CHECK(length(source_version) BETWEEN 1 AND 50), format_version INTEGER NOT NULL, checksum TEXT NOT NULL CHECK(length(checksum)=64), counts TEXT NOT NULL CHECK(json_valid(counts)), principal_id TEXT NOT NULL REFERENCES principals(id), imported_at TEXT NOT NULL, UNIQUE(project_id,checksum)) STRICT;
CREATE INDEX imports_project_time ON imports(project_id,imported_at);
CREATE TRIGGER immutable_events_update BEFORE UPDATE ON events BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_events_delete BEFORE DELETE ON events BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_records_update BEFORE UPDATE ON records BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_records_delete BEFORE DELETE ON records BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_evidence_update BEFORE UPDATE ON evidence BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_evidence_delete BEFORE DELETE ON evidence BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_attestations_update BEFORE UPDATE ON attestations BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_attestations_delete BEFORE DELETE ON attestations BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_operations_update BEFORE UPDATE ON operations BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_operations_delete BEFORE DELETE ON operations BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_imports_update BEFORE UPDATE ON imports BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
CREATE TRIGGER immutable_imports_delete BEFORE DELETE ON imports BEGIN SELECT RAISE(ABORT,'history is append-only'); END;
