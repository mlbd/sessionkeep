-- 0001 foundation: projects, sessions, evidence, events, instructions, capture gaps, jobs.
-- Rules (docs/01-DATA_MODEL.md):
--   * every project-owned row carries project_id and references its parent through a composite
--     key, so a row can never point at a parent in another project;
--   * a missing project never means "global": global scope is explicit;
--   * permitted original text is immutable once stored (it can be deleted, not rewritten);
--   * ids are opaque UUID strings, times are UTC RFC 3339 strings.

CREATE TABLE projects (
  id             TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  capture_policy TEXT NOT NULL DEFAULT 'off' CHECK (capture_policy IN ('off','metadata','conversation','extended')),
  created_at     TEXT NOT NULL,
  archived_at    TEXT
) STRICT;

CREATE TABLE project_roots (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  device_id       TEXT NOT NULL,
  normalized_path TEXT NOT NULL,
  repo_identity   TEXT,
  worktree_id     TEXT,
  last_seen       TEXT NOT NULL,
  UNIQUE (device_id, normalized_path)
) STRICT;
CREATE INDEX project_roots_project ON project_roots(project_id);

CREATE TABLE sessions (
  id                  TEXT PRIMARY KEY,
  project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  agent               TEXT NOT NULL,
  external_session_id TEXT NOT NULL,
  device_id           TEXT NOT NULL,
  worktree_id         TEXT,
  branch              TEXT,
  started_at          TEXT NOT NULL,
  ended_at            TEXT,
  state               TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open','ended','interrupted')),
  adapter_version     INTEGER NOT NULL,
  UNIQUE (project_id, agent, device_id, external_session_id),   -- one host session can move between projects
  UNIQUE (project_id, id)
) STRICT;
CREATE INDEX sessions_project_time ON sessions(project_id, started_at);

CREATE TABLE blobs (
  id                TEXT PRIMARY KEY,
  project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  digest            TEXT NOT NULL,
  size              INTEGER NOT NULL CHECK (size >= 0),
  media_type        TEXT NOT NULL,
  storage_ref       TEXT NOT NULL,
  redaction_version INTEGER NOT NULL,
  created_at        TEXT NOT NULL,
  UNIQUE (project_id, digest),
  UNIQUE (project_id, id)
) STRICT;

-- Permitted original text. `text` holds the content for searchable evidence; large or
-- non-text content lives in a blob.
CREATE TABLE evidence (
  seq             INTEGER PRIMARY KEY AUTOINCREMENT,   -- stable integer key for the FTS index, never reused
  id              TEXT NOT NULL UNIQUE,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  origin          TEXT NOT NULL,            -- e.g. hook:UserPromptSubmit, hook:Stop, transcript, user:remember
  source_locator  TEXT NOT NULL,
  source_revision TEXT,
  range_start     INTEGER,
  range_end       INTEGER,
  observed_at     TEXT NOT NULL,
  text            TEXT,
  blob_id         TEXT,
  completeness    TEXT NOT NULL CHECK (completeness IN ('complete','truncated','partial')),
  trust_level     TEXT NOT NULL CHECK (trust_level IN ('user','assistant','tool','external')),
  redaction_count INTEGER NOT NULL DEFAULT 0 CHECK (redaction_count >= 0),
  CHECK (text IS NOT NULL OR blob_id IS NOT NULL),
  UNIQUE (project_id, id),
  FOREIGN KEY (project_id, blob_id) REFERENCES blobs(project_id, id)
) STRICT;
CREATE INDEX evidence_project_time ON evidence(project_id, observed_at);

CREATE TRIGGER evidence_immutable BEFORE UPDATE OF seq, id, project_id, origin, source_locator, source_revision,
  range_start, range_end, observed_at, text, blob_id, completeness, trust_level, redaction_count ON evidence BEGIN
  SELECT RAISE(ABORT, 'evidence is immutable');
END;

CREATE TABLE events (
  id               TEXT PRIMARY KEY,
  project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  session_id       TEXT NOT NULL,
  event_key        TEXT NOT NULL UNIQUE,    -- idempotency key, namespaced by source
  kind             TEXT NOT NULL,
  observed_at      TEXT NOT NULL,
  received_at      TEXT NOT NULL,
  payload_version  INTEGER NOT NULL,
  safe_payload     TEXT NOT NULL CHECK (json_valid(safe_payload)),
  evidence_id      TEXT,
  redaction_status TEXT NOT NULL CHECK (redaction_status IN ('none','redacted','quarantined')),
  UNIQUE (project_id, id),
  FOREIGN KEY (project_id, session_id)  REFERENCES sessions(project_id, id) ON DELETE CASCADE,
  FOREIGN KEY (project_id, evidence_id) REFERENCES evidence(project_id, id)
) STRICT;
CREATE INDEX events_session_time ON events(project_id, session_id, observed_at);
CREATE INDEX events_evidence ON events(project_id, evidence_id);

