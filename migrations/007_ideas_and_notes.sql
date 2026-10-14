-- Ideas (proposals, never decisions) and short notes become first-class records.
ALTER TABLE records DROP CONSTRAINT records_kind_check;
ALTER TABLE records ADD CONSTRAINT records_kind_check CHECK(kind IN ('decision','issue','observation','handoff','idea','note'));
