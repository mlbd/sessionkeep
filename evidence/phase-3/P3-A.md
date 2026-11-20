# Phase 3, part A — claims with a checked source, and knowing when they go out of date

Date: 2026-11-20 (session 17). Agent: Claude Code (claude-fable-5-2). Windows 10 (amd64), one
machine. Work packages touched: P3-01 (snapshots and claim schema), P3-02 (secure direct fetch,
fixture, allowlist), P3-03 (freshness by category, explicit re-check). Decisions D94–D104.

## 1. What was built

- **Migration 0006**: `sources`, `source_snapshots`, `claims`, `claim_evidence`, `claim_checks`,
  a search index over claims. Everything belongs to one project and is deleted with it.
- **`internal/research`**: `Fetch` (one page, https, public addresses only), `Extract` (HTML to
  plain text), `QuoteIn`, `Passages`, the eight re-check categories.
- **Commands**: `sessionkeep research status | on | off | fetch | record | list | search | show |
  check | withdraw`.
- **Plugin**: a fourth skill, `research`, so Claude looks at recorded claims before relying on a
  fact that goes out of date, and records what it looks up.
- **Settings**: `research.enabled` (off by default), `research.allow_local`.

## 2. The rules, and the test that holds each

| Rule | Test |
|---|---|
| A claim is verified only if the quoted words are on the page SessionKeep fetched | `TestAClaimIsVerifiedOnlyByWordsThatAreOnThePage` |
| The database itself refuses a verified claim without a quote, a rewritten claim, a changed snapshot | `TestTheDatabaseRefusesAVerifiedClaimWithoutEvidence` |
| A claim becomes stale by the calendar; reading it changes nothing and fetches nothing | `TestAClaimIsVerifiedOnlyWithAQuoteAndGoesStaleByItself`, `TestAClaimGoesStaleAndIsCheckedAgainstTheSource` |
| Source unreachable (404, 429, 503, timeout): the claim is left as it was and says why | `TestChecksMoveAClaimOnlyOnWhatTheSourceSays`, same CLI test |
| Quote gone from the page: disputed, never silently kept or rewritten | same |
| "Not modified" answer: verified again; changed page that still holds the quote: verified against the new snapshot | same, `TestConditionalFetchReportsAnUnchangedPage` |
| Nothing is fetched until turned on; `on` needs `--yes` | `TestResearchFetchesNothingUntilTurnedOn` (request counter stays 0) |
| Private, loopback, link-local, metadata and reserved addresses refused, as literal addresses and at connect time for names | `TestOnlyPublicHTTPSAddressesAreFetched`, `TestBlockedAddressRanges` |
| A redirect is checked like the first address; a redirect never reaches a server that was not allowed | `TestRedirectsAreCheckedLikeTheFirstAddress` |
| No `file:`, `ftp:`, plain http, credentials or secret-looking text in an address | `TestOnlyPublicHTTPSAddressesAreFetched` |
| A certificate that cannot be verified is refused | `TestAnUnverifiableCertificateIsRefused` |
| Only the address and a plain identification are sent (no cookie, sign-in or referrer) | `TestFetchReturnsPlainTextAndSendsNothingButTheAddress` |
| Script, style, comments and tags are removed; hostile page text stays plain text and is labelled as data when printed | same, `TestExtractEdgeCases`, `TestResearchFetchPrintsThePageAsData` |
| Oversized page cut at 2 MB and reported; wrong content type refused | `TestFailuresAreReportedNotGuessed` |
| Claims are invisible from, and cannot be superseded or withdrawn from, another project | `TestSupersedeWithdrawAndProjectScope`, CLI test |
| Deleting a project deletes its claims, quotes, checks, sources and snapshots | `TestDeletingAProjectDeletesItsResearch` |
| A statement, quote or address that looks like a secret is refused | CLI test, fetch test |

All of these use a local test server (mock network, real program).

## 3. Rules broken on purpose

Twelve single-line changes, each run against the tests of its package. All twelve made a test
fail: address guard removed at connect; redirects not checked; private literal addresses
allowed; secret in an address allowed; script content kept; quote not required when recording;
missing quote counted as confirmed; fetch allowed while off; claims never stale; unreachable
source counted as verified; supersede across projects; claim readable from another project.

## 4. Fuzzing

`FuzzExtract`, 30 s, 310,727 inputs: no crash, no invalid text, no control characters. Short,
one machine.

## 5. One real source (D104)

The tests cannot cover a real https connection, so the agent made four requests to one public
page, `https://go.dev/doc/devel/release`, on 2026-11-20 between 08:19 and 08:21 UTC, with a
throwaway data folder:

| Step | Result |
|---|---|
| `research fetch ... --find "go1.27 (released"` | 150,684 bytes, 64,094 characters of text; printed the heading "go1.27.0 (released 2026-08-19)" |
| `research record ... --quote "go1.27.0 (released 2026-08-19)"` | stored as verified, re-check after 2026-11-27 (category release) |
| the same with a false date in statement and quote | refused: "does not contain the quoted words", nothing saved |
| `research check ID` | "confirmed" (that server sends no ETag or Last-Modified, so the page was read in full) |
| `research fetch https://localhost/` and `http://go.dev/` | refused before any request |

Nothing about the owner's projects was sent; the requests carried the address and
`sessionkeep/0.0.0-dev`.

## 6. Not built yet (rest of Phase 3)

- A notice at session start and a dashboard page for stale and disputed claims (P3-06).
- Comparing a claim's version with the version a project actually uses. `--applies-to` is free
  text that a person or Claude fills in; nothing checks it against a lock file (P3-03, part).
- Search providers (P3-04). Finding a page is left to Claude's own web search.
- Watching sources on a schedule (P3-05). Re-checks happen only when a command asks.
- Two sources that contradict each other. One claim has one quote from one page for now (the
  database allows several).
- Reading claims through the optional MCP server.

## 7. Not verified

- A real "not modified" (304) answer from a real server.
- A page not written in UTF-8; a page built by JavaScript.
- A network that requires a proxy (not supported: D97).
- Whether Claude follows the `research` skill in a real session.
- Linux, macOS.
