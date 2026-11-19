# ADR-0005 — Graph parser choice deferred to a dedicated spike; no Graphify embedding

- Date / owner / status: 2026-11-19 / project owner / **proposed**
- Requirement IDs and affected phase: R11, R15; D04, D11; Phase 4 (not Phase 1)
- Decision and constraints: Do not pick a parser now and do not let the graph shape the Phase 1 schema beyond reserving the table names in `docs/01-DATA_MODEL.md`. Before Phase 4, run one spike that scores each candidate on the same synthetic TS/TSX gold-edge fixture: (a) `odvcencio/gotreesitter`, (b) `msuozzo/bonsai` TypeScript/TSX modules, (c) tree-sitter WASM under wazero, (d) an optional parser-pack process using the official CGO binding, shipped separately from the core. Graphify is not embedded or vendored; at most it becomes an optional external adapter the user installs separately.
- Evidence: `evidence/phase-0/README.md` G1–G4 and §4 (Graphify is ~12 MB of Python; Apache-2.0 with MIT portions; official Go binding is CGO with no tagged releases; pure-Go options are young and were found by search only).
- Alternatives and reasons for rejecting them: Committing to CGO tree-sitter in the core (breaks ADR-0001's CGO-free build). Committing to a pure-Go runtime first published in 2026 without a correctness test. Reusing Graphify code (Python runtime is excluded for the installed core; mixed licence history needs per-file review).
- Quality, durability, resource and compatibility implications: The ≥95 % deterministic-edge precision target in the test plan stays provisional until the spike produces numbers.
- Security, privacy, licensing and cost implications: Grammar licences (tree-sitter-typescript and others) must be listed per pack before distribution.
- Migration / rollout / rollback / degradation plan: Parser sits behind the `Parser.Parse/Resolve` port already named in the interface contracts.
- Known limitations, open questions and next review trigger: None of the candidates has been compiled or run here. Review at the Phase 4 gate.
- Approval / supersedes: awaiting owner; narrows D11 from "native deterministic graph" to "decide by spike".
