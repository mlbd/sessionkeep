-- 0003: task checkpoints. A checkpoint is the user's (or the assistant's, at the user's request)
-- note of where work stands, with the Git state at that moment so a later session can tell
-- whether the working tree has moved on. Checkpoints are immutable; a newer one supersedes by time.
CREATE TABLE checkpoints (
  id            TEXT PRIMARY KEY,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  note          TEXT NOT NULL CHECK (length(note) > 0),
  git_repo      INTEGER NOT NULL CHECK (git_repo IN (0,1)),
  git_branch    TEXT,
  git_head      TEXT,
  changed_files INTEGER NOT NULL DEFAULT 0 CHECK (changed_files >= 0),
  dirty_digest  TEXT,
  created_at    TEXT NOT NULL
) STRICT;
CREATE INDEX checkpoints_project_time ON checkpoints(project_id, created_at);

CREATE TRIGGER checkpoints_immutable BEFORE UPDATE ON checkpoints BEGIN
  SELECT RAISE(ABORT, 'checkpoints are immutable');
END;
