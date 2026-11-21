# Decision register

Status: **required** = directly established user requirement; **provisional** = proposed baseline awaiting owner/Phase 0; **open** = unresolved. Use `templates/ADR.md` for changes. Do not overwrite prior decision history.

| ID | Decision | Status / reason |
|---|---|---|
| D01 | Quality, accurate research and long-term recall above token savings | Required |
| D02 | Local development, optional cloud storage only; public open source | Required |
| D03 | One-time setup; lightweight; no routine manual worker restart | Required |
| D04 | Include Graphify-style structure and dependency knowledge | Required |
| D05 | Independent Codex review inside Claude; professional UI | Required |
| D06 | Portable workspaces including memory, source and unfinished work | Required |
| D07 | Go core + SQLite/FTS5, browser UI, no always-loaded AI | Provisional; verify perf and packaging |
| D08 | Durable local spool, optional supervised daemon, fail-open hooks | Provisional; failure-mode spike |
| D09 | Preserve original permitted evidence, derivations reversible, no summaries-only | Provisional implementation of D01; privacy/retention consent mandatory |
| D10 | Default manual read-only review via official Codex CLI, App Server later | Provisional; validate installed CLI, sandbox and auth |
| D11 | Native deterministic graph for initial TS/JS support, parser adapters for more | Provisional; compare Graphify reuse vs own parser, license and cost |
| D12 | S3-compatible encrypted immutable checkpoint first, no live SQLite replication | Provisional; verify provider and backup/restore contract |
| D13 | Apache-2.0 preferred project license, pending owner selection and dependency audit | Provisional; no LICENSE generated yet |
| D14 | Opt-in external inference/sync, capture consent, no raw auth tokens | Provisional security baseline; mandatory for release |
| D15 | Windows-first alpha; test macOS/Linux before claiming support | Provisional release sequencing |
| D16 | Product name DevIntel only working name | Open; package/ trademark search needed |
| D17 | Exact transcript portability vs semantic continuation | Open; test official resume formats; fallback required |
| D18 | Optional AI summarization and embeddings provider selection | Open; default no provider; benchmark recall and privacy |
| D19 | Cloud encryption format, key recovery, multi-device concurrency model | Open; prefer vetted backup tool; security review before release |
| D20 | Public alpha features, target hardware and performance budgets | Open; approve after Phase 0 |

## Phase 0 session 1 additions (2026-11-19) — all awaiting owner approval

Earlier rows are unchanged. Evidence: `evidence/phase-0/README.md`.

| ID | Decision | Status / reason |
|---|---|---|
| D07 | Refined by [ADR-0001](docs/adr/ADR-0001-core-stack.md): Go 1.27 + modernc.org/sqlite v1.62.0, version/FTS5 asserted at startup | Proposed; doc-level only, Go spike not run |
| D08 | Refined by [ADR-0002](docs/adr/ADR-0002-claude-hook-integration.md) and [ADR-0003](docs/adr/ADR-0003-process-model.md): exec-form hooks, documented hook fields as primary capture, 9,000-char envelope with coverage manifest, lease-based ingester | Proposed; live hook fixture not run |
| D10 | Refined by [ADR-0004](docs/adr/ADR-0004-codex-review-adapter.md): `codex exec` on an isolated snapshot; sandbox not trusted as the boundary | Proposed; flags verified on 0.167.1, no review run |
| D11 | Narrowed by [ADR-0005](docs/adr/ADR-0005-graph-parser-deferred.md): parser chosen by a Phase 4 spike; Graphify not embedded (Python) | Proposed |
| D16 | Name: 94 GitHub repos match "devintel"; folder is named "SessionKeep" (1 unrelated repo, npm name free) | Open; owner to choose, trademark search still needed |
| D21 | [ADR-0006](docs/adr/ADR-0006-phase-1-split.md): Phase 1 gated as 1A (capture→recall, CLI only) then 1B (UI, export, installer, MCP) | Proposed; scope sequencing needs owner approval |
| D22 | MCP server is a per-session resident process, optional, counted in resource reports; ARCHITECTURE §1 wording "short-lived" to be corrected on approval | Proposed (ADR-0003) |
| D23 | Transcript parsing is secondary reconciliation, never the primary capture path | Proposed (ADR-0002) |

