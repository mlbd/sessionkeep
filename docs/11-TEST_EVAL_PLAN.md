# Test, evaluation and acceptance plan

This is a proposed release qualification plan, not executed results. Tests must use reproducible fixtures, expected outputs and exact version/commit; actual external/OS tests recorded separately from simulations. Preserve evidence with no secrets. Failures block the affected phase or feature must remain disabled with documented limitations.

## 1. Traceable test matrix

| Test ID | Requirement | Scenario and assertion |
|---|---|---|
| T01 | R01,R20 | Fresh install under non-admin user, Windows path spaces/Unicode, offline core; one setup, no prompt on subsequent sessions |
| T02 | R01,R02 | 100 successive starts/ends, 20 concurrent sessions across projects, exactly one daemon instance, no duplicate jobs/windows |
| T03 | R02,R03 | Kill worker before capture and after durable spool write; agent continues, replay all committed events once |
| T04 | R02,R03 | Kill consumer before/after transaction, DB busy, disk full, corrupt item, hard terminal kill; no silent lost acknowledgments, recoverable gaps visible |
| T05 | R03,R04,R06 | Fake clock day 7/30/90 and real restart, new session without prior context; recover exact original 20 instructions and evidence including order/supersession |
| T06 | R04,R05,R19 | Two projects/worktrees and scoped global exceptions; hard deny leakage; conflicts surfaced; stale cache labeled |
| T07 | R03,R06 | Transcript append/partial record/truncation/rotation, repeated prompt and tool duplicates; exact permitted ordering, idempotency, gap accounting |
| T08 | R07 | Restore task checkpoint on changed Git branch and dirty state; warn and link requirements, no destructive reset |
| T09 | R08,R19 | UI auth, keyboard, dark/light, paginated search, capture pause, export and deletion; actual server data, no fake health |
| T10 | R09,R10 | Fake docs change version/price, explicit latest despite valid TTL, 304 and timestamp distinction; correct evidence and applicability |
| T11 | R09,R10,R19 | Offline/404/429/conflicting sources/SSRF redirects/prompt injection; no false verified claim or data disclosure |
| T12 | R11 | Gold graph edges vs AST, unresolved calls, aliases, source coordinates, valid project scope; per-edge precision/recall |
| T13 | R11,R22 | Change/delete/rename/branch/parse failure; incremental equals full rebuild at same snapshot, stale status and bounded resources |
| T14 | R12 | Hybrid vs exact/FTS recall, model mismatch/off, required instructions never dropped; measure quality first |
| T15 | R13,R19 | Mock adapter login/permission errors, malformed report, timeout, cancel, args with spaces; no auth token access or write escalation |
| T16 | R13,R14,R15 | Authorized real Codex read-only review of seeded plan/diff, snapshot digest and filesystem before/after; invalid/stale review can't approve |
| T17 | R14,R15 | Requirement links to code/test/findings, open high defect and missing tests gate completion, explicit override logged; no loops |
| T18 | R16,R19 | Consistent live DB backup, export/import, old version and duplicate replay, encrypted archive integrity and deletion effects |
| T19 | R17 | Desktop offline; restore synthetic repo on second device with staged/unstaged/untracked/delete, tasks/history; compare digests/coverage, warn missing runtimes |
| T20 | R18,R19 | Interrupted upload, missing object, wrong key, path traversal, CAS conflict, two writers and revoked credentials; no silent overwrite |
| T21 | R19,R20 | Secrets redaction fixtures, auth/CSRF/host rebinding, SSRF, project ID enumeration, fuzz JSON/import; no raw sensitive evidence or logs |
| T22 | R20 | Update signed artifact, bad signature, crash mid-migration, occupied binary, rollback and uninstall preserve data; platform matrix |
| T23 | R22 | Idle/active resource bounds, hook response included launch overhead, 100k memory corpus, 20 sessions; no quality sacrifice |
| T24 | R16,R18,R19 | Tombstone and restore from old snapshot, deletion preview, retention/backup limitation; no resurrection in sync-enabled mode |
| T25 | R21 | Later team isolation, key revocation, role permissions and cross-device concurrent edits; not a core release claim |

