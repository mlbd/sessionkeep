-- 0002: pausing a project keeps its chosen capture level, so resume restores it.
ALTER TABLE projects ADD COLUMN paused_at TEXT;
-- How the project came to exist: 'manual' (project add) or 'auto' (auto-add mode).
ALTER TABLE projects ADD COLUMN added_by TEXT NOT NULL DEFAULT 'manual' CHECK (added_by IN ('manual','auto'));
