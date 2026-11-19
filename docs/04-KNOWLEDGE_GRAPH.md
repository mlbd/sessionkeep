# Knowledge graph and codebase intelligence

## Goals (R11/R15)

Graphify-inspired deterministic code understanding, not a visualization-first product. Connect files, functions, types, imports, calls, database entities, routes and documentation to decisions, requirements, research, review findings and tests. Use source coordinates and revision-specific evidence for every edge. Do not claim complete runtime dependency analysis from static syntax. Graphify is a reference project; no code reuse until per-file licensing and dependency review.

## Scope and representation

Initial parser target TS/JS/TSX (validate choice with owner), JSON/package manifests and Markdown links; SQL, Python, PHP, Go and others in later packs. Language support matrix explicitly lists extraction accuracy and limitations. Use Tree-sitter/native parser adapter after packaging tests; optional LSP resolution for precise semantic references; no mandatory Python runtime for installed core. Unknown languages get file metadata, not fabricated edges.

Nodes: repo, worktree, module, file, symbol (class/function/type), package/dependency, route, table/schema, document, external API, research claim, instruction, requirement, task, decision, test, review finding, commit, unresolved reference. Edges: `contains`, `imports`, `declares`, `calls`, `extends`, `implements`, `reads`, `writes`, `documented_by`, `depends_on`, `implements_requirement`, `uses_claim`, `reviewed_by`, `resolved_by`, `tested_by`, `contradicts`, `supersedes`. Each edge includes extraction kind deterministic/resolved/inferred/manual, source file+range or evidence ID, parser version, revision, confidence grade and validity. Inferred or unresolved not equivalent to confirmed. Node key contains project/worktree/file/symbol qualified name and disambiguator; rename mapping best effort, preserve old evidence. Do not equate a call-expression name with resolved call target.

## Indexing pipeline

- Enumerate allowlisted tracked/selected files using Git and safe traversal. Honor `.gitignore`, SessionKeep exclusions, generated/build/vendor/node_modules, binary/large file caps, symlinks/reparse points do not escape root. Never scan credentials or unconsented paths. Scope to branch/worktree and record source manifest.
- File watcher/change hook is advisory; debounce and coalesce; periodic manifest scan to reconcile lost events. Compare content hash + parser version + relevant resolver configuration (path aliases, package/tsconfig). Unchanged files not reparsed. Handle atomic write, delete, rename, branch switch, path alias changes and uncommitted changes. On resolver config change invalidate dependent resolution.
- Parse changed files outside write transaction, extract local definitions and reference candidates. Resolve only supported import rules; unknown/dynamic constructs produce unresolved links. Distinguish syntax nodes from semantic ones. Recompute affected reverse dependencies and bounded affected neighborhood; version graph generation; replace file edges/nodes transactionally. Retain last good generation if parse fails but mark stale, don't present it as current.
- Job concurrency default 1, cancel/defer on resource limits, bound file and graph query size. Handle user editing file while scan running by comparing fingerprint before commit, retry if changed. Full rebuild optional and explicit; graph is a derived index, rebuildable from source and linked evidence.

## Queries and UI

`graph.find_symbol`, `graph.neighbors`, `graph.path`, `graph.impact`, `graph.explain_edge`, `graph.coverage`, `graph.refresh`. Filter project/worktree, file type, edge kinds, revision and depth, cursor and limit. Query returns proof: path with edge IDs, human-readable relationship, file/lines, confidence and parser coverage; limit visited nodes/time to prevent runaway traversal; display partial results. Reverse dependency path is possible impact, not prediction of every runtime behavior. UI defaults to focused 25–100 nodes (adjustable), filters and expand-on-demand, show unresolved/inferred style and text alternative. Graph rendering lazy-loaded browser asset; no continuous background layout.

Example: `requirement:tenant-isolation -> feature:authorization -> file:auth.ts -> external API:identity -> research claim:token audience -> source snapshot`. If claim expires, show related feature at risk, not automatic code change. Link code to requirement through human/agent evidence, not guessed certainty. Reviews identify a snapshot plus finding; resolution linked to actual diff and retest.

## Acceptance tests

Synthetic TS repo with aliases, multi-module imports, calls with same symbol name, inheritance, dynamic imports, circular refs, ignored secrets and generated files. Gold edge sets per extractor, precision/recall by edge type, graph coverage, unresolved labels and incremental/full parity. Change one file, delete/rename, switch branch, break parse, change config; prove no stale valid edges, no cross-project leak and generation consistent. CPU/memory/index time measured at 1k/10k/100k files with explicit limits and partial coverage. Benchmark against Graphify with pinned version only if both run on identical authorized corpus and support same language set.