## Phase 0 session 2 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D16 | Project name is **SessionKeep** (CLI `sessionkeep`, plugin namespace `sessionkeep`) | **Decided by owner 2026-11-19.** Trademark search still not done. Existing documents still say "DevIntel"; renaming them is the first task of Phase 1A-1 |
| D24 | Local Git repository initialised (`main`), no remote, no commits yet | Approved by owner 2026-11-19 |
| D25 | Go 1.27.4 toolchain kept project-local in `.tools/` (git-ignored), system PATH unchanged | Approved by owner 2026-11-19 |
| D07 | ADR-0001 runtime evidence obtained on windows/amd64 (SQLite 3.54.1, FTS5, WAL, backup, cross-builds) | Proposed; ready for owner approval |
| D08 | ADR-0002/0003 runtime evidence obtained (live hooks in print mode, 14 ms p95 hook, exactly-once replay) | Proposed; ready for owner approval with listed limits |
| D10 | ADR-0004: real Codex run could not read the target repository in its Windows sandbox | Proposed, **blocked**; needs a second approved test |
| D26 | Backups run on the writer connection or with ingest paused, with a deadline (finding B1) | Proposed (ADR-0001 update) |

## Session 3 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D07 | ADR-0001 core stack | **Accepted by owner 2026-11-19** |
| D08, D22, D23 | ADR-0002 hook contract and ADR-0003 process model, including manual-mode ingest only by explicit drain | **Accepted by owner 2026-11-19** |
| D21 | ADR-0006 Phase 1A / 1B split; plan is `phases/PHASE_1_PLAN.md` | **Accepted by owner 2026-11-19** |
| D10 | ADR-0004 Codex adapter: second real run also produced no working review | Proposed, **blocked**; further tests need approval |
| D27 | Go module path is `sessionkeep` (no hosting location chosen yet) | Provisional; change to the real repository path before any public release |
| D28 | First local commit of documents, evidence and the 1A-1 scaffold | Approved by owner 2026-11-19 |

## Session 4 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D29 | [ADR-0007](docs/adr/ADR-0007-unregistered-folder-modes.md): unregistered folders are handled by one setting — `manual`, `ask` (shipped default) or `auto` (owner-enabled, explicit level, exclusion list). `project add` has no default capture level | **Accepted by owner 2026-11-19.** Narrows "never auto-enable global capture" to "never by SessionKeep itself" |
| D30 | Onboarding is CLI-only in Phase 1A (`sessionkeep setup`); the web onboarding screen is part of Phase 1B and writes the same settings | Recorded; follows ADR-0006 |
| D31 | Hook-payload field allowlist moves from slice 1A-3 to 1A-4, where payloads are first parsed | Internal sequencing change, recorded |
| D32 | Hook deadline 2 s instead of 60 ms; spool events keyed by generated id; strict field allowlist; `systemMessage` reminder in ask mode (ADR-0002 update) | Internal, reversible; recorded with measurements in `evidence/phase-1/1A-4.md` |
| D33 | Redaction rule set gained command-line credential rules after a test caught `curl -u user:pass` reaching the spool | Done in slice 1A-4 |

## Session 5 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D34 | Migration `0001_foundation` was edited in place: `sessions` is unique per (project, agent, device, external session) so one host session can span projects | Done before any database existed outside test sandboxes. **From slice 1A-6 on (first run against real data) applied migrations are never edited; changes go in new numbered files** |
| D35 | Atomic file replacement on Windows uses a POSIX-semantics rename (needs Windows 10 1709+), classic rename as fallback; files replaced this way are read through `fsx.ReadFile` | Internal; forced by a failing test (`evidence/phase-1/1A-5.md`) |
| D36 | Only exclusions discard already-spooled events. Pausing a project or setting it to `off` stops new capture, but events spooled before that are still ingested | Provisional; the owner may prefer pause to discard as well |
| D37 | `sessionkeep status` (read-only counts and spool backlog) and `sessionkeep ingest` (one drain) added | Internal |

## Session 6 additions (2026-11-19)

| ID | Decision | Status / reason |
|---|---|---|
| D36 | Pause and `off` keep already-spooled events; only exclusions discard them | **Confirmed by owner 2026-11-19** ("keep pause as is") |
| D38 | Without an engine, drains are started by asynchronous plugin hook entries after `SessionStart` and `Stop` (plus `sessionkeep ingest` by hand). Extends the manual-mode answer the owner accepted for ADR-0003 | Implemented and observed live; **owner to confirm**, since the earlier approval said drains happen only when the user or a skill runs one |
| D39 | The plugin directory is generated per installation (`sessionkeep plugin write DIR`) and embeds the binary's absolute path; SessionKeep does not edit any Claude Code setting | Internal; follows ADR-0002 |

