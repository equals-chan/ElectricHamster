-- Encrypted vault. `items.ciphertext` holds the AES-256-GCM-encrypted password
-- and is decrypted with the data key (DEK); the DEK itself is wrapped with the
-- master-password-derived key (KEK) in `meta`.

CREATE TABLE IF NOT EXISTS meta (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  format_version  INTEGER NOT NULL,
  kdf             TEXT NOT NULL,
  kdf_time        INTEGER NOT NULL,
  kdf_memory      INTEGER NOT NULL,
  kdf_parallelism INTEGER NOT NULL,
  key_len         INTEGER NOT NULL,
  kdf_salt        BLOB NOT NULL,
  kek_nonce       BLOB NOT NULL,
  dek_ciphertext  BLOB NOT NULL,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS items (
  id              TEXT PRIMARY KEY,
  label           TEXT NOT NULL,
  nonce           BLOB NOT NULL,
  ciphertext      BLOB NOT NULL,
  note_nonce      BLOB,
  note_ciphertext BLOB,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_items_label ON items(label);
