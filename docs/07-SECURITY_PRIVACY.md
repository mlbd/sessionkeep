# Security and privacy specification

## Threat model and trust boundaries

Assets: private project source, conversations, original instructions, research, test reports, agent authentication, development credentials, cloud keys and backups. Threats: curious local apps/browser origins; untrusted repo and prompts; malicious source content or agent output; unsafe symlink/archive/path traversal; subprocess escalation; leaked logs/credentials; cloud compromise or loss; supply-chain tampering; unauthorized cross-project sync; operator mistakes. Desktop malware with user-level access may access readable local data; do not imply protection against a compromised OS. Single-user local app initially, no multi-tenant public web service.

| Boundary | Controls / failure behavior |
|---|---|
| Native transcript -> spool | project opt-in, source allowlist, size and path bounds, redaction/exclusions before persistence, visible gaps |
| Spool -> SQLite | schema validation, idempotency, local user-only permissions, quarantine sanitized invalid records |
| DB -> agent | scoped authorization; evidence tagged untrusted; confirm external disclosures; no credentials returned |
| Web UI -> local API | loopback bind, random auth/bootstrap, CSRF/origin/host validation, no permissive CORS, session expiry |
| Research -> network | consent, SSRF protections, TLS, limits, sanitized evidence; no private data in query by default |
| Agent adapter -> subprocess | approved tool path and user identity, argv array, least privilege sandbox, no automatic escalation, safe logs |
| Sync -> cloud | authenticated encryption before upload, explicit scope and provider credentials, versioned restore and tombstones |
| Update -> binary | signed/checksummed releases, dependency audit and rollback; no executable download from untrusted repo (status 2026-11-25: not yet. Releases are packed and published by hand from the owner's computer, without provenance or published checksums; npm's registry hash is the only check) |

## Capture policy

Default fresh install capture disabled until onboarding + project authorization. User can pause instantly and see status. Exclude `.env*` (allow examples only when safe), credential dirs, SSH keys, native agent auth, cloud service account JSON, OAuth refresh/access tokens, bearer headers, private markers, secret-like PEM/URI/password patterns; redact in prompts, tool snippets, structured headers, paths and logs before spool/cache/export. Prefer allowlisting event fields over attempting only regex. Test redaction with multiline, encoded, nested, unusual token shapes; cannot guarantee all secrets detected. On uncertain content skip/quarantine non-sensitive metadata; user-visible reason. Do not scan private home dirs or collect unknown raw payloads. Native transcripts may retain original sensitive content; scope and deletion limitations made explicit. No hidden full text logging or telemetry by default. Support retention by project and capture category, export and deletion previews.

## Local authentication and UI

Bind loopback IPv4/IPv6 explicitly, random per-install or per-session secret stored under current OS user, authenticated handshake via short-lived one-time bootstrap (avoid long-lived tokens in URLs/history/logs), secure cookie strategy for HTTP loopback with SameSite/HttpOnly and CSRF token or explicit authenticated headers. Validate Host and Origin to resist DNS rebinding and CSRF; deny external origin, no `*` CORS. Require auth for all non-public resource endpoints, even localhost; SSE auth and reconnect. No arbitrary file read/write via path. CSP, escaped output, no inline execution of rendered markdown, no remote scripts/fonts by default, no browser access to OS credentials. Destructive actions require confirmation and audit. Local API access for agents separate scoped credentials/stdio trusted OS process, not general network. Audit tests for malicious web origin and unauthenticated endpoints; service non-loopback disallowed in v1.

## Provider and secrets policy

Use official CLI login and installed supported auth, API keys by user opt-in OS credential store; never extract/repurpose Claude or Codex login tokens. No credentials in env dumps, process arguments, UI response or report. Minimize inherited subprocess environment where compatible; test actual sandbox and configuration overrides. Read-only review default, not security boundary against every exfiltration; only share approved files, disable or restrict network if supported, warn on unsupported containment and refuse strict request rather than weaken permission. Untrusted repo scripts/commands/build hooks not autoexecuted. User chooses whether external provider receives project data and budget; capture local-only by default, no background billing. Preserve provider terms, licenses and attribution. Do not invoke independent agent blindly with secrets or proprietary context.

## Storage, deletion, backup

OS file permissions, no globally shared DB, local filesystem only. Optional OS full disk encryption; do not advertise local DB encryption unless implemented and tested. Cloud client-side encryption mandatory; vetted format, key recovery and rotation plan. Retention and export scope explicit. Deletion removes live rows, FTS/vector/index links, caches, applicable spool, blob refs and future exports; explain retained historical backups, native transcripts, storage snapshots and physical remnants. Provide purge/retention facilities and restore honoring tombstones. User must explicitly approve destructive removal, default uninstall preserves data. Revocation does not revoke previously downloaded copies.

## Security release gates

No unresolved critical/high defects in enabled feature trust boundaries, or feature disabled and explicitly disclosed pending resolution. Run dependency vulnerability/license scan, SBOM, secret scan of repository and release archives, fuzz hook/API/import parsers, auth/CSRF/SSRF/path tests, redaction fixtures and subprocess sandbox proof. Test update verification and untrusted manifest recovery. Review public logs and privacy docs. Use synthetic fixtures only in CI. Vulnerability reporting contact must be configured before release. Never publish `_private/` handoff or actual client transcripts. Separate license review for Graphify/Claude-Mem/Codex and parser packs; independent design is preferred.
