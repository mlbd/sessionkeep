-- 0005: optional session summaries written by a model (P2-03).
-- A summary is derived text, never evidence: it lives in its own table, names the model and
-- prompt version that wrote it, and says how much of the session the model was given. There is
-- at most one per session; a newer one replaces it.
CREATE TABLE summaries (
  seq             INTEGER PRIMARY KEY AUTOINCREMENT,
  id              TEXT NOT NULL UNIQUE,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  session_id      TEXT NOT NULL,
  text            TEXT NOT NULL CHECK (length(text) > 0),
  provider        TEXT NOT NULL,            -- e.g. claude-cli
  model           TEXT NOT NULL,            -- what the provider reported, else what was asked for
  prompt_version  INTEGER NOT NULL,
  source_count    INTEGER NOT NULL CHECK (source_count > 0),   -- text records in the session when it was written
  source_used     INTEGER NOT NULL CHECK (source_used > 0),    -- of those, how many the model was given
  redaction_count INTEGER NOT NULL DEFAULT 0 CHECK (redaction_count >= 0),
  created_at      TEXT NOT NULL,
  UNIQUE (project_id, session_id),
  FOREIGN KEY (project_id, session_id) REFERENCES sessions(project_id, id) ON DELETE CASCADE
) STRICT;

CREATE VIRTUAL TABLE summaries_fts USING fts5(text, content='summaries', content_rowid='seq', tokenize='porter unicode61');

CREATE TRIGGER summaries_fts_insert AFTER INSERT ON summaries BEGIN
  INSERT INTO summaries_fts(rowid, text) VALUES (NEW.seq, NEW.text);
END;
CREATE TRIGGER summaries_fts_delete AFTER DELETE ON summaries BEGIN
  INSERT INTO summaries_fts(summaries_fts, rowid, text) VALUES ('delete', OLD.seq, OLD.text);
END;
CREATE TRIGGER summaries_immutable BEFORE UPDATE ON summaries BEGIN
  SELECT RAISE(ABORT, 'summaries are replaced, never edited');
END;

-- A summary must not outlive anything it was written from: deleting any event of a session
-- (one record, a project's captured text, a whole project) deletes that session's summary.
CREATE TRIGGER summaries_drop_with_source AFTER DELETE ON events BEGIN
  DELETE FROM summaries WHERE project_id = OLD.project_id AND session_id = OLD.session_id;
END;
