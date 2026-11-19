# ADR-0008 — First dashboard: plain pages embedded in the binary, API as a thin layer over commands

- Date / owner / status: 2026-11-20 / project owner / **proposed** (built this way in slice 1B-2; reversible; owner to confirm)
- Requirement IDs and affected phase: R08, R19, R22; D07 (frontend "candidates"); Phase 1B (1B-2), Phase 6
- Decision and constraints:
  1. The Phase 1 dashboard is three static files — `index.html`, `app.css`, `app.js` — embedded with `go:embed`. No React, Vite, Tailwind, npm or build step. The owner had approved downloading those packages; this decision does not use that approval.
  2. The HTTP API contains no logic. Each endpoint runs the same `sessionkeep` command a person would type, in-process, with `--json`. Consent rules, scope checks, previews and wording are therefore the same in the terminal, the MCP server and the browser, and are tested once.
  3. The server binds the IPv4 loopback address on a random port and nothing else; there is no option to bind another address.
  4. A request is accepted only with: the session cookie (HttpOnly, SameSite=Strict) obtained from a one-time sign-in link valid for two minutes; a Host header naming this server; no foreign Origin or cross-site fetch metadata; and, for anything that changes data, the session's request token in a header with a JSON body.
  5. Pages put data on screen with `textContent` only. A strict Content-Security-Policy forbids inline script, remote resources, framing and form posts.
  6. `sessionkeep dashboard` runs in the foreground until Ctrl+C and never opens a browser by itself.
- Evidence: `evidence/phase-1/1B-2.md` (attack tests against a real listening server, end-to-end owner journey through the API, real-binary smoke run, static checks of the pages).
- Alternatives and reasons for rejecting them: React + Vite + Tailwind/shadcn as listed in ARCHITECTURE §2 — several hundred transitive npm packages to pin, license and keep patched, plus a Node build step, for six screens of forms and tables; and with no browser automation in this run (owner rule) a larger front-end could not be verified beyond what the plain one can. A separate front-end server or Electron — excluded by the spec.
- Quality, durability, resource and compatibility implications: the binary grew by about 1.2 MB for the HTTP server and pages (20.5 MB unstripped). Serving used about 14 MB of memory in one measurement. Plain pages mean less polish than a component library: no virtualised long lists, no client-side routing beyond hash links, no design system.
- Security, privacy, licensing and cost implications: zero front-end dependencies, so nothing to audit or update there. Local malware running as the same user is out of scope, as stated in the threat model. The sign-in token appears in the terminal and, briefly, in the browser's address bar before the redirect.
- Migration / rollout / rollback / degradation plan: the API is the stable part; a richer front-end can replace the three files later without touching it. If the owner prefers React now, only `internal/web/ui` changes.
- Known limitations, open questions and next review trigger: the pages have **not been looked at in a browser by anyone**; appearance, layout at narrow widths, contrast and real keyboard flow are unverified (structure is checked by tests). No server-sent events: pages refresh on action, not live. One browser session per server run. IPv6 loopback is not bound. Review when the owner has used the pages, and at Phase 6.
- Approval / supersedes: awaiting owner; refines the frontend row of ARCHITECTURE §2.
