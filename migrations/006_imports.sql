-- Append-only provenance for owner-run backend-neutral imports.
CREATE TABLE imports(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), project_id uuid NOT NULL REFERENCES projects, export_id uuid NOT NULL, source_edition text NOT NULL CHECK(source_edition IN ('full','mini','ultralite')), source_version text NOT NULL CHECK(length(source_version) BETWEEN 1 AND 50), format_version integer NOT NULL, checksum text NOT NULL CHECK(length(checksum)=64), counts jsonb NOT NULL, principal_id text NOT NULL REFERENCES principals, imported_at timestamptz NOT NULL DEFAULT now(), UNIQUE(project_id,checksum));
CREATE INDEX imports_project_time ON imports(project_id,imported_at DESC);
CREATE TRIGGER immutable_imports BEFORE UPDATE OR DELETE ON imports FOR EACH ROW EXECUTE FUNCTION reject_history_change();