## Session 7 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D38 | Background drains started by plugin hooks | Owner did not object when asked and said to continue; treated as accepted, can be reverted on request |
| D40 | Recall is independent of capture: a paused or `off` project still receives its recorded instructions; excluded and unregistered folders receive nothing | Provisional; owner may prefer pause to stop recall too |
| D41 | Instruction scopes built now: global and project. Branch/task/session scopes and contradiction detection are deferred; identical active text in one scope is refused | Scope reduction inside slice 1A-7, recorded so it is not mistaken for done |
| D42 | `remember` has no default scope and saves nothing without `--yes`; text that looks like a secret is refused, not redacted; one instruction is at most 4,000 characters | Internal; follows the no-default rule of ADR-0007 |
| D43 | Every atomically written file has a second copy (`.alt`) that readers fall back to, because a file being replaced was measured to look missing for up to 65 ms on the reference machine | Internal; forced by an intermittent test failure (`evidence/phase-1/1A-7.md`) |

## Session 8 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D44 | Search always needs exactly one scope: the current (or named) project, or the explicit `--all-projects` flag. Reading a record by id is subject to the same check | Implements R05 / spec "cross-project ID requires scope check" |
| D45 | Queries are "all these words", each word quoted so punctuation and operator words are never interpreted; if nothing matches, the query is retried as one literal string | Internal; chosen for exact recall of errors, paths and ids |
| D46 | Command output uses plain ASCII for its own text (match markers `**`, `...`, `-`) | Internal; Windows console showed garbage for typographic characters |
| D47 | The generated plugin carries two skills, `recall` and `remember`; `remember` instructs Claude to preview, ask the user, and only then save | Not yet exercised live |

## Session 9 additions (2026-11-20)

Owner instruction: "ok lets finish rest phase 1", given in reply to a list of seven defaults. Treated as acceptance of those defaults:

| ID | Decision | Status / reason |
|---|---|---|
| D48 | Real installation into the owner's Claude Code is done by the owner; the agent builds the installer and tests it in a sandbox profile only | Owner default 1 |
| D49 | Up to ten short live Claude Code sessions (sandbox data) are pre-approved for the rest of Phase 1 | Owner default 2; two used in 1A-9 |
| D50 | npm packages for the web UI (React, Vite, Tailwind) may be downloaded at build time, pinned and recorded with licences | Owner default 3 |
| D51 | The dashboard is verified with API and accessibility tests without a browser; the owner looks at the pages. No browser automation | Owner default 4 |
| D52 | Auto-start (per-user scheduled task) is built and tested by creating and removing the task on this machine, leaving it removed | Owner default 5 |
| D53 | A C compiler may be placed in the git-ignored `.tools` folder so the race detector can run | Owner default 6 |
| D40 | A paused project still receives its instructions | Confirmed (owner default 7) |
| D54 | If a slice meets something outside these answers, record it, take the safer option, and continue with the rest | Agreed way of working for this run |
| D55 | Checkpoints are a note plus Git state (branch, commit, fingerprint of uncommitted and untracked content); no task or plan objects yet | Scope of slice 1A-9 |

## Session 10 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D56 | MCP server uses the official Go SDK v1.8.0, is read-only (four tools), optional (`plugin write --with-mcp`), and answers only for the session's project unless the owner starts it with `--all-projects` | Implements ADR-0003 item 4; a model must not be able to widen its own scope |
| D57 | `--` ends option parsing on the command line; all model-supplied text is passed after it | Closes an option-injection path found while designing the MCP tools |
| D58 | Full-text index rebuilt with word stemming (migration 0004); when no record has all the words, a labelled partial-match fallback is returned | Forced by a live session in which a correctly stored fact was not found (`evidence/phase-1/1B-1.md`) |
| D59 | Plan tools `instructions_get` and `checkpoint_get` are merged into `resume_pack` | Internal simplification |

## Session 11 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D60 | [ADR-0008](docs/adr/ADR-0008-dashboard-without-a-build-step.md): the Phase 1 dashboard is three plain embedded files with no npm packages or build step; the API only relays `sessionkeep` commands; loopback bind, one-time sign-in link, Host/Origin checks, request token | **Proposed, built, awaiting owner.** Departs from the React/Vite/Tailwind candidates the owner had approved downloading; chosen as the safer option under D54 because the UI could not be browser-verified in this run |
| D61 | `sessionkeep dashboard` never opens a browser by itself | Internal; follows the owner's no-browser-automation rule |

## Session 12 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D62 | Export is one unencrypted zip (database copy, settings, manifest with digests); import verifies everything and restores only into a data directory with no database; no merge | Phase 1 scope; encryption and cross-device restore are Phase 7 |
| D63 | Deletion (`forget`) previews, then deletes, drops matching spooled events, rebuilds the search index and rewrites the database file; an audit row keeps counts only | Implements the deletion rules in `docs/01-DATA_MODEL.md` §5 |
| D64 | The database copy for export is taken with `VACUUM INTO` on the writer connection under the lease | Follows finding B1 (a separate-connection backup restarts under write load) |

