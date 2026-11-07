CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Tables: everything below is per person ----------

-- A base holds tables that belong together (a small business, a house, a
-- hobby); links and lookups work between the tables of one base.
CREATE TABLE bases (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  icon        TEXT NOT NULL DEFAULT '',   -- an emoji
  color       TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  position    REAL NOT NULL DEFAULT 0,
  template    TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX bases_user ON bases(user_id, position);

-- A table ("sheet" in SQL, so it never reads like a SQL table). settings
-- holds the ties to other apps: a date field kept in Pocket Calendar, a
-- form whose answers become rows, the field ticked when a task is done.
CREATE TABLE sheets (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  base_id     INTEGER NOT NULL REFERENCES bases(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  icon        TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  position    REAL NOT NULL DEFAULT 0,
  next_number INTEGER NOT NULL DEFAULT 1,
  settings    TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX sheets_user ON sheets(user_id, base_id, position);

CREATE TABLE fields (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  sheet_id    INTEGER NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  type        TEXT NOT NULL DEFAULT 'text',
  options     TEXT NOT NULL DEFAULT '{}',
  description TEXT NOT NULL DEFAULT '',
  position    REAL NOT NULL DEFAULT 0,
  is_primary  INTEGER NOT NULL DEFAULT 0,
  required    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX fields_user ON fields(user_id, sheet_id, position);

-- Ways to look at a table: grid, board, calendar, gallery; config holds
-- the filters, sorts, hidden fields, grouping, widths and footers.
CREATE TABLE views (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  sheet_id   INTEGER NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  type       TEXT NOT NULL DEFAULT 'grid',
  config     TEXT NOT NULL DEFAULT '{}',
  position   REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX views_user ON views(user_id, sheet_id, position);

-- Rows. data holds the typed-in values by field id; links live in links.
-- source says which app added it (a form, Pocket Flows, a link); a
-- source_ref is never added twice to the same table. Deleted rows stay
-- 30 days (deleted_at) so they can be brought back.
CREATE TABLE records (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  sheet_id   INTEGER NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,
  number     INTEGER NOT NULL DEFAULT 0,
  data       TEXT NOT NULL DEFAULT '{}',
  position   REAL NOT NULL DEFAULT 0,
  source     TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT '',
  deleted_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX records_user ON records(user_id, sheet_id, deleted_at, position);
CREATE UNIQUE INDEX records_source ON records(sheet_id, source_ref) WHERE source_ref != '';

-- Linked records: stored once, under the field on the side where the link
-- was made; the field on the other table reads them backwards.
CREATE TABLE links (
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  field_id INTEGER NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  from_id  INTEGER NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  to_id    INTEGER NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  position INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (field_id, from_id, to_id)
);
CREATE INDEX links_to ON links(field_id, to_id);
CREATE INDEX links_user ON links(user_id);

-- What happened to a row: comments and changes.
CREATE TABLE history (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  record_id  INTEGER NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL DEFAULT 'change',   -- comment | change | created | made | restored
  text       TEXT NOT NULL DEFAULT '',
  data       TEXT NOT NULL DEFAULT '{}',
  app        TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX history_record ON history(user_id, record_id, created_at);

-- Attached files (kept under DATA_DIR/files/<id>).
CREATE TABLE files (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  record_id  INTEGER REFERENCES records(id) ON DELETE SET NULL,
  name       TEXT NOT NULL DEFAULT '',
  mime       TEXT NOT NULL DEFAULT '',
  size       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX files_user ON files(user_id, record_id);

-- Things a row has in other Pocket apps: a task, a calendar event, a note.
CREATE TABLE made (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  record_id  INTEGER NOT NULL REFERENCES records(id) ON DELETE CASCADE,
  target     TEXT NOT NULL,                 -- task | event | note | sync (the table's calendar)
  app        TEXT NOT NULL,
  ref        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL DEFAULT '',
  stamp      TEXT NOT NULL DEFAULT '',      -- for sync: what was last sent
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  checked_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX made_record ON made(user_id, record_id);
