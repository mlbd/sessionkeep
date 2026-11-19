# ADR-0007 — Unregistered folders: manual, ask (default) or owner-enabled auto-add

- Date / owner / status: 2026-11-19 / project owner / **accepted** by owner 2026-11-19
- Requirement IDs and affected phase: R01, R05, R19; D14; Phase 1A (slices 1A-3, 1A-4, 1A-5)
- Decision and constraints:
  1. One setting decides what happens when Claude Code runs in a folder SessionKeep has not seen: `manual` (nothing saved, nothing said), `ask` (nothing saved; a one-line reminder to run `sessionkeep project add`), `auto` (the folder is registered at a level the owner chose once, and capture starts).
  2. A fresh install is `ask`. `auto` exists only after the owner runs `sessionkeep setup --auto-add auto --level <level>`; the level is mandatory, may not be `off`, and SessionKeep never picks it.
  3. `sessionkeep project add` always requires `--capture <level>`; there is no default level.
  4. An exclusion list overrides everything, including an already registered project: at or below an excluded folder nothing is captured, registered or mentioned.
  5. Auto-add registers the enclosing repository root when there is one, otherwise the folder itself. It refuses a drive root, a top-level folder, the home directory and anything above it; those get a reminder instead.
  6. The decision is made from the settings file and a roots cache only. If either file is damaged the answer is "capture nothing" with an error, never a fallback to defaults.
  7. Hooks do not write the database (ADR-0003). In auto mode the hook spools the event with the root to register; the ingester creates the project (slice 1A-5).
- Evidence: owner request in conversation on 2026-11-19 ("to add a project … automatically added everything or manually by project add"; reply to the three-mode proposal: "that's more like it, let's do this way"). Tests and real-binary run in `evidence/phase-1/1A-3.md`.
- Alternatives and reasons for rejecting them: manual only (works against the one-time-setup requirement R01; easy to forget a project). Auto as the shipped default (records text on the strength of a default; unsafe for client work and hard to defend in a public tool). A default capture level for `project add` (same objection, smaller scale).
- Quality, durability, resource and compatibility implications: the decision costs two small file reads on the hook path. Path matching is lexical: symlinks, junctions and Windows short names are not resolved, so a folder reached through a link is treated as a different folder, and an exclusion does not cover a second path to the same directory.
- Security, privacy, licensing and cost implications: **this narrows a spec sentence.** `docs/10-INSTALL_OPERATE_RELEASE.md` says "Never auto-enable global capture." That now reads: SessionKeep never enables it by itself; the owner may. In auto mode, opening Claude Code in a sensitive folder that is not excluded starts capturing it at the chosen level — the exclusion list is the only guard, and the setup output says so.
- Migration / rollout / rollback / degradation plan: `sessionkeep setup --auto-add ask` or `manual` turns auto off at once; projects it already registered stay registered and can be paused or set to `off`. Projects record whether they were added manually or automatically.
- Known limitations, open questions and next review trigger: link resolution (above); no "remove project" command yet; the reminder text and how often it appears are decided in slice 1A-4/1A-6 when hooks exist. Review before any public release and when the web onboarding screen is built (Phase 1B).
- Approval / supersedes: approved by owner 2026-11-19; amends the capture-default sentence in `docs/07-SECURITY_PRIVACY.md` and `docs/10-INSTALL_OPERATE_RELEASE.md`.
