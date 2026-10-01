-- Per-task archive naming: optional prefix/suffix and an explicit starting
-- sequence number. Existing tasks get sensible defaults.

ALTER TABLE tasks ADD COLUMN name_prefix TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN name_suffix TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN seq_start INTEGER NOT NULL DEFAULT 10000;
