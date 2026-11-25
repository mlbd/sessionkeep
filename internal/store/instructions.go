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
	// ErrNotProposed means the instruction is not waiting for approval.
	ErrNotProposed = errors.New("instruction is not waiting for approval")
)

// Who wrote an instruction's text, as stored with each version.
const (
	AuthorOwner  = "owner"  // the user, in their own terminal or on the dashboard
	AuthorClaude = "claude" // proposed by an AI agent; given to sessions only once the user approved it
	// AuthorReceived begins the author of an instruction received from someone else; the rest is
	// the name they gave. Instructions received before 0.17.2 are stored as AuthorHandoff.
	AuthorReceived = "received:"
	AuthorHandoff  = "handoff"
)

// The ids of what this computer received from others begin with these, so that it is never sent
// on as this computer's own and is shown to Claude as someone else's. A received session's
// prefix is on its host session id. Ids made here never begin so: they are hexadecimal UUIDs.
const (
	ReceivedInstructionPrefix = "hi"
	ReceivedCheckpointPrefix  = "ho"
	ReceivedSessionPrefix     = "handoff-"
)

// Instruction is a recorded instruction with its current version.
type Instruction struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"` // global | project
	ProjectID string `json:"project_id,omitempty"`
	State     string `json:"state"` // active | draft (waiting for the user's approval) | archived
	CreatedAt string `json:"created_at"`
	VersionID string `json:"version_id"`
	Revision  int    `json:"revision"`
	Text      string `json:"text"`
	Digest    string `json:"digest"`
	Recorded  string `json:"recorded_at"` // when the current version was written
	Author    string `json:"author"`      // who wrote the current version: AuthorOwner, AuthorClaude, AuthorReceived+name or AuthorHandoff
	Approved  bool   `json:"approved"`    // the user wrote or approved it; false while waiting and after it was turned down
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
	return s.addInstruction(ctx, n, false)
}

// ProposeInstruction records an instruction that waits for the user's approval. No session is
// given it until ApproveInstruction is called. The same text cannot be proposed twice in a scope,
// nor while it is already in use there.
func (s *Store) ProposeInstruction(ctx context.Context, n NewInstruction) (Instruction, error) {
	return s.addInstruction(ctx, n, true)
}

func (s *Store) addInstruction(ctx context.Context, n NewInstruction, proposed bool) (Instruction, error) {
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
	if err := tx.QueryRowContext(ctx, `select count(*) from instructions i join instruction_versions v on v.instruction_id = i.id and v.status in ('active','draft')
		where i.state in ('active','draft') and i.scope_kind = ? and i.project_id is ? and v.text_digest = ?`, n.Scope, project, digest).Scan(&dup); err != nil {
		return Instruction{}, err
	}
	if dup > 0 {
		return Instruction{}, ErrDuplicateInstruction
	}
	state, confirmation, status := "active", "confirmed", "active"
	if proposed {
		state, confirmation, status = "draft", "suggested", "draft"
	}
	if _, err := tx.ExecContext(ctx, `insert into instructions(id,scope_kind,project_id,state,confirmation_status,created_at) values(?,?,?,?,?,?)`,
		n.ID, n.Scope, project, state, confirmation, ts); err != nil {
		return Instruction{}, fmt.Errorf("record instruction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into instruction_versions(id,instruction_id,revision,text,text_digest,status,author,effective_from,created_at)
		values(?,?,1,?,?,?,?,?,?)`, n.VersionID, n.ID, n.Text, digest, status, n.Author, ts, ts); err != nil {
		return Instruction{}, fmt.Errorf("record instruction text: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Instruction{}, err
	}
	return Instruction{ID: n.ID, Scope: n.Scope, ProjectID: n.ProjectID, State: state, CreatedAt: ts, VersionID: n.VersionID, Revision: 1, Text: n.Text, Digest: digest, Recorded: ts, Author: n.Author, Approved: !proposed}, nil
}

// ApproveInstruction puts an instruction that was waiting for approval into use. It fails with
// ErrDuplicateInstruction when the same text came into use in that scope in the meantime.
func (s *Store) ApproveInstruction(ctx context.Context, id string) (Instruction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Instruction{}, err
	}
	defer tx.Rollback()
	var scope, digest string
	var project sql.NullString
	err = tx.QueryRowContext(ctx, `select i.scope_kind, i.project_id, v.text_digest from instructions i join instruction_versions v on v.instruction_id = i.id and v.status = 'draft'
		where i.id = ? and i.state = 'draft'`, id).Scan(&scope, &project, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return Instruction{}, ErrNotProposed
	}
	if err != nil {
		return Instruction{}, err
	}
	var dup int
	if err := tx.QueryRowContext(ctx, `select count(*) from instructions i join instruction_versions v on v.instruction_id = i.id and v.status = 'active'
		where i.state = 'active' and i.scope_kind = ? and i.project_id is ? and v.text_digest = ?`, scope, project, digest).Scan(&dup); err != nil {
		return Instruction{}, err
	}
	if dup > 0 {
		return Instruction{}, ErrDuplicateInstruction
	}
	if _, err := tx.ExecContext(ctx, `update instructions set state = 'active', confirmation_status = 'confirmed' where id = ?`, id); err != nil {
		return Instruction{}, err
	}
	if _, err := tx.ExecContext(ctx, `update instruction_versions set status = 'active' where instruction_id = ? and status = 'draft'`, id); err != nil {
		return Instruction{}, err
	}
	if err := tx.Commit(); err != nil {
		return Instruction{}, err
	}
	return s.GetInstruction(ctx, id)
}

// RejectInstruction turns down an instruction that was waiting for approval. It is kept, retired,
// so that the same proposal arriving again from a teammate is not taken in a second time.
func (s *Store) RejectInstruction(ctx context.Context, id, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `update instructions set state = 'archived' where id = ? and state = 'draft'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotProposed
	}
	var why any
	if reason != "" {
		why = reason
	}
	if _, err := tx.ExecContext(ctx, `update instruction_versions set status = 'withdrawn', reason = coalesce(?, reason) where instruction_id = ? and status = 'draft'`, why, id); err != nil {
		return err
	}
	return tx.Commit()
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

const instructionSelect = `select i.id, i.scope_kind, coalesce(i.project_id,''), i.state, i.created_at, v.id, v.revision, v.text, v.text_digest, v.created_at, v.author, i.confirmation_status = 'confirmed'
	from instructions i join instruction_versions v on v.instruction_id = i.id
	and v.revision = (select max(revision) from instruction_versions where instruction_id = i.id)`

func scanInstructions(rows *sql.Rows) ([]Instruction, error) {
	defer rows.Close()
	out := []Instruction{}
	for rows.Next() {
		var i Instruction
		if err := rows.Scan(&i.ID, &i.Scope, &i.ProjectID, &i.State, &i.CreatedAt, &i.VersionID, &i.Revision, &i.Text, &i.Digest, &i.Recorded, &i.Author, &i.Approved); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListInstructions returns instructions, global ones first, each group oldest first. With
// includeArchived false only those in use and those waiting for approval are returned.
func (s *Store) ListInstructions(ctx context.Context, includeArchived bool) ([]Instruction, error) {
	q := instructionSelect
	if !includeArchived {
		q += ` where i.state in ('active','draft')`
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