## 2. Evaluation corpora and measurement

Use synthetic repositories and session transcripts with randomized identifiers, multiple project scopes, misleading newer notes, exact error messages, evolving docs, explicit instruction corrections and old task histories. Record corpus generator seed and expected evidence IDs. Test simulated clocks for aging without waiting months; add real long-duration soak tests over time. Use manually annotated critical requirements and graph edges; independent evaluator judges open-ended recall and review findings blind to system identity where feasible. Prevent fixtures/evaluation answers from leaking into retriever. Same corpus, hardware, CLI/provider model/version, task and settings for comparison. Record capability differences and exclusions; don't rank products unsupported on a task as equivalent. Prioritize correctness and coverage over resource metrics; no token-savings promise.

| Metric | Proposed gate/measurement |
|---|---|
| Confirmed instruction exact recall | All fixture active required instructions recovered exactly; no superseded active injection |
| Project isolation | Zero unauthorized rows/evidence in adversarial fixture |
| Durable capture | All acknowledged synthetic events recovered exactly once; report unacknowledged loss/gaps |
| Research freshness | No known stale/version-incompatible claim marked verified in controlled tests; valid provenance and dates |
| Graph correctness | Gold-set per-edge precision and recall; provisional deterministic extraction precision >=95%, thresholds by language after spike |
| Independent review | Precision/recall on known seeded defects and requirements; report false positives/coverage and reviewer variation; no guarantee detects everything |
| Idle engine | Target RSS <=50 MiB, CPU <1% of one logical core average 10 min; OS baseline stated |
| Hook | p95 <=100 ms normal capture from process spawn to exit, including fsync; report p50/p99 and degraded deadline |
| Startup recall | p95 <=500 ms cached scoped context; include cache state and host injection result |
| Search | p95 <=300 ms typical bounded FTS query at 100k permitted records, cold/warm separated |
| Multi-session | 20 active sessions; no duplicate engine and no cross-project contamination |
| Core install | target <=50 MiB uncompressed excluding optional assets and user data; measure both installed and download sizes |
| AI use | Record actual provider usage where available, otherwise unknown; secondary only |

Bench harness reports OS, CPU, RAM, disk type, build flags, dataset, repetitions, resource definitions, feature flags and raw samples. Resource cap may defer indexing but never falsely mark full coverage. Benchmark idle engine excluding OS/browser; also report browser and actively spawned integrations separately. Performance targets are not established measurements and may be revised explicitly if incompatible with durability/quality; record why.

## 3. Release gates

- Phase 0: verified contracts and decision approvals, actual prototype results or explicit blockers. Phase 1: T01–T09 subset for supported OS, T18 local only, T21/T23 relevant subset. No external inference required.
- Research/graph/review: corresponding T10–T17; explicit provider integration tests and consent, no unverified platform claims. Portability: T18–T20/T24 before enabling cloud features, verified restore rehearsal. Public stable: T22 and actual target platforms, security/license/name checks, support docs and signed artifacts. Later T25 separate.
- Unit/integration tests, fuzz tests around untrusted parser input, crash/failure injection, UI accessibility and user journey tests, dependency/license/security scans, release smoke tests. CI should record evidence artifacts and not run expensive agent calls on untrusted PRs. Run race detector where supported; note CGO/testing dependencies separate from installed core.
- Test evidence template: exact command, OS/runtime/provider version, seed, commit and snapshot, expected/actual outcome, stdout/stderr safe extract, measurements, timestamp, real/mocked/not run, unresolved failure and owner decision. Do not weaken tests to claim pass. Require independent review for high-risk architecture/security paths when authorized, then human adjudication.
