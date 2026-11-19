# Agent adapters, review and quality assurance

## 1. Claude Code integration

Plugin hooks capture permitted events; SessionStart/UserPromptSubmit can deliver small scoped recall and freshness notices via documented context fields. Stop/StopFailure and reconciliation collect responses/checkpoints; SessionEnd is best effort. Optional PostToolUse/Failure captures meaningful metadata; do not intercept normal operations or alter permissions. Skills provide explicit recall, research, graph, review and resume workflows; stdio MCP exposes typed tools. Use current plugin/marketplace manifest contracts and installed version probes from Phase 0. Avoid plugin scripts launching background workers each time.

## 2. Codex integration (R13/R14)

The official OpenAI `codex-plugin-cc` uses local Codex CLI/App Server, including read-only review and adversarial review. Treat it as an upstream reference and optional user workflow, not a required runtime. Prefer adapter for `codex exec` for initial structured review. Check installed binary via resolved absolute path, version/help, auth readiness through supported CLI, read-only sandbox and environment restrictions. Do not read/copy authentication files or provider tokens. User's own access and usage limits apply. Do not route subscription credentials through SessionKeep. No automatic login in hooks. If CLI missing/incompatible/not signed in, show actionable status and offer manual supported setup; core unaffected.

Illustrative launch (validate actual flags/version): `codex exec --sandbox read-only --json --output-schema <trusted-schema-path> -o <job-result-path> -` with prompt on stdin, explicit cwd, no shell interpolation. Use verified compatible configuration for no interactive approval; do not fail open to broader sandbox or dangerous flags. Result output to private job directory permitted by actual sandbox contract; test whether host-output path works. Use stdout/stderr bounded and safe, capture final structured result via validated schema; do not parse shell commands from output. Provider may not allow network or tests in read-only mode, disclose; do not implicitly enable network or write permissions. For external submissions require explicit consent identifying files and context transmitted. Structured output does not imply accuracy.

## 3. Review target and handoff

- Review type: plan, diff, requirements, architecture, security, research/fact-check or specific files. User selects target, base ref/working tree/branch and focus. No ambiguous auto scope in strict mode.
- Record snapshot: project, branch/ref, Git HEAD, dirty file hashes, exact diff/plan and requirement versions, selected memory IDs and evidence, omissions and privacy filters. Prefer dedicated read-only copy or temporary worktree plus safe patch for dirty review where possible, not executing untrusted code. Detect change during review and mark stale. If cannot create immutable snapshot, explicitly record observed target and hash comparison, no claim of immutable review.
- Prepare context: original requirements, approved plan, relevant project graph/evidence, latest applicable research and test output. Independent reviewer sees user requirements and target, not earlier reviewer conclusion by default to reduce anchoring. Do not hand over raw full transcripts or credentials unless explicit allowed data scope.
- Run reviewer on demand; job states visible; cancellation terminates process tree safely; partial results not approved. Store raw permitted output, schema parse status, structured findings, provider/model/version if known, actual context manifest, timestamp and optional usage. Retry not auto-billing after ambiguous failure.

## 4. Review result schema (logical)

`run_id, snapshot_id, verdict: needs_attention|no_material_findings|incomplete, summary, evaluated_requirement_ids[], unmet_or_unknown_requirement_ids[], tests_observed[], checks_not_run[], findings[]`. Finding: `id, severity: critical|high|medium|low|info, title, description, path_or_plan_section, line_start/end(nullable), evidence, reproduction_or_failure_scenario, recommendation, confidence(0..1), classification: observed|inferred, requirement_ids[]`. Reject invalid fields/path traversal/unknown IDs; show raw report separately and parser failure. Validate locations against snapshot, do not fabricate lines for plan issue. If provider can't produce schema retain report as unstructured, mark incomplete; no autoapproval.

Claude/owner adjudicates findings `open → accepted|rejected_with_reason|needs_evidence → resolved → verified`, each change linked to evidence. Re-review target after changes; old approval not valid for modified snapshot. No automatic remediation; user can request Claude to address accepted findings. Read-only reviewer may inspect tests but cannot prove they ran; actual test executor separate with explicit permission for write/exec as needed. Reject unverified claims of perfection.

## 5. Modes and configurable roles

Manual default: user explicitly invokes provider job. Checkpoint: opted-in milestones trigger configured reviewer with budget/consent. Strict: application completion status remains blocked until approved evidence prerequisites satisfied; host gate only within documented hooks, finite attempts, user override recorded, offline status incomplete. Never stop normal Claude work if SessionKeep unavailable unless explicit user-configured safety policy permits and platform behavior tested; don't override host's own permission policy. Roles configurable: primary dev, research, planner, plan reviewer, code/security reviewer, summarizer, test verifier. Agent adapter capabilities `review`, `exec`, `structured_output`, `cancel`, `resume`, `network`, `sandbox_read_only`; capability check before operation. App Server live thread integration and other agents later. No always-running Codex process.

## 6. Commands and workflow example

Future `/sessionkeep:review-plan` asks independent Codex to challenge assumptions, versions, feasibility, failure behavior. `/sessionkeep:verify` compares code snapshot against requirement IDs, stores review, returns report path/ID to Claude. `/sessionkeep:review --agent codex --scope working-tree` normal review. `/sessionkeep:research` can use approved external provider/agent. `/sessionkeep:status`, `:result`, `:cancel` monitor jobs. Natural-language invocation via skill/MCP when enabled and permitted. All names proposed; test host namespace/arguments. Existing official Codex plugin may coexist but avoid duplicate gates/jobs.

## 7. Acceptance

Mock CLI fixture validates schema, args, special chars, cwd, cancellation, timeout, nonzero exit, login missing, malformed JSON, no network, large stderr, permission failure, snapshot mismatch and no unexpected write. Real smoke with explicit owner consent: supported installed Codex, seeded defect and insecure plan, read-only filesystem check before/after, actual returned report, correct version and permissions. Test unsupported mode does not escalate; no credentials in logs. Compare independent review to known labeled defects, not number of findings. Review success alone never marks required tests passed.
