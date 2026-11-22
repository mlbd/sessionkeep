-- 0006: researched claims with checked sources (Phase 3, P3-01).
-- A claim is a statement about the outside world (an API, a price, a limit). It is "verified"
-- only while it has at least one quote that DevWeft itself found in a page it fetched; the
-- database refuses a verified claim without such a quote. "Stale" is never stored: it is worked
-- out from expires_at when the claim is read, so reading can never make a claim look current.
CREATE TABLE sources (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  url        TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (project_id, url),
  UNIQUE (project_id, id)
) STRICT;

-- What one successful fetch returned. The page text itself is not kept, only its digest and
-- the quotes used as evidence.
CREATE TABLE source_snapshots (
  id                TEXT PRIMARY KEY,
  project_id        TEXT NOT NULL,
  source_id         TEXT NOT NULL,
  fetched_at        TEXT NOT NULL,
  final_url         TEXT NOT NULL,
  http_status       INTEGER NOT NULL,
  etag              TEXT,
  last_modified     TEXT,
  content_hash      TEXT NOT NULL,
  bytes             INTEGER NOT NULL CHECK (bytes >= 0),
  truncated         INTEGER NOT NULL CHECK (truncated IN (0,1)),
  title             TEXT,
  extractor_version INTEGER NOT NULL,
  UNIQUE (project_id, id),
  FOREIGN KEY (project_id, source_id) REFERENCES sources(project_id, id) ON DELETE CASCADE
) STRICT;
CREATE INDEX source_snapshots_source ON source_snapshots(project_id, source_id, fetched_at);

CREATE TRIGGER source_snapshots_immutable BEFORE UPDATE ON source_snapshots BEGIN
  SELECT RAISE(ABORT, 'source snapshots are immutable');
END;

CREATE TABLE claims (
  seq           INTEGER PRIMARY KEY AUTOINCREMENT,
  id            TEXT NOT NULL UNIQUE,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  subject       TEXT NOT NULL CHECK (length(subject) > 0),
  statement     TEXT NOT NULL CHECK (length(statement) > 0),
  category      TEXT NOT NULL,
  applies_to    TEXT,
  state         TEXT NOT NULL CHECK (state IN ('candidate','verified','disputed','withdrawn','superseded')),
  verified_at   TEXT,
  expires_at    TEXT,
  superseded_by TEXT,
  note          TEXT,
  created_at    TEXT NOT NULL,
  CHECK (state <> 'verified' OR (verified_at IS NOT NULL AND expires_at IS NOT NULL)),
  UNIQUE (project_id, id)
) STRICT;
CREATE INDEX claims_project ON claims(project_id, state, expires_at);

CREATE TABLE claim_evidence (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL,
  claim_id    TEXT NOT NULL,
  snapshot_id TEXT NOT NULL,
  quote       TEXT NOT NULL CHECK (length(quote) > 0),
  created_at  TEXT NOT NULL,
  FOREIGN KEY (project_id, claim_id)    REFERENCES claims(project_id, id) ON DELETE CASCADE,
  FOREIGN KEY (project_id, snapshot_id) REFERENCES source_snapshots(project_id, id) ON DELETE CASCADE
) STRICT;
CREATE INDEX claim_evidence_claim ON claim_evidence(project_id, claim_id);

CREATE TRIGGER claim_evidence_immutable BEFORE UPDATE ON claim_evidence BEGIN
  SELECT RAISE(ABORT, 'claim evidence is immutable');
END;

-- The wording of a claim never changes: a correction is a new claim that supersedes it.
CREATE TRIGGER claims_text_immutable BEFORE UPDATE OF seq, id, project_id, subject, statement, category, applies_to, created_at ON claims BEGIN
  SELECT RAISE(ABORT, 'a claim is superseded, never rewritten');
END;
CREATE TRIGGER claims_insert_unverified BEFORE INSERT ON claims WHEN NEW.state = 'verified' BEGIN
  SELECT RAISE(ABORT, 'a claim is verified only after its evidence is stored');
END;
CREATE TRIGGER claims_verified_needs_evidence BEFORE UPDATE OF state, verified_at, expires_at ON claims
  WHEN NEW.state = 'verified' AND NOT EXISTS (SELECT 1 FROM claim_evidence e WHERE e.project_id = NEW.project_id AND e.claim_id = NEW.id) BEGIN
  SELECT RAISE(ABORT, 'a verified claim needs a quote from a fetched source');
END;

-- One row per quote per re-check, kept apart from the fetch it used.
CREATE TABLE claim_checks (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL,
  claim_id    TEXT NOT NULL,
  evidence_id TEXT NOT NULL REFERENCES claim_evidence(id) ON DELETE CASCADE,
  checked_at  TEXT NOT NULL,
  result      TEXT NOT NULL CHECK (result IN ('confirmed','unchanged','quote_missing','unavailable')),
  snapshot_id TEXT,          -- confirmed: the new snapshot; unchanged: the snapshot the server says still stands
  detail      TEXT,
  FOREIGN KEY (project_id, claim_id) REFERENCES claims(project_id, id) ON DELETE CASCADE
) STRICT;
CREATE INDEX claim_checks_claim ON claim_checks(project_id, claim_id, checked_at);

CREATE VIRTUAL TABLE claims_fts USING fts5(subject, statement, content='claims', content_rowid='seq', tokenize='porter unicode61');
CREATE TRIGGER claims_fts_insert AFTER INSERT ON claims BEGIN
  INSERT INTO claims_fts(rowid, subject, statement) VALUES (NEW.seq, NEW.subject, NEW.statement);
END;
CREATE TRIGGER claims_fts_delete AFTER DELETE ON claims BEGIN
  INSERT INTO claims_fts(claims_fts, rowid, subject, statement) VALUES ('delete', OLD.seq, OLD.subject, OLD.statement);
END;
