ALTER TABLE events ADD COLUMN details jsonb NOT NULL DEFAULT '{}'::jsonb;
