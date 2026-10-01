-- Application schema. Passwords are never stored here; they live in the
-- encrypted vault (vault_items.password_id references a vault item id).

CREATE TABLE IF NOT EXISTS tasks (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL,
  source_dir      TEXT NOT NULL,
  dest_dir        TEXT NOT NULL,
  include_globs   TEXT,
  exclude_globs   TEXT,
  format          TEXT NOT NULL DEFAULT '7z',
  codec           TEXT NOT NULL DEFAULT 'zstd',
  level           INTEGER NOT NULL DEFAULT 15,
  encrypt         INTEGER NOT NULL DEFAULT 1,
  header_encrypt  INTEGER NOT NULL DEFAULT 1,
  split_size      INTEGER NOT NULL DEFAULT 0,
  password_policy TEXT NOT NULL DEFAULT '{}',
  dedup_rule      TEXT NOT NULL DEFAULT 'content_sig',
  enabled         INTEGER NOT NULL DEFAULT 1,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
  id          TEXT PRIMARY KEY,
  task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  status      TEXT NOT NULL,
  total       INTEGER NOT NULL DEFAULT 0,
  done        INTEGER NOT NULL DEFAULT 0,
  bytes_in    INTEGER NOT NULL DEFAULT 0,
  bytes_out   INTEGER NOT NULL DEFAULT 0,
  started_at  TEXT,
  finished_at TEXT,
  error       TEXT
);
CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id);

CREATE TABLE IF NOT EXISTS archives (
  id           TEXT PRIMARY KEY,
  run_id       TEXT REFERENCES runs(id) ON DELETE SET NULL,
  task_id      TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  seq          INTEGER NOT NULL,
  folder_name  TEXT NOT NULL,
  source_path  TEXT NOT NULL,
  archive_path TEXT NOT NULL,
  archive_size INTEGER NOT NULL DEFAULT 0,
  file_count   INTEGER NOT NULL DEFAULT 0,
  content_sig  TEXT NOT NULL,
  password_id  TEXT,
  status       TEXT NOT NULL DEFAULT 'ok',
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_archives_sig ON archives(content_sig);
CREATE UNIQUE INDEX IF NOT EXISTS idx_archives_path ON archives(archive_path);
CREATE INDEX IF NOT EXISTS idx_archives_task ON archives(task_id);

CREATE TABLE IF NOT EXISTS counters (
  name  TEXT PRIMARY KEY,
  value INTEGER NOT NULL
);
