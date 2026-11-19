# Persistent memory, instructions and continuity

## Capture contract (R02–R07)

Project consent is required before SessionKeep captures any conversation. Discover root and agent session from supported hook metadata; record project/worktree/session/source identity, event timestamp and adapter version. Hook makes bounded redaction and atomic spool write, then returns successfully regardless of downstream service health. If project disabled, don't capture. For supported transcripts, reconcile incremental offsets on Stop, at startup/import and through idle scanner. Transcript access is per project and opt-in; no scanning unrelated home-directory secrets. Do not rely on SessionEnd or async hook completion as a guaranteed durability boundary.

| Evidence | Default conversation mode |
|---|---|
| User conversation and assistant responses | Original permitted text with redaction annotations and ordered source IDs |
| Tool calls/results | Metadata and meaningful approved snippets; selected errors, changed paths, commands after secret filtering |
| Images/PDFs/deliverables | References and metadata by default; actual content explicit opt-in |
| Source tree | Graph and metadata separately; don't duplicate full code on every read |
| Native agent logs | Read only supported files; external data remains owned by agent |
| Credentials / private-marked content | Excluded before SessionKeep persistence |

Capture policy modes off/metadata/conversation/extended, preview and pause. Mark any missing assistant text, unsupported event type, incomplete transcript, dropped large output or redacted span; show capture health. In extended mode bounded evidence size and retention consent explicitly configured. Redaction and consent are stronger than completeness. Never claim permanent storage of every unfiltered token.

## Instruction handling

1. Explicit `remember` command, UI pin, or clearly requested durable instruction produces candidate with original evidence.
2. Confirm scope (global, workspace/project, branch, task or session), persistence and exact text. Default ambiguous extracted instruction is a suggestion. User-confirmed instructions remain until superseded or deleted; no TTL.
3. Deterministic scope resolution: host/system safety + current explicit user intent + active approved scoped instruction policy. Global rules apply unless an authorized narrower override is allowed; contradictions surface with both versions and require resolution, not arbitrary scope last-write-wins. Historical text is evidence, never new authority.
4. Session startup and prompt-aware retrieval includes applicable instruction versions independent of semantic rank; export IDs and digest. If too large for host, retrieve in chunks and report coverage; high-impact jobs cannot be labeled fully verified with missing mandatory requirements.
5. Changes use append/version history and audit. Old instructions clearly superseded; preserve historical reasons, not injected as active. If database unavailable, use atomic sanitized cached instructions only if scope matches, label cached state and expose timestamp.

## Retrieval design

Query context includes project ID, task, branch/worktree, allowed cross-project flag, query, evidence depth. Hard authorization and scope filter before candidate ranking; search original messages, confirmed facts, decision history and graph separately. Prioritize exact identifiers and known requirement IDs. Combine FTS ranks, recency, task links, active validity and optional semantic rank via reciprocal rank fusion or evaluated alternative. Confidence not a substitute for relevance. Do not prefer recency over a stable active instruction. Contradictions included intentionally; superseded evidence excluded by default except historical queries. Evaluate on actual tasks before tuning ranking weights.

Return compact index (`id, kind, title, snippet, scope, time, validity, evidence_count`) with pagination, full detail and original evidence range available via separate tool. Query exact memory IDs returns full permitted record; dedup by evidence ID. Context bundle includes required instruction list, matched decision IDs, unresolved conflicts, recent checkpoint, research freshness status, candidate IDs, omitted counts and reason. Use dynamic host-aware length threshold, no fixed budget that silently cuts instructions. Hook-injected envelope small enough for verified host behavior; direct agent tool retrieves remainder. Tagged archive/research sections untrusted. No automatically executing embedded tool directives.

## Consolidation and enrichment

Baseline works without any model: capture/FTS, explicit user notes, Git metadata, checkpoints. Optional rule extraction for known tool events. AI summarization batched at task/turn/idle checkpoints only with provider opt-in. Record input IDs, model, prompt version and output; validate schema; split proposed facts from owner-confirmed decisions. Never replace original evidence with a lossy summary. Dedup derived records while preserving evidence links; contradiction detector proposes alternatives, not silent edits. Embeddings optional; on unsupported model or high CPU, fallback keyword and report availability. Store dimensions and model version; local-only computation if selected.

## Recovery and import

Project identity is stable beyond directory path; map clones/worktrees with user approval. Record last task, branch/ref, dirty-state digest, original requirements, actions, blockers, tests and next steps. Restore with warning if current Git state differs. Provide explicit `resume` context pack; native session resume only through supported versioned agent adapter, fall back to new session with evidence. Import older transcript formats in dry-run preview; confirm project mappings and capture policy; idempotent, progress/cancel, no overwriting current instruction versions. Claude-Mem import later after schema/license review, optional migration only. Do not alter native session files in core milestone.

## User-visible controls

Search exact phrase/ID/date/session; filter by agent/project/kind/validity; source viewer highlights evidence; pin, correct, archive, delete, export, privacy pause. Show last captured and reconciled times, pending jobs, coverage gaps, retained data size and retention estimate. Global instruction page shows scope, original text and version history. Memory corrections update search and dependent context, not only UI.

## Verification

Use synthetic multi-month fixture with exact 20-item instructions, misleading recent summaries, changes, two projects and worktrees, repeated prompt, partial transcript and worker-off gap. Test exact recovery, mandatory scope/override behavior, cross-project isolation, redaction, supersession, deleted item absence, offline fallback, pagination, task resume. Critical confirmed instructions must have 100% deterministic test coverage in fixture; measure open-ended retrieval recall separately. Failed lookup must explicitly report missing knowledge, never invent memory.
