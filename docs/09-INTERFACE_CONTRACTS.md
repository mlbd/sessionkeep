# Proposed interface contracts

All names, wire formats and paths below are design inputs, not shipped interfaces. Version contracts and validate against actual upstream release before generating manifests. Avoid coupling internal APIs to Claude-specific fields. Errors have stable machine codes, safe messages and retryability; never leak sensitive payloads.

## CLI target surface

| Command | Purpose |
|---|---|
| `sessionkeep setup` | one-time interactive consent, environment probes, optional startup/plugin setup |
| `sessionkeep start / stop / status` | manage one per-user core, no startup prompt in hook |
| `sessionkeep doctor [--json]` | read-only diagnostics; `--repair` separate explicit consent |
| `sessionkeep hook <event>` | receive host JSON stdin; neutral bounded capture/context response |
| `sessionkeep mcp` | stdio transport (stdout protocol only) |
| `sessionkeep project add/list/pause` | scoped register/consent |
| `sessionkeep memory search/get` | query scoped knowledge, evidence pagination |
| `sessionkeep remember` | confirm persistent instruction version |
| `sessionkeep resume <project>` | continuity context pack, optional native resume later |
| `sessionkeep dashboard` | open authenticated local web UI |
| `sessionkeep jobs list/get/cancel` | observability, cancellation |
| `sessionkeep research verify` / `graph refresh` / `review` | optional feature operations |
| `sessionkeep backup/restore/sync/travel/devices/conflicts` | portability after feature available |

CLI machine output via `--json` envelopes and separate stderr diagnostics; interactive commands may render text. Exit statuses distinguish success, validation, failure, unavailable, needs-consent; hook path intentionally neutral exit 0 for recoverable problems, no normal code blocking. No shell-interpolated user strings. Global data dir override for test sandbox only and controlled env/config, never infer caller path as authorization. Do not register commands for unavailable feature without truthful capability response.

## Host hook adapter

| Event | Contract |
|---|---|
| SessionStart | resolve project, enqueue event, lightweight current instruction + checkpoint context; no startup daemon or research blocking |
| UserPromptSubmit | filter/capture prompt, deterministic instruction and relevant context/freshness notice; no external inference |
| PostToolUse / Failure | filtered metadata and selected evidence, scope, event IDs |
| Stop / StopFailure | capture turn endpoint and schedule transcript reconciliation/extraction; quality gate opt-in later |
| SessionEnd | best effort state; no reliance on last event, no long uploads |
| PreCompact / PostCompact or subagent/task hooks | add only after compatibility verified, capture checkpoint/parent scope |

Example normalized event (safe synthetic data):

```json
{"version":1,"event_id":"uuid","source_key":"claude:device:session:message-id","agent":"claude-code","adapter_version":"1","project_id":"uuid","session_id":"uuid","kind":"user_prompt","observed_at":"2026-11-19T12:00:00Z","received_at":"2026-11-19T12:00:01Z","payload":{"text":"Verify API behavior"},"redaction":{"applied":true,"count":0},"completeness":"complete"}
```

Input host fields validated from supported schemas, unknown fields ignored or sanitized as metadata. Output for supported context event is documented hook JSON, e.g. `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"SessionKeep: current project instructions ..."}}`. Verify character/size limits; retrieve remainder via MCP. If no context or timeout, neutral output; stdout never raw logs. `async` not sufficient for reliable capture at teardown; keep durable capture synchronous and fast; async expensive operations managed separately. Validate version-specific Windows command quoting and process spawn. Plugin manifests only generated after Phase 0. Hooks never supply automatic permission allow or overwrite user prompt.

## Local HTTP API `/api/v1` (initial then optional)

| Method/path | Input/output |
|---|---|
| `GET /health` | authenticated readiness/version/feature/gap metrics; public minimal endpoint only if justified |
| `GET /projects` / `POST /projects` | paginated scoped list / explicit create with consent |
| `GET /projects/{id}/sessions` | cursor/date filter |
| `GET /projects/{id}/memory` | q, type, validity, cursor, limit; result rows + coverage |
| `GET /projects/{id}/memory/{id}` | details/evidence refs, version, status |
| `GET /projects/{id}/evidence/{id}` | authorized content/range; no arbitrary paths |
| `POST /projects/{id}/instructions` | verbatim text, scope, confirmation; new version |
| `PATCH /projects/{id}/instructions/{id}` | expected revision, correction/supersession |
| `POST /projects/{id}/capture-policy` | preview and confirm pause/settings |
| `GET /projects/{id}/research/claims` / `POST .../research/runs` | status/claims, consented verification job |
| `GET /projects/{id}/graph/query` | bounded node/edge query, proof/coverage |
| `POST /projects/{id}/reviews` | snapshot, agent, permissions, consent/idempotency; job ID |
| `GET /jobs/{id}` / `POST /jobs/{id}/cancel` | state, progress, result ref or cancel |
| `GET /events` | scoped SSE, cursor/reconnect, authorized event metadata |
| `POST /projects/{id}/exports` / `DELETE /projects/{id}/memory/{id}` | export/deletion confirmation, scope/backup effect |
| `POST /projects/{id}/snapshots` / `POST /restore` | authorized checkpoint/restore preview+commit (later) |

DTO envelope `{data, next_cursor?, warnings:[], coverage?, request_id}`; error `{error:{code,message,retryable,details_safe},request_id}`. HTTP statuses 400 invalid, 401 unauthorized, 403 forbidden, 404 scoped absent, 409 version/conflict, 413 too large, 429 bounded, 503 unavailable. Cross-project ID requires scope check even with ID; do not trust route ownership alone. Pagination stable by sort key+id. Mutations use expected version and idempotency key where applicable. Limit body and response; destructive preview token scoped to exact payload and expires. Jobs events are sequenced, SSE does not expose raw evidence.

## MCP tools

Initial names (use host-compatible namespace): `memory_search`, `memory_get`, `evidence_get`, `instructions_get`, `task_checkpoint_get`, `project_status`. Later `research_search`, `research_fetch`, `research_verify`, `research_record`, `graph_neighbors`, `graph_impact`, `review_start`, `review_status`, `review_result`, `review_cancel`.

Each tool uses JSON Schema with explicit project ID or session-derived scope, bounded `limit`, cursor, allowed args, `additionalProperties:false` where appropriate. Search returns IDs/snippets/evidence refs/coverage, get returns original permitted content; report invalid scope/unknown facts. Mutating tools require user confirmation when appropriate and revision; no tool executes arbitrary SQL, opens arbitrary local file, passes unrestricted shell or escalates provider permissions. `review_start` includes exact target and consent linkage, not auto-transmitting full project. Tool responses untrusted data; mark side effects and errors. Use supported MCP transport/protocol version negotiation; no hand-coded incompatible protocol if official SDK viable. Verify stdout purity, initialization, cancellation and host permission prompts.

## Jobs, configs, provider adapter shapes

Job fields `id, kind, status, attempt, progress_current/total nullable, stage, retry_at, result_ref, error_code, created_at, updated_at, cancel_requested`. Never fake percentage. Interfaces conceptually `Agent.Probe/Start/Cancel/Result`, `Research.Search/Fetch`, `Parser.Parse/Resolve`, `Storage.Put/Get/ConditionalUpdate/Capabilities`, `Vault.Get/Set`; context cancellation, versioned request and typed error. Configuration persisted atomically (schema_version, capture policy, resource caps, provider references, sync schedule, startup opt-in) and validated; secrets referenced, not stored. Defaults documented and migrations backward compatible. Tests include malformed/oversize JSON, unknown version, read-only scope, idempotent replay and parent mismatch.