CREATE TRIGGER events_immutable BEFORE UPDATE ON events BEGIN
  SELECT RAISE(ABORT, 'events are immutable');
END;

CREATE TABLE capture_gaps (
  id          TEXT PRIMARY KEY,
  project_id  TEXT REFERENCES projects(id) ON DELETE CASCADE,   -- NULL only when the project could not be determined
  session_id  TEXT,
  source      TEXT NOT NULL,
  reason      TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  count       INTEGER NOT NULL DEFAULT 1 CHECK (count > 0),
  recoverable INTEGER NOT NULL CHECK (recoverable IN (0,1)),
  resolved_at TEXT
) STRICT;
CREATE INDEX capture_gaps_project ON capture_gaps(project_id, occurred_at);

-- Instructions are separate from derived memories. Scope is explicit; global rows have no project.
CREATE TABLE instructions (
  id                  TEXT PRIMARY KEY,
  scope_kind          TEXT NOT NULL CHECK (scope_kind IN ('global','project','branch','task','session')),
  project_id          TEXT REFERENCES projects(id) ON DELETE CASCADE,
  scope_ref           TEXT,                 -- branch name, task id or session id for the narrower scopes
  state               TEXT NOT NULL CHECK (state IN ('draft','active','conflict','archived')),
  confirmation_status TEXT NOT NULL CHECK (confirmation_status IN ('suggested','confirmed')),
  created_at          TEXT NOT NULL,
  CHECK ((scope_kind = 'global') = (project_id IS NULL)),
  CHECK ((scope_kind IN ('global','project')) = (scope_ref IS NULL)),
  CHECK (state <> 'active' OR confirmation_status = 'confirmed')
) STRICT;
CREATE INDEX instructions_scope ON instructions(project_id, scope_kind, state);

CREATE TABLE instruction_versions (
  id             TEXT PRIMARY KEY,
  instruction_id TEXT NOT NULL REFERENCES instructions(id) ON DELETE CASCADE,
  revision       INTEGER NOT NULL CHECK (revision >= 1),
  text           TEXT NOT NULL CHECK (length(text) > 0),   -- verbatim permitted text
  text_digest    TEXT NOT NULL,
  evidence_id    TEXT REFERENCES evidence(id),
  status         TEXT NOT NULL CHECK (status IN ('draft','active','superseded','withdrawn')),
  supersedes_id  TEXT REFERENCES instruction_versions(id),
  author         TEXT NOT NULL,
  reason         TEXT,
  effective_from TEXT NOT NULL,
  created_at     TEXT NOT NULL,
  UNIQUE (instruction_id, revision)
) STRICT;
-- At most one active version per instruction; supersession is a status change, never an overwrite.
CREATE UNIQUE INDEX instruction_versions_one_active ON instruction_versions(instruction_id) WHERE status = 'active';

CREATE TRIGGER instruction_versions_text_immutable BEFORE UPDATE OF instruction_id, revision, text, text_digest,
  evidence_id, supersedes_id, author, effective_from, created_at ON instruction_versions BEGIN
  SELECT RAISE(ABORT, 'instruction version text is immutable');
END;

CREATE TABLE jobs (
  id              TEXT PRIMARY KEY,
  kind            TEXT NOT NULL,
  payload_ref     TEXT,
  state           TEXT NOT NULL CHECK (state IN ('queued','running','success','failed','canceled','retry_wait','needs_consent')),
  idempotency_key TEXT NOT NULL UNIQUE,
  attempt         INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  next_run        TEXT,
  lease_owner     TEXT,
  lease_until     TEXT,
  cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK (cancel_requested IN (0,1)),
  last_error_code TEXT,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
) STRICT;
CREATE INDEX jobs_runnable ON jobs(state, next_run);

-- Full-text index over evidence text. It is derived data: rebuildable from `evidence`, never a
-- source of truth. Queries must join back to `evidence` and filter by project_id.
CREATE VIRTUAL TABLE evidence_fts USING fts5(text, content='evidence', content_rowid='seq', tokenize='unicode61');

CREATE TRIGGER evidence_fts_insert AFTER INSERT ON evidence WHEN NEW.text IS NOT NULL BEGIN
  INSERT INTO evidence_fts(rowid, text) VALUES (NEW.seq, NEW.text);
END;
CREATE TRIGGER evidence_fts_delete AFTER DELETE ON evidence WHEN OLD.text IS NOT NULL BEGIN
  INSERT INTO evidence_fts(evidence_fts, rowid, text) VALUES ('delete', OLD.seq, OLD.text);
END;
