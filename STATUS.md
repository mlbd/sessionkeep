# Project status

**Last updated:** 2026-11-20 10:30 UTC • **Project name:** SessionKeep • **Active phase:** Phase 1 built, **awaiting the owner's gate decision** • **Approval:** ADR-0001, 0002, 0003, 0006, 0007 and `phases/PHASE_1_PLAN.md` accepted by the owner on 2026-11-19; rest-of-Phase-1 defaults accepted on 2026-11-20 (`DECISIONS.md` session 9). Awaiting owner: ADR-0008 (plain dashboard pages instead of React), D66 (logon list instead of scheduled task), D72 (Phase 1 gate). ADR-0004 (Codex) blocked; ADR-0005 (graph parser) proposed.

## Start here

`evidence/phase-1/ACCEPTANCE.md` is the single record of what Phase 1 does, what was measured, which test rows are pass / partial / not run, and which defects the acceptance run found.

## What exists (Go module `sessionkeep`, CGO-free, one program of about 20 MB)

- **Capture:** `sessionkeep hook <Event>` called by Claude Code hooks (field allowlist, capture level, redaction, durable spool, always exit 0); `sessionkeep ingest` under an OS-level lease; background drains started by the plugin's own hooks; recovery of interrupted captures.
- **Consent:** `setup` (manual / ask / auto), `project add|list|pause|resume|capture|which`, `exclude add|remove|list`.
- **Instructions and continuity:** `remember`, `instruction list|show|supersede|archive`, `checkpoint`, `resume`; instructions and the latest checkpoint are delivered at SessionStart in a bounded envelope.
- **Recall:** `memory search` (stemmed words, literal fallback, labelled partial matches), `memory get`.
- **Data control:** `export`, `import [--dry-run]`, `forget record|captured|project`.
- **Surfaces:** generated Claude Code plugin with three skills (`plugin write`), optional read-only MCP server (`mcp`), local dashboard (`dashboard`).
- **Lifecycle:** `install`, `uninstall`, `autostart`, `doctor`, `status`, `paths`, `version`.
- **Storage:** SQLite (modernc v1.62.0, SQLite 3.54.1) with engine gate; migrations 0001–0004.

## What has not happened

- **SessionKeep is not installed in the owner's Claude Code.** Every run used sandboxes and `--plugin-dir` or an isolated Claude profile. The real data directory and install folder do not exist. Nothing has been captured from the owner's real sessions.
- **Nobody has opened the dashboard in a browser.**
- Nothing has been run on Linux, macOS, arm64 or Windows 11 (they compile).
- No remote exists; all commits are local on `main`.

## Latest session record (session 14, 2026-11-20)

- Agent: Claude Code (claude-fable-5-2). See `git log` for the commits of this run (1A-9, 1B-1 … 1B-5 and two fixes).
- `bash scripts/check.sh` → **CHECK PASSED**, including `go test -race` (no race reported). Suite also green ten times in a row after the last concurrency fix.
- End-to-end demonstration with three real Claude Code sessions: every required step passed on the second run; the first run exposed a recall gap (checkpoint not delivered) that was fixed.
- 100,000-record run: word search p95 6.6–29 ms; literal search about 300–500 ms (**over the 300 ms target**); hook p95 21.5 ms.
- Product defects found by this acceptance run and fixed: drain failing on a briefly locked spool file; record lookup scanning every row; one event silently lost when a hook hit its time limit under disk load (now recovered by the drain).
- Process slips, recorded in the acceptance file: one commit made with a failing gate; one edit script that emptied a source file (restored from git).
- All ten pre-approved live sessions are used. Any further live session needs new approval.

## Not yet verified

A real install and real daily use; the dashboard in a browser; a real Windows logon running the auto-start entry; whether any console window appears; interactive-mode sessions; SessionStart after compaction or resume; coexistence with the owner's other plugins; disk full and power loss; fuzzing; 20 real concurrent sessions; import of an older data version; any other operating system; a working Codex review; trademark search for SessionKeep.

## Blockers / awaiting owner

1. **Phase 1 gate (D72):** accept for a private pilot with the partial rows listed in the acceptance file, or say what must be closed first.
2. ADR-0008: keep the plain dashboard pages or ask for React.
3. D66: auto-start through the per-user logon list (a scheduled logon task needs administrator rights).
4. If the gate is accepted: run `sessionkeep install` and the two `claude plugin` commands it prints.

## Next smallest task and acceptance checks

Nothing is approved beyond Phase 1. Depending on the owner's gate decision, the candidates are: (a) close named gaps from the acceptance file — disk-full test, literal-search speed, fuzzing of the hook and import parsers, transcript reconciliation, branch scope; (b) support the owner through a real install and a week of real use, collecting capture gaps and recall misses; (c) start Phase 2 planning. Phase 2 and later must not start before the owner approves the Phase 1 gate (`ROADMAP.md`).

## Update template

- Date, agent, phase, exact commit/diff and working tree state:
- Requirements addressed:
- Changed files and rationale:
- Commands and results (real, mocked or not run):
- Evidence IDs / source links:
- Open findings / blockers:
- Decisions made and approvals:
- Next smallest task and acceptance checks:
