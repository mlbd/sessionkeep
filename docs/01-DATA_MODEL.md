# Data model and storage contract

**Status:** logical schema, to be translated to reviewed SQL migrations during implementation. Every project-owned row has `project_id` and references its parent under the same project with composite foreign keys or equivalent checked transactions. Global records have an explicit global scope; never interpret a missing project as global authorization. IDs use opaque UUIDs; time is UTC RFC3339; schema/payload versions are integers. Do not use filenames or clocks as identity.

## 1. Foundation and evidence

| Table | Fields (in addition to ID where applicable) | Constraints and indices |
|---|---|---|
| `projects` | uuid, name, created_at, archived_at, capture_policy | stable ID, name not unique |
| `project_roots` | project_id, device_id, normalized_path, repo_identity, worktree_id, last_seen | unique device/path; repo match is candidate, never automatic merge of unrelated clones |
| `sessions` | project_id, agent, external_session_id, device_id, worktree_id, branch, start/end_at, state, adapter_version | unique agent/device/external ID; project/date index |
| `transcript_cursors` | source_id, session_id, file_identity, offset, prefix_hash, last_read, format_version | unique source; handle rotation/truncation |
| `events` | project_id, session_id, event_key, kind, observed_at, received_at, payload_version, safe_payload, evidence_id, redaction_status | unique event_key (namespace by source), session/time; immutable permitted content |
| `blobs` | digest, size, media_type, storage_ref, created_at, redaction_version | digest scoped to workspace or local domain; reference tracking |
| `evidence` | project_id, origin, source_locator, source_revision, range_start/end, observed_at, blob_id, completeness, trust_level | original permitted text, stable source reference; immutable versions |
| `messages` | project_id, session_id, event_id, external_message_id, role, evidence_id, order_key | unique source message identity, chronological deterministic order |
| `capture_gaps` | source, session_id, reason, time, count, recoverable, resolved_at | explicit dropped/truncated/unsupported data |

Dedup raw events by host-provided ID where reliable, otherwise deterministic source key composed of exact adapter, transcript source identity, byte range/record identity and safe digest. Do not dedup intentional repeated user instructions just because text is identical. Use raw message IDs to reconcile hook/transcript duplicates, keep both provenance paths when they overlap. Partial transcript line is not a record until complete. Redaction occurs before durable spool/DB/logging; evidence consists of permitted original text, never a guarantee of unredacted retention.

## 2. Memory and project work

| Table | Main fields / relationships |
|---|---|
| `instructions` | stable instruction ID, scope_kind, scope_id, state, current_version_id, confirmation_status |
| `instruction_versions` | instruction_id, verbatim permitted text/evidence_id, effective_from, supersedes, author, status, reason, revision |
| `memories` | project_id, kind, title, status, current_version_id, importance, author_kind |
| `memory_versions` | memory_id, content, evidence links, created_at, supersedes, derivation_model, prompt_version, confidence band |
| `memory_evidence` | memory_version_id, evidence_id, relation (supports/contradicts/derived_from) |
| `tasks` | project_id, title, state, owner, parent_id, active_checkpoint_id, branch, updated_at |
| `checkpoints` | task_id, session_id, git_head, dirty_manifest_hash, next_steps, blockers, evidence_ids, created_at |
| `requirements` | task/project_id, original evidence_id, statement, scope, version, status |
| `plans`, `plan_versions` | task_id, version, assumptions, steps, requirement IDs, approved_by/at, content digest |
| `requirement_links` | requirement version, graph node/review/test/commit ref, relationship, evidence |
| `test_runs` | snapshot_id, command argv, cwd, exit status, started/ended_at, report evidence, environment, result_kind (actual/mocked/not_run) |

Instructions are distinct from candidate memories. Explicit owner confirmation needed for durable global rules; suggestions remain drafts. Supersession preserves prior versions, not last-write-wins. Enforce at most one active confirmed version for a logical instruction within its scope; conflicts remain visible until resolution. Task states: draft → planned → approved → in_progress → blocked/review → done or abandoned; transitions record actor/reason and referenced evidence. Done requires policy-specific evidence, not blanket AI approval.

## 3. Research & graph

| Table | Key attributes |
|---|---|
| `research_runs` | project, question, task, requested_freshness, status, adapter, started/ended, coverage |
| `sources` | normalized URL, authority classification, access policy, first/last_seen |
| `source_snapshots` | source_id, fetched_at, final_url, http_status, etag, last_modified, content_hash, evidence_id, extraction_version, accessability |
| `claims`, `claim_versions` | stable claim ID, subject/predicate/value or text, version constraints, applicability, verified_at, expiry, state, supersedes, provenance |
| `claim_evidence` | claim_version_id, snapshot_id, exact range, relation, evaluation |
| `freshness_policies` | category, TTL, risk, trigger, override version |
| `verification_runs`, `source_monitors` | claim/source, check method, due_at, observed result, next_check, backoff |
| `graph_files` | root/worktree, relative path, content hash, language, parse_status, generation, revision |
| `graph_nodes` | stable node key, project, kind, display name, file/symbol, byte range, source revision, provenance |
| `graph_edges` | src_id, dst_id, relation, evidence_id, extraction_kind, confidence, parser_version, generation |
| `graph_index_runs` | root/worktree, snapshot/ref, parser version, coverage, state, timestamps |

Research snapshots immutable as allowed by source rights; 304 response may confirm content unchanged but not independently establish claim applicability. Claim states: candidate, verified, stale, disputed, withdrawn, unavailable; evaluation record separate from fetch event. Graph edges source and destination scoped; external unresolved symbols represented as explicit unresolved nodes, not false references. Indices for both edge traversal directions and node type/file.

## 4. Reviews, jobs, system and sync

| Table | Key attributes |
|---|---|
| `agent_profiles` | agent/adapter, capability, binary path, permission preset, auth_mode reference, version, consent |
| `review_snapshots` | project, head/ref, changed file manifest digest, plan/requirement versions, included evidence IDs, exclusions |
| `review_runs` | snapshot, provider, requested type, sandbox, status, raw output evidence, schema_version, usage metadata, timestamps |
| `review_findings` | run, severity, location, claim, evidence, reproduction, confidence, status, adjudication evidence |
| `jobs` | kind, payload reference, state, idempotency_key, attempt, next_run, lease_owner/until, cancel_requested, last_error_code |
| `settings`, `audit_events` | typed config versions; security-relevant metadata, actor, action, outcome, no secrets |
| `devices`, `sync_operations` | device identity/public metadata, sequence, operation_id, entity ID, base revision, tombstone, encrypted transport ref |
| `snapshots`, `sync_conflicts` | manifest ID, parent, remote generation, status, object refs, verification; both competing versions and resolution |
| `usage_metrics` | operation, durations/counts, queue depth, optional provider usage, local-only aggregate |

Job state queued → leased/running → success / failed / canceled / retry_wait / needs_consent; terminal runs immutable except explicit retry creates next attempt. Commit domain side effects and state transition together when local; for external jobs persist request/response identity, ambiguous outcome requires reconciliation not automatic double-billing. Jobs must be idempotent at least at local result insertion.

## 5. Physical design and operations

- SQLite supports a single writer at a time. Enable foreign keys on each connection; use WAL on a local filesystem, patched embedded SQLite, bounded busy timeout and justified durability setting (`synchronous=FULL` baseline for acknowledged critical records). Verify actual library version via SQL and validate FTS5. Use migration version/checksum; exclusive upgrade lock with consistent backup; rollback release only if data compatible or restore explicit backup.
- FTS index allowed message text, memory titles/content, research claims and symbol names with scope re-check at join. Bind parameters; safely tokenize and handle punctuation (exact file path/hash/error index for identifiers). Rebuildable derived indexes never source of truth. If vector optional, store model/version/dimension/content hash; invalidation and re-embedding on changes. Do not share result caches across scope.
- Blob files written atomically before DB reference; garbage-collect only unreferenced eligible blobs after a grace period and backup awareness. Do not use plaintext temporary copies of secrets. Backup uses SQLite online backup or consistent supported export, integrity check and restore rehearsal, not live file copy.
- Deletion: transaction marks tombstone and removes live eligible content/indices/derived records; purge blobs and caches, encrypted remote history governed by retention; SQLite page remnants and existing external backups limit secure deletion claims. Document behavior, provide purge/compaction options; do not resurrect deleted items from old sync data. User understands native agent transcript deletion is separate.
- Test schema migrations from empty and every supported previous release, interruption, rollback, foreign-key violations, cross-project joins, FTS rebuild, unique replay, concurrent writer/reader, corrupt DB and restoration. Schema evolution adds backwards-readable contracts or explicit migration, never silently drops history.
