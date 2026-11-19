# Phase 0 evidence — session 1 (2026-11-19)

**Agent:** Claude Code (model claude-fable-5-2). **Window:** approx. 2026-11-19 15:00–16:05 UTC (per-source timestamps were not individually recorded; all sources below were fetched inside this window). **Working tree:** not a Git repository; no commit exists. **Scope actually executed:** read-only machine inventory, live documentation/source-metadata research, local CLI `--help`/status probes, and proxy measurements with synthetic data in a temp directory. **Nothing was installed, no Claude/Codex configuration was changed, no AI job was invoked, nothing was uploaded.**

Result labels: **verified** (checked against the stated source or measured here), **doc-level** (official documentation read, not exercised at runtime), **failed**, **not tested**.

> **Session 2 update (2026-11-19, ~16:10–16:30 UTC):** §9 at the end records runtime results obtained after the owner approved a project-local Go install, one live Claude hook run and one Codex review. Where §9 and the tables below disagree, §9 is newer.

## 1. Ticket status (as of session 1)

| Ticket | Status | What exists | What is missing |
|---|---|---|---|
| P0-01 inventory | **verified** | §2 | — |
| P0-02 Claude hooks/plugin | **doc-level** + spawn cost **measured** | §3, §5 (H1–H12) | Live payload fixtures and injection proof on installed 2.1.328: **not tested** (needs owner consent: an isolated `claude -p --plugin-dir` run consumes subscription usage) |
| P0-03 native memory / Claude-Mem | **partial** | License scope and release pinned (§4) | Architecture comparison at commit level, issue status re-check, native memory docs: **not tested** |
| P0-04 Go + SQLite spike | **doc-level**; C-SQLite proxy **measured** | §4, §6 | Go is not installed. Real driver, embedded version via SQL, FTS5, backup API, cross-build, binary size: **not tested** |
| P0-05 spool/replay/latency | proxy **measured** | §6 | Go prototype, kill/replay idempotency, power-loss boundary, supervisor: **not tested** |
| P0-06 Codex | local help + auth readiness **verified**; docs **doc-level** | §5 (C1–C8) | Real read-only review, cancellation, filesystem before/after, Windows sandbox enforcement: **not tested** (needs consent) |
| P0-07 graph parser | **doc-level** survey | §5 (G1–G4) | Gold-edge spike, CPU/memory, any parser runtime test: **not tested** |
| P0-08 research providers | **not tested** | Spec review only | Provider terms, SSRF fixture |
| P0-09 backup/Restic/S3 | license/version **verified** only | §4 | Local snapshot prototype, key model, transcript resume: **not tested** |
| P0-10 threat model / name / license | name collision **verified**; rest partial | §5 (N1–N3), §7 | Full dependency licence audit (no dependencies chosen yet), trademark search by a qualified party |
| P0-11 baseline + plan | **proposed** | `docs/adr/`, `phases/PHASE_1_PLAN.md` | Owner approval; independent Codex critique not requested (not authorized) |

**Phase 0 is not complete.** The gate cannot be passed until the "not tested" runtime items for P0-02, P0-04, P0-05 and P0-06 are run or explicitly waived by the owner.

## 2. P0-01 machine inventory (verified, read-only commands)

| Item | Observed |
|---|---|
| OS | Windows 10 Pro 10.0.19045 (22H2), 64-bit, NTFS system volume |
| CPU / RAM / disk | Intel Core i5-6500 3.20 GHz, 4 cores / 4 threads; 15.9 GB; SATA SSD 932 GB, ~285 GB free |
| Shells | Windows PowerShell 5.1.19041; Git Bash (Git 2.47.1.windows.1); WSL 2 present (default distro Ubuntu). PowerShell 7 (`pwsh`) absent |
| Long paths | `LongPathsEnabled = 0` (260-char limit active for non-opted-in APIs) |
| Claude Code | 2.1.328, installed via npm shim (`claude.ps1`/`.cmd`), running under a **non-default Claude config directory** |
| Codex CLI | codex-cli 0.167.1 (npm shim); `codex login status` → "Logged in using ChatGPT" |
| Node / npm / Bun / Python | v24.21.0 / 11.19.0 / 1.3.5 / 3.11.9 |
| GitHub CLI | 2.74.0 |
| **Absent** | Go, gcc, zig, sqlite3 CLI, restic, rclone, cosign, goreleaser |