## Session 13 additions (2026-11-20)

| ID | Decision | Status / reason |
|---|---|---|
| D65 | `sessionkeep install` copies the program to a per-user folder and generates the plugin plus a one-plugin local marketplace; it prints the two `claude plugin` commands and edits no Claude Code setting. `uninstall` removes only recorded files and keeps data unless `--purge-data --yes` | Follows D48 and the "do not mutate global Claude configuration" rule |
| D66 | Auto-start uses the current user's "Run at logon" registry list, not a scheduled task | **Changed from the agreed default 5**: Windows refused to create a logon-triggered task without administrator rights. Same effect, no elevation; owner to note |
| D67 | Auto-start is opt-in and only stores waiting events once at logon; there is still no resident process | Follows ADR-0003 |

## Session 14 additions (2026-11-20) — Phase 1 acceptance run

| ID | Decision | Status / reason |
|---|---|---|
| D68 | The latest checkpoint is delivered at SessionStart together with the instructions, worded as a record of where work stood | Implements the spec's "new session receives the last task checkpoint"; forced by the demonstration (`evidence/phase-1/ACCEPTANCE.md` §5.4) |
| D69 | The drain recovers a complete event from a spool temp file older than five hook time limits, and records a gap when it discards an incomplete one after an hour | Closes a silent-loss path found under disk load (§5.5). The hook's 2 s limit is unchanged |
| D70 | Record lookup by id uses an index range; query plans of hot statements are checked by a test | Performance defect found at 100k records (§5.3) |
| D71 | A C compiler (w64devkit 2.11.0, checksum verified) lives in the git-ignored `.tools/cc`; the gate runs the race detector when it is present | Owner default 6 |
| D72 | Phase 1 gate status as the agent sees it: **not met by the plan's wording** (several test rows partial, transcript tests not run); no known unresolved data-loss or cross-project defect | **Owner decision required** |

## Session 15 additions (2026-11-20) — redesign, Phase 2 part A, first install

