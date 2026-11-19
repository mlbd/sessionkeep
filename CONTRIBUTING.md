# Contributing — draft policy

SessionKeep is in pre-implementation planning. Contributions are welcome once the repository and scope are approved. Do not assume the blueprint is a supported release. Use the current `STATUS.md` and approved phase. Start with a scoped issue that references requirement IDs, expected behavior, alternatives and acceptance tests; discuss architecture changes through an ADR. Avoid speculative dependencies and broad unrelated refactors.

- Provide reproducible synthetic fixtures, not actual user transcripts, source code or credentials. Never include `_private/` in commits, public issues, release packages or AI submissions.
- Follow `CLAUDE.md` and `AGENTS.md`. Verify current authoritative interfaces, include documentation and test evidence; record real versus mocked checks. Unit, integration, failure recovery and security tests are expected for changed boundaries.
- Preserve compatibility, migrations, backup/restore and consent. No silent weakening of privilege/security rules, quality targets or test assertions. Optional integrations must not degrade core operation. Benchmark resource-sensitive changes on stated hardware.
- Document dependency version, licenses/NOTICE obligations and why the dependency is needed. No proprietary code, unsupported auth or hidden telemetry. Use pinned, reviewed release tooling.
- Changes to capture, networking, permission, encryption, process startup and licensing need maintainer review. AI-generated changes are the contributor's responsibility; independent review reports must be verified against the actual snapshot and test evidence.

Before public release choose code of conduct, maintainers, contribution license policy, review/branch rules, vulnerability reporting contact and support expectations. License is not finalized by this draft; do not publish until owner-approved LICENSE and required upstream notices are present.