Environment facts that affect design: (a) Go is missing, so no stack prototype was possible; (b) the owner's Claude Code uses a non-default config directory, so setup/doctor must never assume `~/.claude`; (c) Claude-Mem, the Codex plugin and several other plugins with SessionStart hooks are already installed here, so coexistence is a real test condition, not a hypothetical; (d) both CLIs are npm `.ps1/.cmd` shims, which exec-form hooks cannot launch.

Reproduce: `Get-CimInstance Win32_OperatingSystem`, `Win32_Processor`, `Get-PhysicalDisk`, `Get-Command <tool>`, `<tool> --version`, `codex login status`.

## 3. Hook contract summary for the installed version (doc-level, checked 2026-11-19)

Source: <https://code.claude.com/docs/en/hooks> (294 k chars, read in full across three fetches) and <https://code.claude.com/docs/en/plugins-reference>.

- Common stdin fields: `session_id`, `prompt_id`, `transcript_path`, `cwd`, `scratchpad_dir`, `permission_mode`, `effort`, `hook_event_name`, optional `agent_id`/`agent_type`.
- `SessionStart`: `source` ∈ startup/resume/clear/compact/fork, optional `model`, `session_title`. Output `hookSpecificOutput.additionalContext`.
- `UserPromptSubmit`: `prompt` (pasted placeholders expanded). Output `additionalContext`; default timeout **30 s**.
- `PostToolUse`: `tool_name`, `tool_input`, `tool_response`, `tool_use_id`, `duration_ms`. `PostToolUseFailure`: `error`, `is_interrupt`.
- `Stop`: `stop_hook_active`, **`last_assistant_message`**, `background_tasks`, `session_crons`. `StopFailure`: `error` type enum, `error_details`.
- `PostCompact`: `trigger`, `compact_summary`. `SessionEnd`: `reason`; no decision control.
- 33 events exist in total, including `PreCompact`, `SubagentStop`, `TaskCreated/Completed`, `CwdChanged`, `WorktreeCreate/Remove`.

## 4. Upstream versions and licences (verified via GitHub API / Go module proxy, 2026-11-19)

