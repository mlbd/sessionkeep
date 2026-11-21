-- 0008: how a project's saved sessions reach a new session. 'asked': only when the assistant
-- looks them up. 'automatic': a note of recent sessions at every start, and with every request
-- the stored pieces that share its words.
ALTER TABLE projects ADD COLUMN recall TEXT NOT NULL DEFAULT 'asked' CHECK (recall IN ('asked','automatic'));
