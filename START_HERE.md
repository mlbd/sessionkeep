# First Claude Code prompt

Open Claude Code at the extracted folder and paste the following. The specification package is the input; no implementation exists yet.

```text
You are the lead engineer for SessionKeep, an open-source, local-first developer intelligence platform. Read README.md, CLAUDE.md, PRODUCT_SPEC.md, ARCHITECTURE.md, DECISIONS.md, ROADMAP.md, STATUS.md and every linked specification under docs/ and phases/. Read _private/Original_Project_Context.md if available for historical context only; do not commit or disclose it. The newest explicit requirements take precedence over old suggestions.

Start Phase 0 only. Inspect the actual machine and repository read-only. Research current official documentation, source code and release contracts, recording exact versions, links, commit SHAs where available, evidence and unresolved limitations. Do not substitute model recollection for live research. Verify the Claude Code hook payload/response and plugin-loading behavior; Codex CLI structured read-only review with approved local auth; lightweight Go/SQLite (including WAL security fixes and FTS5); graph parser distribution; the daemon-free capture/recovery path and startup options on Windows, macOS and Linux. Assess upstream licensing before copying anything. Prototype only with local synthetic data and explicitly approved tool installations. Do not change my existing Claude configuration or use real project data, invoke paid AI jobs, upload anything or publish a repo without my approval.

Deliver Phase 0 evidence, ADRs, recommended baseline and an ordered Phase 1 implementation plan with acceptance tests. Mark each item verified, failed, or not tested. Ask only for missing decisions that materially affect architecture or external actions. Flag conflicts without silently changing requirements. If Codex is available and I authorize its use, request an independent read-only critique and record the actual result. Do not begin production implementation until I approve the Phase 0 baseline.

Our primary goal is high-quality engineering, reliable long-term recall, live research, and developer continuity. Never sacrifice relevant context or quality to save tokens. The application must be lightweight and fail open for normal coding; optional integrations may not become mandatory dependencies. Keep STATUS.md current with actual progress, commands, results and next steps.
```

## After approving Phase 0

```text
Implement Phase 1 from phases/PHASE_1.md using the approved ADRs. Work through one vertical slice at a time. Add tests before or alongside implementation, run the actual tests, and record reproducible evidence. Do not claim unsupported operating systems or mocked integrations work. Stop at the phase gate, present unresolved defects and trade-offs, and update STATUS.md. Use Codex for an independent review only if authorized. Preserve the original product requirements. Do not implement later feature phases until their gates are approved.
```

## Starting a later session

```text
Read CLAUDE.md, STATUS.md, DECISIONS.md, and the relevant phase/specifications. Inspect Git status without resetting or discarding changes. Recover the latest approved plan, unresolved findings, evidence and next task. Verify current external requirements where necessary. Continue the next approved work package; report what you actually tested and update STATUS.md.
```