| Component | Version / commit | Licence | Note |
|---|---|---|---|
| Go | 1.27.4 and 1.26.11 (both 2026-11-10) are the supported lines | BSD-3 | <https://go.dev/doc/devel/release> |
| SQLite | newest 3.54.1 (2026-10-16); patch releases 3.53.6/3.50.8/3.44.7 on 2026-11-13 | public domain | <https://www.sqlite.org/chronology.html> |
| modernc.org/sqlite | v1.62.0, 2026-11-07, commit `d04d243c`; `go 1.26.0`; requires `modernc.org/libc v1.80.1` exactly | BSD-3-Clause | pkg.go.dev table lists embedded SQLite 3.54.1 (the same page's benchmark text says 3.53.4 — **inconsistent, must be confirmed with `select sqlite_version()`**) |
| MCP Go SDK | v1.8.0, tag commit `3f3b699b`; `go 1.25.0` | MIT → Apache-2.0 transition (both stated in LICENSE) | official `modelcontextprotocol/go-sdk` |
| Claude-Mem | v16.0.1 (2026-11-19), head `4b48c572` | Apache-2.0; `docs/license.md` reserves hosted cloud, team sync, enterprise, premium knowledge agents, private evals | TypeScript/JS; no code reuse planned |
| Graphify | v0.9.112 (2026-11-17), head `340268a7` | Apache-2.0 with MIT-licensed earlier portions (NOTICE, LICENSE-MIT) | **~12 MB of Python**; cannot be embedded under the no-Python rule |
| codex-plugin-cc | v1.0.6 (2026-07-08), head `db52e28f` | Apache-2.0 | reference only |
| openai/codex | rust-v0.170.0 (2026-11-18); **installed is 0.167.1** | Apache-2.0 | three releases behind within days — fast drift |
| restic | v0.20.0 (2026-11-19) | BSD-2-Clause | Phase 7 candidate only |
| wazero | v1.12.0 | Apache-2.0 | WASM parser route |
| tree-sitter/go-tree-sitter | no releases; head `c9492002` (2025-11) | MIT | CGO binding |
| sqlite-vec | v0.1.9 (2026-03-31) | Apache-2.0 | optional, later |
| GoReleaser | v2.20.2 | MIT | release tooling, later |

## 5. Findings register

### Claude Code hooks and plugins

| ID | Finding | Status | Consequence |
|---|---|---|---|
| H1 | `additionalContext`, `systemMessage`, `initialUserMessage` and plain stdout are each capped at **10,000 characters**. Over the cap the text is saved to a file and replaced by the path plus a 2,000-character preview; "Claude Code doesn't ask Claude to read the file"; no setting raises the cap | doc-level | Instruction injection needs an explicit budget and a coverage manifest; see ADR-0002 |
| H2 | Several hooks returning `additionalContext` for one event are all delivered; each string is measured separately | doc-level | Possible way to carry more than 10 k, **not tested**; do not rely on it yet |
| H3 | `transcript_path` is "written asynchronously and may lag the current turn"; at Stop "the transcript file isn't guaranteed to include the final message … on all versions". The docs recommend `last_assistant_message` | doc-level | Primary capture must use documented hook fields; transcript parsing becomes secondary reconciliation |
| H4 | SessionEnd shares a 1.5 s budget and "timeouts set on plugin-provided hooks don't raise the budget" | doc-level | Confirms the spec: nothing durable may depend on SessionEnd |
| H5 | Exec form (`command` + `args`) on Windows needs a real `.exe`; `.cmd`/`.bat` shims are not executables. Shell form runs through Git Bash, or PowerShell when Git Bash is absent | doc-level | Hook must be exec form pointing at the absolute path of the native binary |
| H6 | Process spawn on this machine: native exe direct p50 13 ms; through Git Bash p50 69 ms / p99 122 ms; through PowerShell p50 190 ms; `node -e ''` p50 59 ms | **verified (measured)** | Shell-form or Node hooks consume most or all of the 100 ms p95 budget before any work is done |
| H7 | Without Git Bash, a PreToolUse/PermissionRequest command hook that exits with a code other than 0 or 2 **blocks** the tool call | doc-level | Do not ship PreToolUse hooks in the core; always exit 0 on recoverable failure |
| H8 | Async hooks are killed at `-p` teardown; results are delivered only while the session runs | doc-level | Confirms capture must be synchronous and fast |
| H9 | Stop-hook continuation is capped at 8 consecutive blocks; `stop_hook_active` signals a continuation | doc-level | Bounded-gate design (later phases) has a documented host guard |
| H10 | "Text framed as out-of-band system commands can trigger Claude's prompt-injection defenses" | doc-level | Injected instructions must be worded as factual records with provenance; needs a live test of recall fidelity |
| H11 | `${CLAUDE_PLUGIN_ROOT}` changes on every plugin update; `${CLAUDE_PLUGIN_DATA}` persists across updates but is **deleted on uninstall by default** | doc-level | User data must live in the OS data directory, never in plugin directories |
| H12 | `claude plugin validate`, `--plugin-dir`, `--settings`, `--setting-sources`, `--strict-mcp-config` exist locally (2.1.328 help); manifest `engines` needs ≥ 2.1.323 | verified (local help) + doc-level | An isolated fixture run is feasible without touching the owner's configuration |

### Codex CLI

| ID | Finding | Status |
|---|---|---|
| C1 | Local 0.167.1 `codex exec` has `--sandbox read-only|workspace-write|danger-full-access`, `--json`, `--output-schema <FILE>`, `-o/--output-last-message`, `-C/--cd`, `--skip-git-repo-check`, `--ephemeral`, `--ignore-user-config`, `--ignore-rules`; prompt from stdin with `-` | verified (local help) |
| C2 | `codex exec review` exists with `--uncommitted`, `--base`, `--commit`, plus `--json`/`--output-schema`/`-o` | verified (local help) |
| C3 | `codex exec` defaults to a read-only sandbox, reuses saved CLI auth, emits JSONL events `thread.started`, `turn.*`, `item.*`, `error` | doc-level (<https://learn.chatgpt.com/docs/non-interactive-mode>) |
| C4 | Auth readiness is observable without reading credential files: `codex login status` | verified |
| C5 | `--full-auto` is no longer supported | doc-level; consistent with local help |
| C6 | Native-Windows sandbox enforcement of read-only: official page not found (guessed URL returned 404); public issues from 0.152–0.169 conflict (reads outside allowed roots reported; restricted-token failures reported) | **not verified — treat as unknown** |
| C7 | The docs do not state whether the read-only sandbox blocks network access | not verified |
| C8 | Codex releases every few days (0.167.1 installed, 0.170.0 current) | verified |

### Graph parser

| ID | Finding | Status |
|---|---|---|
| G1 | The official Go binding is CGO; it would end CGO-free cross-compilation of the core | verified (repo) |
| G2 | Pure-Go routes exist: `odvcencio/gotreesitter` (first published Feb 2026, single author, self-reported benchmarks), `msuozzo/bonsai` (grammar→WASM→Go, TypeScript on tree-sitter-typescript v0.23.2), a wazero wrapper marked pre-release | search-level only; none inspected or run |
| G3 | Graphify is a Python application | verified |
| G4 | No candidate has been tested for correctness on TS/TSX | not tested |

### Name

| ID | Finding | Status |
|---|---|---|
| N1 | 94 GitHub repositories match "devintel" in the name, including an "AI-powered developer productivity platform" called DevIntel AI. npm `devintel` is unregistered (404) | verified |
| N2 | The project folder is named "SessionKeep". One GitHub repository of that name exists (0 stars); npm `sessionkeep` is unregistered | verified |
| N3 | "DevIntel" contains the mark of a large chip vendor. This is my risk observation, not legal advice; no trademark database was searched | not tested |

## 6. Proxy measurements (synthetic data, this machine)

Script: `evidence/phase-0/probes/probe.py` (Python 3.11.9, C SQLite 3.45.1 bundled with Python). **This is not the target Go/modernc stack.** It measures the storage device, the OS process-spawn floor and SQLite engine behaviour; Go results must be measured separately. Command: `python -I probe.py <empty-temp-dir>`. One run, no repetition across reboots, machine not idle (a Claude Code session was running).

| Measurement | n | p50 | p95 | p99 |
|---|---:|---:|---:|---:|
| Spool: write 2 KB temp file + fsync + atomic rename | 300 | 1.91 ms | 2.42 ms | 2.67 ms |
| Same without fsync | 300 | 0.78 ms | 0.98 ms | 1.14 ms |
| Spawn native exe directly | 40 | 13.2 ms | 14.0 ms | 17.3 ms |
| Spawn via Git Bash `-c` | 40 | 69.2 ms | 72.8 ms | 122.2 ms |
| Spawn via `powershell -NoProfile` | 15 | 190.2 ms | 195.2 ms | 212.2 ms |
| Spawn `node -e ''` | 40 | 58.6 ms | 67.8 ms | 70.1 ms |
| Single-row commit, WAL, `synchronous=FULL` | 200 | 0.94 ms | 1.36 ms | 1.63 ms |
| FTS5 bm25 top-20 with project filter, 100k rows (64 MB), warm | 200 | 0.91 ms | 10.7 ms | 11.2 ms (max 52.5) |

Reading: an exec-form native hook has roughly 85 ms of headroom under the 100 ms p95 target after spawn; durable spool I/O costs about 2–3 ms. The 300 ms search target looks comfortable for the C engine; modernc is a transpiled build and must be measured. Caveat: sub-3 ms fsync on a SATA SSD suggests a volatile drive write cache, so "fsync returned" is not proof of power-loss durability on this hardware. The Python SQLite here is 3.45.1, which is inside the WAL-reset-bug range; irrelevant to the product but a reminder to assert the version at startup.

## 7. Spec conflicts and weaknesses found (details and proposals in the ADRs)

1. **ARCHITECTURE §1 calls MCP "a short-lived stdio adapter".** A stdio MCP server is started by the host per session and stays up for the session. Twenty sessions means twenty resident processes, which conflicts with the "one persistent process per user" framing and the idle-RSS target as written. (Based on how stdio MCP servers work in Claude Code; resident lifetime on 2.1.328 not measured here.)
2. **Daemon-optional vs "one writer service; hooks read-only".** If the engine never runs, nothing ingests the spool, so search and instruction versions never update. The spec has no ingester for manual mode.
3. **Acceptance outcome 1 ("not truncated silently") vs H1.** Over-limit context is replaced by a preview without the model being told to read the rest.
4. **Transcript reconciliation is treated as a main capture path** (02-MEMORY, P1-06); H3 says the transcript lags and the format is not a documented contract.
5. **Hook launch form is unspecified**; H5/H6 show it decides whether the latency target is reachable on Windows.
6. **Read-only Codex review is assumed enforceable**; C6 says this is unknown on native Windows.
7. **D11 "native deterministic graph, Tree-sitter or native parser"** has no CGO-free option that is both mature and tested (G1–G4).
8. **Phase 1 bundles thirteen tickets** including a React UI, installer, supervisor and export/import before the capture→recall loop has been proven once.
9. **modernc.org/sqlite pins `modernc.org/libc` exactly**; any other dependency that pulls libc at a different version breaks the build or behaviour. Single-maintainer project.
10. **Reference machine runs Windows 10 22H2 with long paths disabled.** Windows 10's support status and Go 1.27's minimum Windows version were not re-verified today.

## 8. Not done, and why

- No Go toolchain → no P0-04/P0-05 prototype. Requires approval to install Go 1.27.4 (or unpack the official zip into a project-local tools directory without changing PATH).
- No live hook fixture → requires approval for one isolated `claude -p` run with a temp plugin directory and `--setting-sources` restricted, synthetic prompt only.
- No real Codex review → requires approval; would use a synthetic repository in a temp directory and record filesystem digests before/after.
- P0-08 and most of P0-09 not started. macOS/Linux: nothing tested.
- Private context file was read for history; nothing from it was copied into these records.

## 9. Session 2 runtime results (2026-11-19)

Owner approvals given in conversation: install Go project-locally; one isolated `claude -p` hook run; one synthetic Codex read-only review; `git init`; project name **SessionKeep**. All data synthetic. The owner's Claude and Codex configuration files were not edited.

**Setup.** Go 1.27.4 `windows-amd64.zip` from go.dev, SHA-256 `178ab989…5cde1e7e` matched the value published at `https://go.dev/dl/?mode=json`. Unpacked to `.tools/go` (git-ignored); `GOPATH` and `GOCACHE` also under `.tools`; system PATH unchanged; `CGO_ENABLED=0`, `GOTOOLCHAIN=local`. `git init` done, branch `main`, nothing committed.

### 9.1 P0-04 Go + SQLite spike — verified on windows/amd64 (`spikes/sqlite/main.go`)

| Check | Result |
|---|---|
| `select sqlite_version()` with modernc.org/sqlite v1.62.0 + libc v1.80.1 | **3.54.1** (≥ 3.51.3, WAL-reset fix included) |
| `pragma compile_options` | `ENABLE_FTS5`, `THREADSAFE=1` |
| WAL + `synchronous=FULL` | journal_mode reports `wal` |
| Bulk insert 100k rows + FTS5 index, one transaction | 4.7 s, 62 MB |
| 1 writer (durable single-row commits) with 4 read-only connections querying for 4 s | writer n=3413, p50 1.00 ms, p95 3.36 ms, p99 6.90 ms; **0 writer errors, 0 reader errors** |
| FTS5 bm25 top-20 with project filter at 100k rows, during writes | n=1120, p50 3.26 ms, p95 37.9 ms, p99 44.3 ms (target ≤ 300 ms) |
| Online backup (`NewBackup`/`Step`/`Finish`) started during load | completed; copy `integrity_check` = ok, 103,413 rows, FTS query works on the copy |
| Source `integrity_check` after test | ok |
| Cross-build, `-trimpath -ldflags "-s -w"` | windows/amd64 6.6 MB, windows/arm64 6.3 MB, linux/amd64 6.8 MB, linux/arm64 6.6 MB, darwin/arm64 6.6 MB, darwin/amd64 6.8 MB — **all compiled; only windows/amd64 was executed** |

**Finding B1 (backup under write load).** The backup, taken from a separate connection, needed 4.07 s and its copy contains every row the writer produced, so it finished only when the writer stopped. Likely cause: SQLite restarts a backup when another connection modifies the source (not re-checked against the SQLite backup page in this session). Consequence: the lease holder must run the backup on its own writer connection or pause ingest for it, and a backup needs a deadline and a "not completed" outcome.

### 9.2 P0-05 spool, lease and hook latency — verified on windows/amd64 (`spikes/spool/`)

| Check | Result |
|---|---|
| Hook exe: read 2 KB stdin → temp file → `Sync` → rename → exit; timed from spawn by the driver | n=1000, **p50 12.7 ms, p95 14.4 ms, p99 20.4 ms**, max 39.3 ms (an earlier run had one 565 ms outlier; cause not investigated) |
| Fail-open: invalid JSON; missing spool directory | exit 0 and empty stdout in both cases |
| Second consumer while the first holds the `LockFileEx` lease | exits with "lease held by another process" |
| Lease after a hard kill of the holder | next consumer acquired it (the OS released the lock) |
| Crash/replay: 1000 events, 31 drain rounds with 13 exits before commit, 6 exits after commit but before file delete, 10 hard kills | **rows = 1000, distinct = 1000, integrity ok, spool empty** |

Limits: one machine; process crashes only — no power-loss or disk-full test; antivirus interference not examined; single-threaded throwaway consumer. The first driver version injected only one crash before the spool drained; its result was discarded and the driver was changed to force crashes through the whole drain.

### 9.3 P0-02 live hook fixture — verified on Claude Code 2.1.328 in print mode (`spikes/hookfixture/`, fixtures in `fixtures/claude-2.1.328/`)

Command shape: `claude -p <synthetic prompt> --plugin-dir <temp> --setting-sources project --strict-mcp-config --no-session-persistence --model sonnet --allowedTools Read --output-format json`, run in a temp project whose path contains a space and a non-ASCII character. One run, 2 turns, 3.4 s, reported cost USD 0.0485.

| Check | Result |
|---|---|
| `claude plugin validate` on the fixture plugin | Validation passed |
| Exec-form hook `"command": "${CLAUDE_PLUGIN_ROOT}/tools/recorder.exe"` with `args` | launched for SessionStart, UserPromptSubmit, PostToolUse, Stop, SessionEnd; `CLAUDE_PLUGIN_ROOT` and `CLAUDE_PROJECT_DIR` present in the process environment |
| SessionStart `additionalContext` (110 chars) | the model repeated the codeword placed there |
| UserPromptSubmit `additionalContext` of **8,903 characters**, codeword in the last line | the model repeated it correctly |
| `Stop.last_assistant_message` | equals the final answer exactly |
| Observed stdin keys | SessionStart: `cwd, hook_event_name, session_id, source, transcript_path` (no `model`, no `permission_mode`). UserPromptSubmit adds `prompt, prompt_id, permission_mode`. PostToolUse adds `tool_name, tool_input, tool_response, tool_use_id, duration_ms, effort`. Stop adds `stop_hook_active, last_assistant_message, background_tasks, session_crons, effort`. SessionEnd adds `reason, prompt_id` |
| `transcript_path` | present in every payload, but the file **did not exist** (session persistence was off) |

Limits: print mode only, one model, one run. Interactive mode, `resume`/`compact` sources, PostToolUseFailure, StopFailure and PostCompact were not triggered. A filler-padded envelope is an easier case than 9,000 characters of real instructions. Whether the owner's other installed plugins also ran hooks in this session was not checked, so coexistence remains untested. Fixture payloads have user name, config directory name and UUIDs replaced by placeholders.

### 9.4 P0-06 Codex read-only review — run; the review itself failed on this machine (fixtures in `fixtures/codex-0.167.1/`)

Command shape: `codex exec --sandbox read-only --json --output-schema <schema> -o <result> --ephemeral --ignore-rules -C <temp git repo> -` with the prompt on stdin. Repository: two synthetic files with two seeded defects, under the user's temp directory. One run, 235 s, exit 0; usage in the event stream: 216,782 input tokens (192,384 cached), 987 output tokens.

| Check | Result |
|---|---|
| Flags accepted; JSONL events parse (`thread.started`, `turn.started`, `item.started/completed`, `turn.completed` with usage) | yes |
| `--output-schema` honoured; `-o` file written outside the `-C` directory | yes; result matches the schema |
| Working-tree digest, `git status`, HEAD before vs after | **identical**; no probe file created |
| Reviewer could read the repository | **No.** Commands in the repo directory failed with `CreateProcessWithLogonW failed: 267`; a later attempt returned `Get-Content : Access is denied`. Verdict `incomplete`, zero findings — neither seeded defect was found |
| Write probe / network probe | both failed before the shell started, so write-blocking and network behaviour are **still unverified** |
| Other observations | one shell call ran with the working directory at the user's home, not `-C`; one `mcp_tool_call` item appeared (user-level Codex config was loaded because `--ignore-user-config` was not passed) |

Reading: with Codex 0.167.1 on native Windows, the sandbox identity apparently cannot access a repository under the user's temp directory. Error 267 is the Windows "directory name is invalid" code; the cause (sandbox user ACLs, location, elevated vs. unelevated mode) was not diagnosed. Reviewing a disposable copy in a temp directory, as ADR-0004 proposed, **does not work as written on this machine**. The reviewer reported `incomplete` rather than inventing findings, and nothing was modified. A second run in another location would be a new paid job and was not started.

### 9.5 Ticket status after session 2

| Ticket | Status |
|---|---|
| P0-01 | verified |
| P0-02 | verified for print mode on 2.1.328 (limits in 9.3); interactive mode and coexistence not tested |
| P0-03 | partial (unchanged) |
| P0-04 | verified on windows/amd64; other targets compile only |
| P0-05 | verified for process crashes on windows/amd64; power loss, disk full and supervisor not tested |
| P0-06 | flags, schema, output path and no-write outcome verified; **a working read-only review is not demonstrated**; sandbox write/network behaviour unknown |
| P0-07, P0-08, P0-09 | unchanged (survey only / not started) |
| P0-10 | name chosen by owner (SessionKeep); trademark search not done; no full dependency licence audit |
| P0-11 | proposal exists; owner approval pending; ADR-0006 and the ADR-0003 manual-mode question unanswered |

### 9.6 Session 3 (2026-11-19): approvals and second Codex run

Owner reply "yes, let's continue" to the five open items: ADR-0001/0002/0003 accepted, Phase 1A/1B split accepted (ADR-0006), manual-mode drain accepted, second Codex test approved, first local commit approved.

**Second Codex read-only review** (`spikes/codexreview/run.sh`, result in `fixtures/codex-0.167.1/result-run2.json`). Same synthetic repository, schema and prompt; two changes: repository placed outside the user profile (inside the git-ignored `.tools/` folder of this project) and `--ignore-user-config` added. 25 s, exit 0, 56,958 input tokens (37,376 cached), 565 output tokens.

| Check | Result |
|---|---|
| Working-tree digest, `git status`, HEAD before vs after | identical; no probe file created |
| Reviewer could read the repository | **No.** No command was executed at all: the event stream contains no `command_execution` items, and the result quotes `rejected: blocked by policy` for the attempted PowerShell commands. Verdict `incomplete`, zero findings |
| Write / network probes | rejected before execution — still no evidence of how a *running* sandbox treats writes or network |
| User MCP servers | not loaded this time (no `mcp_tool_call` items) |

Reading: with the user configuration ignored, Codex 0.167.1 on native Windows refuses every shell command under `--sandbox read-only`; with the user configuration loaded (run 1) commands are attempted but fail to start in, or read, the target directory. A public issue found in session 1 describes the same "blocked by policy" symptom and attributes it to no Windows sandbox backend being configured; that explanation is **not verified here**. Neither failure was diagnosed further. **Two real runs, zero working reviews.** ADR-0004 stays blocked. Possible next tests, none started: an explicit `-c windows.sandbox=...` override, `codex exec review`, running Codex inside WSL, or updating Codex from 0.167.1 to the current 0.170.0. Each is another paid run and some change machine state, so each needs owner approval.