| ID | Decision | Status / reason |
|---|---|---|
| D73 | The dashboard is redesigned (loom identity: navigation rail with a woven band, state "strands", choice tiles, reading serif for recorded words). ADR-0008 stands as amended: still no build step, React or npm; a fourth embedded file (`favicon.svg`) is allowed | Owner rejected the plain pages on 2026-11-20 and asked for a premium, non-generic look. **Owner to look at the result** |
| D74 | The agent took five headless-browser screenshots of the redesign against synthetic data | The owner's standing rule forbids automatic browser automation. The agent judged the 2026-11-20 instruction ("decide whatever would be best... finish everything") to cover looking at a visual redesign it could not otherwise judge. **Agent's interpretation; owner may overrule, and the rule stands for all later work** |
| D75 | Sessions can be listed and read in full (`sessionkeep session list|show`, History page); read-only, project scope enforced before the lookup | P2-05 |
| D76 | Event and gap times are stored with nine fractional digits (`capture.TimeFormat`) so text order equals time order | Defect found by a test; no real database existed yet, so no data migration |
| D77 | History import (`sessionkeep history import`): preview unless `--yes`; registered projects at conversation/extended only; never registers a folder; honours exclusions and pause; skips sessions captured live; same redaction and size limit as live capture; idempotent; imports only typed and answered text | P2-01. Parser version 1, learned from key names only |
| D78 | Phase 2 work started on the owner's instruction of 2026-11-20 ("move to phase 2"). The agent reads this as acceptance of the Phase 1 gate for a private pilot with the partial rows in `evidence/phase-1/ACCEPTANCE.md` still open | **Agent's interpretation of D72; owner to confirm** |
| D79 | Not started: fact versioning (P2-02), summaries (P2-03), embeddings (P2-04), claude-mem database import | Each needs an owner decision: a model download or paid inference, or a schema and licence review |
| D80 | Version 0.1.0 is installed in the per-user programs folder by `sessionkeep install`, run by the agent at the owner's request. Enabling the plugin in Claude Code stays the owner's step; no auto-start entry was made | Keeps D48 and the rule against changing Claude Code configuration |
| D81 | Optional session summaries (P2-03) are written by running the user's own Claude Code program (`claude -p`) under their plan sign-in. SessionKeep holds no key and opens no connection. Off by default; `sessionkeep summarize on` explains and needs `--yes` | Owner asked for this on 2026-11-20 and supplied the support-article text saying `claude -p` draws on plan limits. Official local CLI authentication, as the project rules require |
| D82 | Summaries live in their own table (migration 0005) with model, prompt version and how many pieces the model saw. They are never evidence or instructions, are shown apart from original text, and are deleted with any text they were written from | Keeps "permitted original text" immutable and separate from model output |
| D83 | The summary call gets no tools, runs with every customisation off, starts in an empty folder, and SessionKeep's hooks ignore it. Default model `sonnet`; the owner can choose another (`--model opus`). Daily limit counts requests (default 20) | Session text is untrusted input; a summary call must not be captured or start another. Summarising is routine work, so the cheaper model is the default; the owner decides |
| D84 | Background summaries (`--auto`, one per drain, 30-minute pause after a failed attempt) are a separate opt-in | Not verified in a live session: the host may end a background hook before the request finishes |
| D85 | Embeddings (P2-04) stay unbuilt. Instead, summaries are searched, and the search output, the `recall` skill and the MCP tool tell Claude to retry with other words | Anthropic offers no embedding model; a local model needs a download and an ADR. Revisit after real use |
| D86 | Summaries are automatic when on (supersedes the opt-in of D84). `--manual` limits them to `summarize run`. The background drain starts a detached process that writes up to five; it makes no request itself | Owner, 2026-11-20: "everything needs to be automatic, behind the scenes". Claude Code ends background hooks when a `-p` session finishes, so the work is detached. Verified in one real session |
| D87 | Three real requests were made against synthetic text, under the owner's plan sign-in | Owner agreed on 2026-11-20 after being asked for about five. They exposed a defect no unit test had caught (a program path with a space) |
| D88 | Summaries are turned on (Sonnet, 20 requests a day) in the owner's installed copy | Owner, 2026-11-20: using the subscription is fine and it must work without a separate command. Sonnet is the agent's choice; Opus is one click on the Overview page |
| D89 | Voyage AI embeddings are not used, although the owner offered a key | It would send all stored text to a second company and add a stored secret and a paid service, against the local-first rule, before real use has shown that summaries are not enough. Revisit with recall numbers from real use |
| D90 | SessionKeep knows about several Claude Code accounts on one computer. An account is a configuration folder (`~/.claude`, or any `~/.claude*` folder holding `projects`); the short command that starts it is found by looking for a small script on PATH that sets `CLAUDE_CONFIG_DIR` to it. `sessionkeep account list / use / auto`. The chosen account decides whose plan pays for summaries, where earlier sessions are imported from, and which command SessionKeep prints. With one account nothing is asked and `claude` is used. With several and no choice, a session's own account is used, and `summarize on` refuses until one is chosen | Owner, 2026-11-20: two accounts on this computer (the default one, not in use, and a second one started with its own short command, in use); every `claude ...` command must use that command instead, and other users must be able to choose. SessionKeep runs the real program with `CLAUDE_CONFIG_DIR` set rather than the user's script: same effect, and a script is not something to execute on trust. It reads no sign-in file |
| D91 | The owner's installed copy is set to the owner's second (active) account | Owner's instruction, 2026-11-20 |
| D92 | History import stores one session file per transaction (`store.IngestEvents`, all or none) instead of one transaction per piece of text | Measured: about 39 s to about 13 s for 12,800 synthetic pieces. Live capture keeps one transaction per event |
| D93 | Redaction skips an expression when a plain-string "need" shows it cannot match. Results must be identical to running every expression; a test and a fuzz target compare the two | Measured: 0.4 ms to 0.011 ms per 700-byte text; this cost was paid by every captured prompt and answer. A wrong need would leak a secret, hence the equivalence checks (`evidence/phase-2/P2-C.md`) |

## Session 17 additions (2026-11-20) — Phase 2 accepted, Phase 3 part A

