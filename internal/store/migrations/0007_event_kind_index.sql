-- 0007: find one kind of event in a project (failed commands, for the index of known errors)
-- without reading every event of the project.
CREATE INDEX events_project_kind ON events(project_id, kind);
