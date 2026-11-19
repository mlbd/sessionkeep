# Implementation roadmap and gates

No time estimate or promise of feature superiority. Each phase is a testable increment with separate acceptance and approval. Build the smallest vertical slice first; expand only when the core is reliable. Owner's desired final breadth remains intact, yet alpha release scope is smaller. Files `phases/PHASE_0.md`, `phases/PHASE_1.md` and `phases/LATER_PHASES.md` provide work packages. Keep STATUS and decisions current.

| Phase | Outcome | Gate / dependencies |
|---|---|---|
| 0 | Current contract research, actual environment inventory, architecture decisions, small synthetic prototypes | Approve baseline; no production implementation yet |
| 1 | Local reliable capture/search/recall, instructions, projects, continuity, minimal UI and export | T01–09 core, local T18, relevant T21/T23, owner-approved supported OS |
| 2 | Deep recall and optional enrichment, historical import and robust memory controls | T05–07, T14, redaction, quality eval |
| 3 | Research source/evidence, freshness and version-aware verification | T10–11, explicit offline and real opt-in research smoke |
| 4 | Graphify-style incremental code graph and linked knowledge | T12–13, language/coverage matrix |
| 5 | Manual Codex review, plan/requirement evidence, quality workflows | T15–17, approved real CLI test and read-only checks |
| 6 | Complete UI, configurable checkpoints/strict gates, operational diagnostics | UI journeys/accessibility, bounded gate tests, no fake status |
| 7 | Encrypted backup, travel handoff and restore; optional scheduled checkpoint | T18–20/T24, security-reviewed format, actual second-device restore |
| 8 | Public alpha/beta/stable distribution and OS support expansion | T22, full enabled feature gates, signed builds/license/name review |
| 9 | Two-way multi-device record sync, team features and other agents/provider adapters | T25 + separate threat model and conflict eval; no required cloud |

Phases 3–5 can prototype independently after Phase 1; review adapter spike Phase 0 is required to reduce risk early. UI evolves with each vertical slice, not as an unconnected mock. Core alpha may precede portability and graph expansion if owner approves feature labeling. Local export/restore is part of Phase 1, actual cloud is optional and later. Strict review gates cannot be shipped merely because a hook exists; measure reliability and allow safe fallback.

## Definition of done per work package

Requirement IDs and scope; actual code with migration/interfaces; unit/integration/failure tests; verified OS/provider behavior where claimed; privacy and license checks; documented operational changes and limitations; measured results where relevant; update STATUS, decisions, README and test evidence; independent review if authorized, findings adjudicated. No production work beyond approved gate. No auto Git commit/push or release; preserve user work.

## Release scope recommendation

Private developer pilot: Phase 1 + early approved research and Codex review slices. Public alpha: install/start/recovery, exact local memory, instruction management and UI, plus any separately qualified optional modules. Public beta: expand enabled graph/research/review and portability after actual gates. Stable: support matrix and reproducible security/performance/restore evidence. Do not advertise unfinished features as current or use a broad "best" claim without benchmarks.