| ID | Decision | Status / reason |
|---|---|---|
| D94 | **Phase 2 is accepted by the owner** ("accept Phase 2, lets move to next", 2026-11-20). Phase 3 (current research) is started. Unbuilt Phase 2 items stay open and are not claimed: fact versioning (P2-02), embeddings (P2-04), claude-mem database import | Owner's words. The gate rule in `phases/LATER_PHASES.md` is met for Phase 3 |
| D95 | A researched claim is stored as verified only when SessionKeep itself fetched the named page and found the quoted words on it. The database refuses a verified claim without such a quote (triggers in migration 0006). "Stale" is never stored: it is worked out from the re-check date each time a claim is read | "No stale false verification" (Phase 3 rule). A model's memory, a search snippet or a summary cannot verify anything |
| D96 | The text of a fetched page is not kept. Stored: address, final address, date, status, ETag / Last-Modified, SHA-256 digest, size, title, and the quote (at most 600 characters) | Third-party pages are not SessionKeep's to republish (docs/03 section 6). Consequence: an old and a new version of a page cannot be compared side by side yet |
| D97 | Fetching is off until `sessionkeep research on --yes`, which first prints what is sent. A page is fetched only when a command names it or a recorded claim is re-checked by a command; nothing runs in the background or on a schedule. Only https; only public addresses, checked again at the moment of connecting (so a name resolving to a private address and a redirect to one are refused); no proxy, cookies or credentials; 2 MB, 5 redirects, 25 s. `research.allow_local` in the settings file lists local addresses the owner intentionally allows | New network capability: off by default under the rule on network disclosure. No proxy support is a known limit for networks that require one |
| D98 | `robots.txt` is not consulted | SessionKeep fetches single pages a person or their assistant named, like a browser; it does not crawl. A re-check run fetches at most 20 pages, one second apart per site. **Agent's judgement; owner may overrule** |
| D99 | HTML is turned into text by a small tag remover in `internal/research`, not by a new dependency | Keeps the dependency list unchanged; the text is only ever shown as plain text and matched against quotes. Limits: pages not in UTF-8 are read wrongly; pages built by JavaScript yield little text |
| D100 | Re-check windows are the eight categories of docs/03 section 3 (security and price 1 day ... concept 180 days), fixed in code | The owner can ask for them to be configurable; no evidence yet that the defaults are wrong |
| D101 | When the server answers "not modified" to a conditional request, the claim counts as verified again from that day | The page that held the quote is unchanged. docs/03 allows this by policy; it does not prove the page is right |
| D102 | A fourth skill, `research`, is generated with the plugin. It tells Claude to look at recorded claims before relying on a fact that goes out of date, to treat STALE and DISPUTED as not current, to quote exactly, and not to turn fetching on by itself | Without it the commands would only be used when the owner types them |
| D103 | Fetching is turned on in the owner's installed copy | Owner, 2026-11-20: everything must work without separate commands, and Phase 3 was accepted. **Agent's reading; `sessionkeep research off` undoes it** |
| D104 | One real page was fetched by the agent to prove the https path, which the tests cannot cover (they use a local server) | A read-only request for a public documentation page; recorded in `evidence/phase-3/P3-A.md`. **Agent's judgement under "ask before external side effects"; owner may overrule** |

## Session 18 additions (2026-11-20) — installing through npm

