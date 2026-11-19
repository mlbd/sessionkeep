# SessionKeep — engineering blueprint

**Status:** planning package, not runnable software. **Date:** 2026-11-19. **Name:** SessionKeep (owner decision 2026-11-19; trademark not checked).

A local-first, open-source intelligence layer for AI software development: durable conversational memory and instructions, evidence-backed fresh research, code relationship graphs, independent agent review, and encrypted portable workspaces. Optimize for engineering quality and continuity first; token savings are secondary. Keep optional features off until configured. The core must be fast, dependable and useful without hosted infrastructure or additional AI subscriptions.

## Start here (for the project owner)

1. Extract this archive into a new empty project directory, e.g. `D:\Projects\SessionKeep`. Keep a backup of it. This folder is not an installed plugin.
2. Open Claude Code from that directory. Paste the prompt in [START_HERE.md](START_HERE.md). The root [CLAUDE.md](CLAUDE.md) and [AGENTS.md](AGENTS.md) are concise standing instructions for coding agents.
3. Complete Phase 0 before production implementation. Review the proposed decisions, threat model and actual platform compatibility probes. Approve the baseline and then build a single vertical slice at a time, using test results as gates.
4. Keep the [STATUS.md](STATUS.md) updated after every session. Do not attempt to implement every feature in one prompt. Do not install unknown binaries, change machine startup settings or publish any repository without approval.

## Reading map

| File | Responsibility |
|---|---|
| [PRODUCT_SPEC.md](PRODUCT_SPEC.md) | Product requirements, priorities, scope and measurable outcomes |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Runtime, failure isolation, modules, stack and design decisions |
| [docs/01-DATA_MODEL.md](docs/01-DATA_MODEL.md) | DB schema contracts, migrations, invariants and search indexes |
| [docs/02-MEMORY.md](docs/02-MEMORY.md) | Capture, lossless permitted evidence, instructions, recall and continuity |
| [docs/03-RESEARCH.md](docs/03-RESEARCH.md) | Research pipeline, provenance, freshness, change detection and quality gates |
| [docs/04-KNOWLEDGE_GRAPH.md](docs/04-KNOWLEDGE_GRAPH.md) | AST, incremental indexing, graph queries and evidence relationships |
| [docs/05-AGENT_COLLABORATION.md](docs/05-AGENT_COLLABORATION.md) | Claude integration, Codex verifier, review workflows and adapter contracts |
| [docs/06-SYNC_AND_PORTABILITY.md](docs/06-SYNC_AND_PORTABILITY.md) | Versioned encrypted snapshots, device management, restore and later sync |
| [docs/07-SECURITY_PRIVACY.md](docs/07-SECURITY_PRIVACY.md) | Threat model, consent, credentials, trust boundaries and release blockers |
| [docs/08-UI_UX.md](docs/08-UI_UX.md) | Professional web UI, screens, flows, states and accessibility |
| [docs/09-INTERFACE_CONTRACTS.md](docs/09-INTERFACE_CONTRACTS.md) | CLI, hook/event, HTTP, MCP and job contracts |
| [docs/10-INSTALL_OPERATE_RELEASE.md](docs/10-INSTALL_OPERATE_RELEASE.md) | Packaging, startup, recovery, diagnostics and public distribution |
| [docs/11-TEST_EVAL_PLAN.md](docs/11-TEST_EVAL_PLAN.md) | Verification matrix, benchmarks and release gates |
| [docs/12-UPSTREAM_RESEARCH.md](docs/12-UPSTREAM_RESEARCH.md) | Official references, verified findings and outstanding spikes |
| [ROADMAP.md](ROADMAP.md) | Sequenced implementation and scope gates |
| [phases/PHASE_0.md](phases/PHASE_0.md) | Executable research/prototype backlog |
| [phases/PHASE_1.md](phases/PHASE_1.md) | First tested end-to-end vertical slice |
| [phases/LATER_PHASES.md](phases/LATER_PHASES.md) | Remaining feature work packages |
| [DECISIONS.md](DECISIONS.md) | Architecture decision register and approval requirements |
| [templates/](templates/) | Reusable plan, ADR, review, research, handoff and release forms |
| [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md) | Draft community and vulnerability reporting policies |

## Document precedence

Owner's explicit instructions and newest approved decisions > product requirements and security invariants > approved ADRs > detailed proposed contracts > historical context. Conflicts must be recorded, not papered over. Technical suggestions are provisional until tested; all scores, budgets and commands for SessionKeep are targets, not evidence of implementation. The current package verifies selected upstream capabilities but does not constitute a complete audit.

**Private material:** `_private/Original_Project_Context.md` is the earlier comprehensive conversation handoff and may contain company and personal project context. It is included for continuity only, ignored by the provided `.gitignore`, and must not be committed or published. Inspect the staging area before any commit; do not rely solely on ignore rules. The public documentation uses fictitious examples.
