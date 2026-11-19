-- 0004: rebuild the full-text index with word stemming, so "refresh" finds "refreshed" and
-- "decide" finds "decided". Found necessary in a live session: a search phrased slightly
-- differently from the stored sentence returned nothing. The index is derived data; it is
-- dropped and rebuilt from the evidence table.
DROP TRIGGER evidence_fts_insert;
DROP TRIGGER evidence_fts_delete;
DROP TABLE evidence_fts;

CREATE VIRTUAL TABLE evidence_fts USING fts5(text, content='evidence', content_rowid='seq', tokenize='porter unicode61');

CREATE TRIGGER evidence_fts_insert AFTER INSERT ON evidence WHEN NEW.text IS NOT NULL BEGIN
  INSERT INTO evidence_fts(rowid, text) VALUES (NEW.seq, NEW.text);
END;
CREATE TRIGGER evidence_fts_delete AFTER DELETE ON evidence WHEN OLD.text IS NOT NULL BEGIN
  INSERT INTO evidence_fts(evidence_fts, rowid, text) VALUES ('delete', OLD.seq, OLD.text);
END;

INSERT INTO evidence_fts(evidence_fts) VALUES ('rebuild');
