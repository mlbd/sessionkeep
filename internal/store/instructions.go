package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrNoInstruction means no instruction matches the given id or id prefix.
	ErrNoInstruction = errors.New("no such instruction")
	// ErrAmbiguousInstruction means an id prefix matched more than one instruction.
	ErrAmbiguousInstruction = errors.New("more than one instruction matches")
	// ErrDuplicateInstruction means the same text is already active in the same scope.
	ErrDuplicateInstruction = errors.New("this exact instruction is already recorded in that scope")
	// ErrNotActive means the instruction is archived and cannot be changed.
	ErrNotActive = errors.New("instruction is not active")
)

// Instruction is a recorded instruction with its current version.
type Instruction struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"` // global | project
	ProjectID string `json:"project_id,omitempty"`
	State     string `json:"state"` // active | archived
	CreatedAt string `json:"created_at"`
	VersionID string `json:"version_id"`
	Revision  int    `json:"revision"`
	Text      string `json:"text"`
	Digest    string `json:"digest"`
	Recorded  string `json:"recorded_at"` // when the current version was written
}

// InstructionVersion is one entry in an instruction's history.
type InstructionVersion struct {
	ID         string `json:"version_id"`
	Revision   int    `json:"revision"`
	Status     string `json:"status"`
	Text       string `json:"text"`
	Digest     string `json:"digest"`
	Supersedes string `json:"supersedes,omitempty"`
	Reason     string `json:"reason,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// TextDigest is the SHA-256 of the exact instruction text.
func TextDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// NewInstruction is the input to AddInstruction. Scope is "global" (ProjectID empty) or "project".
type NewInstruction struct {
	ID, VersionID    string
	Scope, ProjectID string
	Text, Author     string
	Now              time.Time
}

// AddInstruction records a confirmed instruction and its first version, verbatim.
func (s *Store) AddInstruction(ctx context.Context, n NewInstruction) (Instruction, error) {
	ts := n.Now.UTC().Format("2006-01-02T15:04:05.000000000Z")
	digest := TextDigest(n.Text)
	var project any
	if n.Scope != "global" {
		project = n.ProjectID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Instruction{}, err
	}
	defer tx.Rollback()
	var dup int
	if err := tx.QueryRowContext(ctx, `select count(*) from instructions i join instruction_versions v on v.instruction_id = i.id and v.status = 'active'
		where i.state = 'active' and i.scope_kind = ? and i.project_id is ? and v.text_digest = ?`, n.Scope, project, digest).Scan(&dup); err != nil {
		return Instruction{}, err
	}
	if dup > 0 {
		return Instruction{}, ErrDuplicateInstruction
	}
	if _, err := tx.ExecContext(ctx, `insert into instructions(id,scope_kind,project_id,state,confirmation_status,created_at) values(?,?,?, 'active', 'confirmed', ?)`,
		n.ID, n.Scope, project, ts); err != nil {
		return Instruction{}, fmt.Errorf("record instruction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into instruction_versions(id,instruction_id,revision,text,text_digest,status,author,effective_from,created_at)
		values(?,?,1,?,?, 'active', ?,?,?)`, n.VersionID, n.ID, n.Text, digest, n.Author, ts, ts); err != nil {
		return Instruction{}, fmt.Errorf("record instruction text: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Instruction{}, err
	}
	return Instruction{ID: n.ID, Scope: n.Scope, ProjectID: n.ProjectID, State: "active", CreatedAt: ts, VersionID: n.VersionID, Revision: 1, Text: n.Text, Digest: digest, Recorded: ts}, nil
}

// SupersedeInstruction replaces the active text with a new version. The old version is kept,
// marked superseded, and never rewritten.
func (s *Store) SupersedeInstruction(ctx context.Context, id, newVersionID, text, reason, author string, now time.Time) (Instruction, error) {
	ts := now.UTC().Format("2006-01-02T15:04:05.000000000Z")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Instruction{}, err
	}
	defer tx.Rollback()
	var oldID, oldDigest string
	var rev int
	err = tx.QueryRowContext(ctx, `select v.id, v.revision, v.text_digest from instruction_versions v join instructions i on i.id = v.instruction_id
		where i.id = ? and i.state = 'active' and v.status = 'active'`, id).Scan(&oldID, &rev, &oldDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return Instruction{}, ErrNotActive
	}
	if err != nil {
		return Instruction{}, err
	}
	if oldDigest == TextDigest(text) {
		return Instruction{}, ErrDuplicateInstruction
	}
	if _, err := tx.ExecContext(ctx, `update instruction_versions set status = 'superseded' where id = ?`, oldID); err != nil {
		return Instruction{}, err
	}
	var why any
	if reason != "" {
		why = reason
	}
	if _, err := tx.ExecContext(ctx, `insert into instruction_versions(id,instruction_id,revision,text,text_digest,status,supersedes_id,author,reason,effective_from,created_at)
		values(?,?,?,?,?, 'active', ?,?,?,?,?)`, newVersionID, id, rev+1, text, TextDigest(text), oldID, author, why, ts, ts); err != nil {
		return Instruction{}, fmt.Errorf("record new version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Instruction{}, err
	}
	return s.GetInstruction(ctx, id)
}

