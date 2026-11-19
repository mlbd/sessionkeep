# Web UI/UX — proposed product design

The dashboard is a first-class control center, not just a memory feed. Existing browser, embedded frontend assets, no Electron, no remote network dependencies for core screens. Responsive desktop-first workspace, polished light/dark/system themes, keyboard accessible, virtualization/pagination for large histories. All states backed by actual data; no fake success, activity, percentages or tests. Reference project examples below are fictional.

## Information architecture

| Screen | Primary user tasks | Key states |
|---|---|---|
| Onboarding | choose capture scope, select projects, test local engine, connect optional integrations | not installed, consent missing, ready, degraded, skip |
| Overview | active project, last task, current instructions, pending research/reviews, engine health, sync status | empty, queued, failed, offline, degraded |
| Projects | add/disable/archive, assign paths/worktrees, project ID and policy, change project | duplicate root, ambiguous repo, missing path |
| Memory | search filter, original evidence, pin/correct/export/delete, history | superseded, redacted, gaps, stale cached data |
| Instructions | create/confirm scope, exact text, active/superseded/conflicts | draft, active, conflict, archived |
| Sessions/tasks | chronological conversation, task continuation and checkpoint/plan | interrupted, blocked, complete, branch changed |
| Research | claims, sources, freshness, verify now, compare versions, monitor | stale, disputed, unavailable, verified-with-date |
| Graph | select symbol/file, bounded graph, impact/path, edge evidence & coverage | indexing, partial, inferred, stale parse |
| Plans/requirements | approved immutable versions, mapping to code and tests | draft, review requested, missing evidence |
| Reviews/agents | choose provider, scope/permissions, job progress, triage and retest | not connected, refused, running, canceled, invalid report |
| Sync/travel | provider setup, snapshot preview, last upload/restore, devices, conflicts | disabled, unsynced, failed, uploaded, verified restore |
| Diagnostics | actual resource metrics, queued jobs, version compatibility, safe repair/export | paused, warning, failed, recovering |
| Settings/privacy | capture levels and exclusions, provider auth references, startup, retention, export/delete | explicit consent and confirmation |

Layout: left nav 220–260px collapsible, header with project/worktree switcher, status text, search and quick actions, content main with readable detail panels/right inspector optional. At small viewport nav becomes drawer and tables become scrollable or stacked detail, not horizontally clipped. Use consistent spacing 4/8/12/16/24/32; body 14–16px, high-contrast neutral palette, modest accent, status semantic colors + labels/icons, focus outline, no color-only differences. Avoid gratuitous animation. User can open dashboard via CLI or skill, no browser auto-open per session.

## Key wireframes (textual, not rendered assets)

```text
+ SessionKeep | Project: Example App / main | Search | Engine: Healthy +
| Overview | Last task: OAuth plan         | Action needed             |
| Memory   | Resume checkpoint [Open]     | 2 research claims stale    |
| Research | Current requirements         | 1 independent review open  |
| Graph    | Active instructions (7)      | Last capture 10:32          |
| Reviews  | Recent real event timeline   | Queue 0 / sync disabled     |
| Sync     | Review plan | Verify facts   | Diagnostics                |
| Settings |                             |                            |
```

Memory detail: list (exact search and filters) → detail with type/scope/version and original evidence tab → action pin/correct/supersede/delete. Show original and current derived text side by side, redaction markers, links to source session and related graph nodes. User-confirmed rules clearly distinct from AI suggestions. Deletion preview lists downstream records and backup limitations.

Research detail: claim statement, version constraint, checked date, expiry, source cards with publisher and exact evidence, previous-vs-current comparison, linked code/requirements, "verify now" with provider/data-sharing preview. Unknown or stale not disguised as green. Research output copy/export Markdown with source links and limitations.

Review composer: choose plan/diff/specific files and base ref, exact original requirements, independent provider, read-only default, inclusion/exclusion summary, estimate scope (cost unknown unless provider gives), start/cancel. Progress is state-based, not invented percentage. Findings triage includes severity/evidence/location/observed vs inferred/reproduction, adjudication, actual test evidence and new review when snapshot changes. Separate raw output vs parsed result.

Travel: choose projects/data; inspect dirty state, selected files, excluded secrets and dependencies; choose destination provider; validate recovery configured; create checkpoint/upload; show remote commit and separate restore verification. On restore show destination path, conflicts, missing runtimes and recovery options before modifying any local data. No destructive single-click overwrites.

## Engineering UI contracts

- Use typed API DTOs, schema validation, accessible native inputs, query cache invalidation by revision and SSE event IDs with polling fallback. Long lists server paginated; lazy load graph; avoid adding heavy bundle dependencies without measuring.
- Consistent loading skeleton, empty explanation + next action, recoverable errors with safe code and retry, destructive confirmation and undo where possible, toast for completion but persistent failure in details. Offline and engine-off distinguish from zero results.
- Keyboard navigation, focus restore, screen-reader labels, visible outlines, WCAG 2.2 AA design target, reduce motion, status text, graph accessible table alternative. Test contrast and keyboard flows using Playwright/axe plus manual review.
- Real resource panel: sample intervals, idle vs active engine stats, index size, queue age, last verified sync, storage categories, capture latency p50/p95. Optional processes/browser excluded or reported separately. Never claim token saving from hypothetical history size.
- Settings mutations validated on server, credentials never returned, permission scope preview, safe undo/repair. Provide accessible diagnostics export with redaction preview, no automated upload.

## UI MVP and later

Phase 1 minimal professional overview/project/capture consent/memory search/detail/health + settings. Research, graph and review screens arrive as working backend vertical slices; no dead placeholder buttons in release. Sync wizard deferred to portability release. Use fixture-only mockups for design development labeled example data. Test key journeys in desktop at 1280px and mobile at 390px, dark/light, empty, large history, degraded mode. No UI or pixel-perfect designs implemented in this package; design tokens/components refined after baseline review.
