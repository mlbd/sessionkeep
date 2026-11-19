# Research intelligence and knowledge freshness

## 1. Purpose (R09/R10)

When an agent asks about current API requirements, versions, prices, policies, security, quota or implementation details, use previous knowledge as a starting point, not truth. Freshness is claim-specific and version-specific. Current source is not necessarily correct for the installed dependency. Research must work across sessions and preserve evidence, timestamps, uncertainty and comparisons.

## 2. Pipeline

1. Determine task scope, installed library/API version, target platform and material claims. Prompt classifier suggests research; explicit `/sessionkeep:research --latest` always requires new verification. A user requesting offline mode gets honest unavailable status.
2. Retrieve relevant previous claims and authoritative source metadata. Evaluate policy based on risk, version match, last successful verification and explicit current request.
3. Search via configured adapter or agent's supported web integration; fetch authoritative docs, changelog, official source/releases; community reports only secondary evidence. Search snippets are discovery not full verification. Direct HTTP fetch for known URLs; dynamic browser/external search optional later. Do not automatically crawl entire sites.
4. Normalize and secure URLs; record retrieval time, response/final URL, version/snapshot digest, relevant small evidence excerpts/locations, source terms and access limitations. Respect rights, robots/terms where applicable, and rate limits; never store/paywall bypass or assume blanket rights to republish third-party content.
5. Extract candidate claims with evidence links, applicability, contradictions and confidence. Technical claims that cannot be sourced remain candidate/unverified. Validate version against actual dependency, install/runtime probe where possible. Human review for conflicting material decisions.
6. Commit claim version, evidence and link to project/requirement/code dependencies. Retain prior version marked superseded only when justified; stale when deadline elapsed; disputed if evidence conflicts. No timestamps updated merely because an agent read saved research.
7. Return report with checked claims, date, applicable version, source links, missing evidence and next steps; record check in job history. On future queries, revalidate before implying current truth.

## 3. Initial configurable policy (proposed)

| Category | Default revalidation window | Hard trigger |
|---|---:|---|
| Security advisory / live price | 1 day | before security-sensitive fix or purchase decision |
| Quota / availability / rate limits | 3 days | account-specific check before relying on limits |
| OAuth, permissions, platform policy | 7 days | before new integration or sensitive auth change |
| SDK releases | 7 days | dependency version changes |
| API contracts | 14 days | version/endpoint changes or breaking change report |
| Stable framework patterns | 30 days | installed version mismatch |
| General concepts | 180 days | new material evidence |
| Confirmed user instruction | no TTL | user correction |
| Architecture decision | no TTL | superseding approved decision or invalidated premise |

TTL means time after last successful appropriate verification, not after local access. Explicit latest, conflicting source, changed version and high-impact decisions override TTL. Scheduler optional, only monitor enrolled sources, cap requests and concurrency, retry with jitter, timeouts, max size and provider quotas; avoid waking sleeping machine. UI shows next check and failure reason. A 304 may extend snapshot freshness if policy allows and applicability has been checked; not proof a claim is correct. Content change triggers targeted re-evaluation of linked claims, not automatic overwrite. Security and high-risk claims may need release/advisory and actual platform validation, not just unchanged docs.

## 4. Source trust and data contract

Claim: `id, project_id, subject, statement, scope, version_constraint, status, verified_at, expires_at, source_evidence_ids, unresolved_conflicts, evaluator, verification_method`. Snapshot: `source_url, final_url, fetched_at, status, content_hash, etag, last_modified, extracted_text_or_quote, location, source_published_at_if_known, license/retention`. Separate observed source text from model interpretation. Verification grades: primary authoritative documented; reproduced against target; secondary report; unknown. "Verified" requires at least appropriate evidence and no known blocking contradiction; it does not mean infallible. Distinguish official docs vs observed account-specific access.

## 5. Agent enforcement and failure modes

Use prompt-time context listing stale/missing claims; tools `research.search`, `research.fetch`, `research.verify`, `research.record`, `research.freshness`, `research.compare`. Host web tool visibility may be limited; require explicit source recording with evidence. Built-in direct-fetch adapter verifies actual retrieval; provider search adapter behind consent and credential boundary. In normal mode nonblocking reminder and dashboard warning; strict configured completion gate checks required evidence and flags unsupported claim before completion. Cannot guarantee model compliance or block all agent pathways. Stop gates bounded (one continuation attempt by default and honor host loop protection), explicit override and unavailable states; never create infinite loop or mandatory network dependency.

## 6. Research security

Untrusted page text never system instructions. Strip scripts and active HTML; limit redirects, prevent SSRF (deny private/reserved addresses by default; re-resolve/validate each redirect/connection; explicitly allow listed local sources if intentional), prevent file URLs, credential-bearing URLs and arbitrary shell. Use TLS validation, caps, timeouts and explicit user consent for outbound project context. Queries should not include secrets or full internal code. Store quote-sized evidence when required by source restrictions, no republication of proprietary documents. Rendering sanitized, escaped text only.

## 7. Acceptance tests

Fixture HTTP source changes endpoint version, price, redirects and metadata; fake provider returns plausible stale answer. Verify explicit latest bypass, correct installed-version filtering, TTL transition, 304 handling, unsupported/404/429/timeouts, contradictory docs, stale offline fallback, prompt-injection text treated as data, SSRF redirects blocked, monitored-source update flags dependent code. Verify stale claim cannot be represented as current without evidence. Manual integration run with approved real public source logs timestamp and evidence. Provider search and model output may be inaccurate; no external service required for core.
