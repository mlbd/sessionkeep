# Product specification

## 1. Mission and principles

SessionKeep is an open-source developer-owned intelligence platform that works with existing AI coding agents. Developers keep their source code and execution on local machines, carry durable project knowledge across sessions and devices, research changing technical facts from current evidence, understand code relationships and request independent review from a different agent. It is not a cloud IDE, model provider, token-metering product, secret manager by default or substitute for tests, Git and human judgment.

Priorities in order: correctness and project/user instruction fidelity; durable cross-session evidence; current technical research; reliability and recovery; lightweight predictable local operation; cross-agent review and project understanding; convenient optional portability; secondary efficiency. Do not trade recall or correctness for lower token counts. Critical instructions are retrieved deterministically, not through vector search alone. Long history is navigable and original permitted evidence recoverable; if input exceeds host limits, use staged retrieval and visible coverage warnings.

## 2. Users and scenarios

- Solo developer: close a terminal and return weeks later; recover requirements, decisions, file context and next task without repeating everything.
- Multiple projects: switch directories and branches; retrieve only relevant authorized knowledge, cross-project reuse explicit.
- Research-driven engineer: verify current API version, pricing, limits or security requirements; preserve original source evidence and conflicting results for later revalidation.
- Independent reviewer: request Codex review of a Claude plan or exact code diff without manually switching terminals, with no write permission by default.
- Traveling developer: publish a recoverable encrypted workspace checkpoint from desktop, power it off, restore to laptop and work locally; transfer work back without destructive overwrites.
- Open-source adopter: install a signed binary and optional plugin without containers or mandatory cloud account; see and control data capture, resources, health and deletion.

## 3. Requirements and release priority

M = local core milestone; E = advanced local expansion; P = portability; L = later ecosystem. Phase numbers in ROADMAP are sequencing, not all public v1 guarantees.

| ID | Requirement | Milestone |
|---|---|---|
| R01 | One-time install; no per-session setup or worker prompts; optional OS-managed daemon; diagnostics and safe recovery | M |
| R02 | Bounded hooks, durable worker-independent capture, ordinary coding continues on failure | M |
| R03 | Authorized original user/assistant text, important tool evidence and session IDs survive restarts; coverage and redaction visible | M |
| R04 | Explicit permanent instructions versioned, scoped, supersedable and deterministically recovered | M |
| R05 | Project/worktree identity, branch awareness, isolation, explicit cross-project search | M |
| R06 | Search exact error, symbol, historical decision, date and session; recover source evidence | M |
| R07 | Task checkpoints, unfinished work and native-session/semantic continuity | M |
| R08 | Local dashboard, project switching, memory management, health, accessible UI, privacy controls | M |
| R09 | Current authoritative research capture with sources, claim evidence, version and verification dates | E |
| R10 | Risk-based freshness, explicit latest bypass, change/conflict detection, truthful unavailable status | E |
| R11 | Incremental deterministic code graph, dependency paths, code-to-requirement/research links and evidence | E |
| R12 | Optional semantic/hybrid recall, validated retrieval quality, no mandatory vector service | E |
| R13 | Invoke Codex from Claude, manual read-only plan/code review, persistent structured results & cancellation | E |
| R14 | Configurable agent roles, manual/checkpoint/strict checks; independent review, human adjudication and test evidence | E |
| R15 | Plans and original requirement traceability to changed code, commits, test results and review findings | E |
| R16 | Local export/import, encrypted versioned cloud checkpoints; bring your own provider | M/P |
| R17 | Restore committed/uncommitted selected files, knowledge, Git state, optional sessions and environment manifest | P |
| R18 | Manual and scheduled incremental sync, conflicts, device enrollment and recovery | P |
| R19 | No auth token copying; local-only default; capture consent, redaction, access control, deletion and export | M |
| R20 | Signed cross-platform releases, install/update/uninstall/rollback, docs, OSS licensing and measured compatibility | M |
| R21 | Optional team memory, additional agents and remote sync server; separate permissions | L |
| R22 | Profile resource use and responsiveness; defer costly work visibly, never silently drop essential evidence | M |

## 4. Product boundaries and defaults

- Default capture: after explicit project opt-in, retain original permitted user and assistant conversation text; record provenance and redact known secrets before writing to SessionKeep. Tool outputs selected by policy (no bulk code, media or complete terminal logs automatically). Per-project `off`, `metadata`, `conversation`, `extended` levels; show capture gaps. Existing external transcripts are outside SessionKeep's deletion guarantee.
- Core does not require AI summarization. Enrichment is asynchronous and optional; local embeddings and local models optional, downloaded only with consent. No forced authentication besides configured integrations.
- Research is triggered by task risk, not automatic broad crawling. Network unavailable = stale/unverified warning; no fabricated verified state. Users can add authoritative sources.
- Core UI uses existing browser. No Electron. No running terminal UI or remote shell by default. No secrets vault until separately designed; restore environment configuration excluding passwords by default.
- Public alpha may be Windows-first only; macOS/Linux support must be tested before advertising. Public stable release can be modular: local core and independently labeled graph/research/review/sync features. Do not claim full feature parity or superiority without evaluations.
- Integrate with existing Git and coding tools rather than replacing them. No automatic Git push, commit, publish or deployment without configured authorization.

## 5. Acceptance outcomes

1. A permitted instruction recorded on day 1 is recovered verbatim on day 90 from an isolated new session and remains correctly superseded when edited. Required instructions are not truncated silently.
2. With the worker stopped, a new session still runs, captures locally and can retrieve cached valid instructions or shows explicit degraded status. Replay is idempotent after restart.
3. A current API task detects stale claims and cannot be labeled verified without fetched appropriate version evidence. Offline behavior is explicit.
4. An index changes only affected files on update; query traces real edges to code locations and distinguishes inferred relationships.
5. A read-only independent review uses a known snapshot, stores original output, distinguishes missing checks, and cannot mark a task done without required test/requirement evidence.
6. Restore checkpoint on a second device with the first powered off; no auth tokens are copied, no conflicting data silently overwritten, and dependency gaps are shown.
7. Actual measured reliability, capture latency, resource use and recall results are published separately from target budgets. See `docs/11-TEST_EVAL_PLAN.md`.