// ArchiveInstruction retires an instruction: it stays in history and is no longer injected.
func (s *Store) ArchiveInstruction(ctx context.Context, id, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `update instructions set state = 'archived' where id = ? and state = 'active'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotActive
	}
	var why any
	if reason != "" {
		why = reason
	}
	if _, err := tx.ExecContext(ctx, `update instruction_versions set status = 'withdrawn', reason = coalesce(?, reason) where instruction_id = ? and status = 'active'`, why, id); err != nil {
		return err
	}
	return tx.Commit()
}

const instructionSelect = `select i.id, i.scope_kind, coalesce(i.project_id,''), i.state, i.created_at, v.id, v.revision, v.text, v.text_digest, v.created_at
	from instructions i join instruction_versions v on v.instruction_id = i.id
	and v.revision = (select max(revision) from instruction_versions where instruction_id = i.id)`

func scanInstructions(rows *sql.Rows) ([]Instruction, error) {
	defer rows.Close()
	out := []Instruction{}
	for rows.Next() {
		var i Instruction
		if err := rows.Scan(&i.ID, &i.Scope, &i.ProjectID, &i.State, &i.CreatedAt, &i.VersionID, &i.Revision, &i.Text, &i.Digest, &i.Recorded); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListInstructions returns instructions, global ones first, each group oldest first. With
// includeArchived false only active ones are returned.
func (s *Store) ListInstructions(ctx context.Context, includeArchived bool) ([]Instruction, error) {
	q := instructionSelect
	if !includeArchived {
		q += ` where i.state = 'active'`
	}
	rows, err := s.db.QueryContext(ctx, q+` order by (i.scope_kind <> 'global'), i.created_at, i.id`)
	if err != nil {
		return nil, err
	}
	return scanInstructions(rows)
}

// ActiveInstructions returns what applies in one project: active global instructions, then the
// project's own, each group oldest first. An empty projectID returns the global ones only.
func (s *Store) ActiveInstructions(ctx context.Context, projectID string) ([]Instruction, error) {
	rows, err := s.db.QueryContext(ctx, instructionSelect+` where i.state = 'active' and v.status = 'active'
		and (i.scope_kind = 'global' or (i.scope_kind = 'project' and i.project_id = ? and ? <> ''))
		order by (i.scope_kind <> 'global'), i.created_at, i.id`, projectID, projectID)
	if err != nil {
		return nil, err
	}
	return scanInstructions(rows)
}

// GetInstruction resolves an instruction by exact id or unique id prefix (at least 4 characters).
func (s *Store) GetInstruction(ctx context.Context, idOrPrefix string) (Instruction, error) {
	if len(idOrPrefix) < 4 || strings.ContainsAny(idOrPrefix, `%_\`) {
		return Instruction{}, ErrNoInstruction
	}
	rows, err := s.db.QueryContext(ctx, instructionSelect+` where i.id = ? or i.id like ? || '%'`, idOrPrefix, idOrPrefix)
	if err != nil {
		return Instruction{}, err
	}
	all, err := scanInstructions(rows)
	if err != nil {
		return Instruction{}, err
	}
	for _, i := range all {
		if i.ID == idOrPrefix {
			return i, nil
		}
	}
	switch len(all) {
	case 0:
		return Instruction{}, ErrNoInstruction
	case 1:
		return all[0], nil
	}
	return Instruction{}, ErrAmbiguousInstruction
}

// InstructionHistory returns every version of an instruction, oldest first.
func (s *Store) InstructionHistory(ctx context.Context, id string) ([]InstructionVersion, error) {
	rows, err := s.db.QueryContext(ctx, `select id, revision, status, text, text_digest, coalesce(supersedes_id,''), coalesce(reason,''), created_at
		from instruction_versions where instruction_id = ? order by revision`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InstructionVersion{}
	for rows.Next() {
		var v InstructionVersion
		if err := rows.Scan(&v.ID, &v.Revision, &v.Status, &v.Text, &v.Digest, &v.Supersedes, &v.Reason, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