| ID | Decision | Status / reason |
|---|---|---|
| D105 | SessionKeep is delivered as an npm package: `npm install -g sessionkeep-VERSION.tgz` installs the command and sets everything up. The program stays the Go program; npm only carries it. Node is needed to install, never to run (the plugin's hooks call the installed program directly) | Owner, 2026-11-20: wants an npm/npx install that does everything behind the scenes. Rewriting in JavaScript was rejected: it would need a Node runtime at all times and discard the tested code |
| D106 | The package is built as a local file and is **not published**. `package.json` says `"private": true`, so `npm publish` refuses it. Publishing, the public name and the licence are a later owner decision | Owner, 2026-11-20: "build it locally first". Public distribution needs explicit approval (CLAUDE.md) |
| D107 | **D48 and D65 are changed by the owner**: `sessionkeep install --enable-plugin` turns the plugin on by running Claude Code's own two commands (`plugin marketplace add`, `plugin install`) for the account SessionKeep is set to. Without the flag nothing changes: the commands are only printed. SessionKeep still writes no Claude Code settings file itself, and a failure of that step leaves the installed program in place and prints the commands | Owner, 2026-11-20, accepting the agent's recommendation that one command does everything. Both commands were tried twice in a row against a throwaway configuration folder with Claude Code 2.1.329: the second run says "already" and exits 0 |
| D108 | After a global install the package runs `sessionkeep install --enable-plugin` by itself (npm "postinstall"). It never fails the npm install; if setup cannot finish, the reason is kept and the next `sessionkeep` command shows it. It does nothing for a non-global install, with `SESSIONKEEP_SKIP_SETUP` set, or with `--ignore-scripts` | "Everything behind the scenes" (D105). npm 11.19 prints a notice that install scripts will need to be allowed in future; if a later npm blocks the script, `sessionkeep` says "not set up yet" and names the command |
| D109 | One package holds the programs in `vendor/OS-CPU/` (this computer's only by default; `all` builds six). The usual layout, one optional package per system, is not used yet | Per-system packages that are not published would be looked up by name on the public registry, where anyone could own that name. Revisit when publishing (D106) |
| D110 | The `sessionkeep` command from npm runs the installed copy when it is the same version, and the packaged copy otherwise and for `install` / `uninstall` | Keeps npm's own files unlocked on Windows, so an update works while the dashboard is running |
| D111 | The plugin can be turned on and off per Claude Code account from the dashboard (Overview, "Claude Code accounts") and with `sessionkeep account plugins | enable NAME | disable NAME`. Whether it is on is asked of Claude Code itself (`plugin list --json` for that account), never read from its files; when Claude Code cannot be asked the state is shown as unknown, not guessed. Turning it off runs `plugin uninstall`; saved data stays | Owner, 2026-11-21: wants turning accounts on to be easy, from the dashboard. Extends D107: the button is the owner's request, and the change is still made only by Claude Code's own commands. All accounts share one memory, which is keyed by project folder; which account pays for summaries remains a separate choice (D91) |
| D112 | A session is told at its start which researched facts of the project are past their re-check date or disputed: counts, at most five named by id and subject, and the commands to read and re-check them. The note is built in the hook from the per-project cache file (now also holding id, subject, state and re-check date of up to 200 sourced claims) and the clock. No page is fetched, the database is not opened, and the fact's wording is not repeated. It is given at SessionStart only, not with later prompts | P3-06. A stale fact must not be relied on silently, but the hook path must stay fast and offline (ADR-0002). Staleness is worked out when the session starts, so a fact that went stale after the cache was written is still named. Not repeating the statement keeps a stale fact from reading as a current one |
| D113 | `--applies-to` is compared with the project's installed version only when written as `PACKAGE@VERSION` or `PACKAGE VERSION`. The installed version is read from lock files in the project's top folder: `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `go.mod`, `Cargo.lock`, `uv.lock`, `poetry.lock`, `requirements.txt` (exact pins). The comparison is made each time a claim is shown, never stored. Outcomes: same, another major version (labelled NOT ABOUT THIS PROJECT'S VERSION), another minor/patch version, several versions, not found. Free text ("the Pro plan") is shown as before and not compared | P3-03. Agent's choice of form and of files; reversible. Comparing at read time means the label follows the project when it upgrades. Not read: lock files in sub-folders (monorepo packages), `composer.lock`, `Gemfile.lock`, Maven/Gradle/NuGet files, version ranges. "Not found" is said, never guessed |
| D114 | The dashboard has a Research page: per project, each fact with status, dates, statement, version label, source address and quote, and two buttons, "Check the page again" (the same as `sessionkeep research check ID`: one fetch, refused while fetching is off) and "Withdraw". Recording a fact stays with the research skill and the terminal | P3-06. The check button fetches a page only on the owner's click, within the consent already given by `research on` (D96/D103). Everything is rendered as text; a source address becomes a link only when it starts with https |
| D115 | The dashboard is rebuilt to the design the owner chose ("Margin", made by the owner in Claude Design from `design/DESIGN_PROMPT.md`): a sidebar grouped by task, one project chooser that applies to every page, lists that open in a pane beside them (History, Instructions, Research, Projects), a "Needs attention" list first on the Overview, a Settings page, a delete confirmation in a dialog. Same server routes; no route was added or changed. Differences from the design, all because the data or a rule is missing: the theme switch lasts until the page is reloaded (browser storage stays forbidden, so the system setting decides at load); "Files mentioned" is found by a pattern in the text; a search result opens "the whole piece" and a link to its session, not the turns around it; settings keep their Save buttons; Health has no repair button. A checkpoint is written in three boxes and stored as one note with `Done:`, `Next:`, `Blocking:` lines; older notes are shown whole | Owner's choice of design (D73 answered). Reversible: three files in `internal/web/ui`. The three-line note needs no schema change and stays readable in the terminal and for Claude |
| D116 | History lists every saved session of the chosen project (or of all projects), read 200 at a time through a new `--offset` on `session list` and `offset` on `GET /api/sessions`. What SessionKeep saved is removed only by the owner's own delete commands (`forget`, shown first and confirmed): not when Claude Code clears a conversation (`/clear`, also typed `/reset` or `/new`), and not when Claude Code's own session files are deleted | Owner's request after losing sessions from Claude Code's own list. The second half was already how the store worked; it is now held by a test (`TestSavedSessionsOutliveAClearAndTheLossOfClaudeCodesFiles`). SessionKeep cannot put a session back into Claude Code's resume list: it keeps what was said, not Claude Code's session file |
| D117 | The session start that follows a compaction also carries the session's last exchanges word for word (up to six prompts and replies, plus the request the session began with), after the instructions and the checkpoint and inside the same 9,000-unit limit. The hook keeps them in a small file per session under the cache directory (`recent/`), written only where the project saves text and only after secrets were removed; files go after 30 days and when the owner deletes the project's stored text | Owner approved the research report's list (2026-11-21). After a compaction the model holds a model-written summary only, and the most reported complaint about Claude Code is work lost at that point. The hook cannot open the database and the newest exchanges are usually still in the spool, so a file is the only source. Not verified against a real compaction: the `compact` source is taken from the hook documentation, and a compaction that changed the session id would simply give nothing back |
| D118 | When a shell command fails with an error that an earlier session of the same project already met, the failed-command hook tells the session so: how often, when, the command then, and the assistant's reply at the end of that turn, quoted as a record and not as an instruction. Errors are matched by their text with numbers and spacing evened out; no model is used. Only shell tools (Bash, PowerShell), only errors of 30 characters or more beyond the exit code, once per error and session, never across projects. The ingester keeps the index per project under the cache directory (`errors/`), from stored text only; deleting stored text rebuilds it. Database change 0007 adds an index on event kind | Owner approved the research report's list (2026-11-21). Failed tool calls were already stored; recognising a repeat needs only a text match. The limits exist because a wrong "seen before" is worse than none: general tool messages repeat in every session and say nothing. The hook answer uses `hookSpecificOutput.additionalContext`, which the hook documentation lists for PostToolUseFailure (read 2026-11-21, partly through a summary); not yet seen working in a real session |
| D119 | `sessionkeep recap [today / yesterday / week / DATE]` and a Recap page say what was done in whole local days: per project the sessions (time, number of requests, failed commands, first request, last reply or the model-written summary where one exists, files edited), the checkpoints written and the commits made in that time on the current branch. It reads only, uses no model, and covers the current project unless `--all-projects` is given | Owner approved the research report's list (2026-11-21). A stand-up answer was the one daily use no tool found in the research offered. The scope default follows search: a command Claude can run must not hand another project's text into a session unasked. Commits are every non-merge commit reachable from HEAD in the span, whoever made them |
| D120 | A project can be handed to another developer in one file: `sessionkeep handoff make FILE` writes the project's own standing instructions and its checkpoints, and with `--with-sessions` the prompts and replies of its sessions; `sessionkeep handoff receive FILE` adds them to a project the receiver registered on their own computer, in whatever folder it lives there. Both show what would happen and need `--yes`; receive prints every instruction in full first. The file is plain JSON, not encrypted. On the way out the sender's project and home folders are shortened to `<project>` and `~` and the secret rules run again; they run once more on the way in. Left out: global instructions, tool input and output, failed commands, model-written summaries, researched facts, the code itself. Receiving the same file twice adds nothing; a file made on the same computer, or one with any malformed or unknown field, is refused whole. Sessions are taken only by a project that saves text | Owner approved the research report's list (2026-11-21), which named this as the first step toward team use (ROADMAP phase 9); it is one-way and by hand, not sync. Instructions from someone else become text Claude is told every session, so the receiver must see them before agreeing. Plain JSON lets both sides read exactly what travels. No encryption: a format of our own would need the security review docs/06 asks for. Not removed: names or paths typed in a conversation outside the two folders |
| D121 | `sessionkeep file PATH` lists the sessions that changed a file, each with the request that led to its first change and the reply after its last; `sessionkeep why COMMIT` and `sessionkeep why FILE:LINE` (through `git blame`) list the sessions that changed a commit's files in the 7 days before it, the one that changed most of them first. Both read only, use no model, and add nothing to Git. A file is matched by its path inside the project, so the folder may have moved | Owner approved the research report's list (2026-11-21). The file names of tool calls were already stored at conversation level; these commands only look them up. A commit is tied to a session by file and time, which the output says: nothing is written into commit messages, unlike the hosted tool the research found. Known limits: imported sessions carry no tool calls, renames are not followed, and a change typed by hand has no session |
| D122 | `sessionkeep suggest` and a "Things you repeat" button on the Instructions page list requests the owner made in nearly the same words in three or more sessions and that no instruction covers. Sentences are compared by their words (70% in common), only sentences with a word such as always, never, use or do not are considered, and nothing is saved: the button only fills the existing add-an-instruction form, which still previews and asks | Owner approved the research report's list (2026-11-21). No model, so no paid inference; the price is that a rewording with few shared words is missed and the page says the results must be read. An instruction is never created without the owner's own confirmation |

## Approval rules

Changes to scope, licensing, public distribution, cloud trust, provider data sharing, secret policy or security defaults require explicit owner approval. Reversible internal implementation choices may be approved in a phase plan; record reasons and tests. No claims of API availability, supported platform or measurements based solely on discussion.
