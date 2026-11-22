# Dashboard redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. (Owner's standing rule: inline, no subagents.)

**Goal:** Replace the selector-driven dashboard with two levels (Everything, or inside one project) in the approved mockup's look, keeping every feature.

**Architecture:** Same files served by the Go program (`internal/web/ui/index.html`, `app.css`, `app.js`). Routing becomes `#/<page>` (Everything) and `#/p/<projectID>/<page>` (project); the old global `scope` is set from the route, so existing page bodies keep working. Every API route still maps to one `sessionkeep` command; the folder browser is a new command.

**Tech Stack:** Go 1.27 (stdlib), plain JavaScript (no framework, no build), CSS.

**Spec:** `docs/superpowers/specs/2026-11-23-dashboard-redesign-design.md`; look: `docs/design/dashboard-mockup.html`.

## Global Constraints

- Pages load nothing from elsewhere; no `innerHTML`/`eval`/storage APIs in `app.js`; no `style=` in `index.html`; CSP `style-src 'self'` means no inline style attributes from JS either (use classes) (`internal/web/ui_test.go`).
- Exactly four UI files: `index.html`, `app.css`, `app.js`, `favicon.svg`.
- Every form control labelled (`field(...)`, inside a `label`, or `x.setAttribute('aria-label', ...)`); buttons have text; radio groups are `fieldset` + `legend`; status never by colour alone.
- CSS keeps `:focus-visible`, `prefers-color-scheme: dark`, `prefers-reduced-motion`, `max-width: 520px`; the theme switch keeps working.
- Fonts: `Bahnschrift` for headings and figures, `Segoe UI` text, system fallbacks; no web fonts.
- Colours (light): canvas `#f3f5f8`, surface `#ffffff`, ink `#141922`, muted `#5d6675`, line `#e1e5eb`, accent `#0f7b65`, attention `#b5761a`, sidebar `#141920`.
- Sentence-case labels; no all-caps labels.

## Review Focus

1. Opening an old bookmark (`#/history/ID/SID/read`, `#/recap/week`, `#/projects/ID`) lands on the matching new page, not a blank one. (Task 4 test in smoke.)
2. A project id in the route that no longer exists (deleted project) shows the Everything overview with a notice, not an exception. (Task 4.)
3. Folder browser on a folder the user cannot read, or a path that is a file, answers with a plain error, and never lists files. (Task 1 tests.)
4. No projects at all: Overview and Projects show the empty state that leads to Add a project. (Task 5 smoke state.)
5. Dark theme: new graphite sidebar and Continue panel stay readable (dark variables defined for every new colour). (Task 3, checked by eye by the owner.)

---

### Task 1: `sessionkeep folders [PATH]`

**Files:**
- Create: `internal/cli/folders.go`, `internal/cli/folders_test.go`
- Modify: `internal/cli/cli.go` (dispatch `case "folders"`, usage line)

**Interfaces:**
- Produces: `sessionkeep folders [PATH] --json` data:
  `{"path": string, "parent": string ("" at a root), "places": [{"name","path"}], "folders": [{"name","path","git":bool,"project":bool,"excluded":bool}]}`.
  No PATH means the home folder.

- [ ] Test `TestFoldersListsSubfoldersOnly`: in a sandbox dir with `a/`, `b/.git/`, `.hidden/`, `file.txt`, and `a` registered as a project: folders are exactly `a`, `b` (sorted), `b.git == true`, `a.project == true`, no `file.txt`, no `.hidden`; `parent` is the sandbox's parent; `places` non-empty and contains the home folder.
- [ ] Test `TestFoldersRefusesWhatIsNotAFolder`: a file path and a missing path exit non-zero with code `not_a_directory`.
- [ ] Implement `foldersCmd(inv invocation, rest []string) error`: `os.ReadDir`, keep dirs, skip names starting with `.` or `$` and `System Volume Information`; `git` = `<dir>/.git` exists; `project` = normalized path equals a root of a project on this device; `excluded` = inside a user exclusion. Places: home, home/Desktop and home/Documents if they exist; Windows: each existing `X:\`; other systems: `/`, plus `/Volumes` (darwin) or `/mnt`, `/media` (linux) when present.
- [ ] Run `go test ./internal/cli -run TestFolders` → PASS. Commit.

### Task 2: Dashboard routes for folders, move and web research

**Files:**
- Modify: `internal/web/routes.go`
- Test: `internal/cli/dashboard_routes_test.go` (new; uses `signIn`)

**Interfaces:**
- Produces: `GET /api/folders?path=P` → `folders [P]`; `POST /api/projects/{id}/move` body `{to, name}` → `project move ID TO [--name NAME]`; `GET /api/research/fetching` → `research status`; `POST /api/research/fetching` body `{on: bool}` → `research on --yes` or `research off`.

- [ ] Test `TestDashboardFoldersMoveAndResearchSwitch`: folders lists a sandbox dir's subfolder; move points a registered project at a renamed folder (then `/api/projects` shows the new root); research switch on → status says on, off → off.
- [ ] Add the three handlers (move: `move` in the `{action}` switch reads `to`, `name`). Run test → PASS. Commit.

### Task 3: Shell and theme

**Files:**
- Modify: `internal/web/ui/index.html`, `internal/web/ui/app.css`

- [ ] `index.html`: sidebar = brand, `<div id="sidetop">` (back link + project card, filled by JS), `<nav aria-label="Sections"><ul id="nav"></ul></nav>` (filled by JS), `<div id="sideprojects">` (project list at the Everything level), footer (saving chip, version, theme switch). No scope `<select>`. Main: crumb, title row with `<span id="scopetag">`, lede, actions, notice, view. Keep skip link, one `h1`, `#dialog`.
- [ ] `app.css`: rewrite tokens and components to the mockup (sidebar, scope tags, cards, tables, tiles, seg, status line, Continue panel, This week grid, Needs you, Latest, folder picker, tabs) with dark-theme values for every token; keep the four required media/selectors. Commit with Task 4 (the shell needs the script).

### Task 4: Routing and sidebar

**Files:**
- Modify: `internal/web/ui/app.js` (routing, `show`, sidebar, header)

**Interfaces:**
- Produces: `parseRoute(hash) -> {project: Project|null, page: string, args: string[]}`; `GLOBAL_PAGES`, `PROJECT_PAGES` (ids exactly: global `overview, projects, search, instructions, settings, health`; project `overview, resume, recap, sessions, search, instructions, research, sharing, settings`); `projectHref(id, page, ...args)`; `scope` is `'*'` at the Everything level and the project id inside a project.
- Old routes map: `history/PID/...` → `p/PID/sessions/...`; `resume/PID` → `p/PID/resume`; `recap/DAY` → `p/<most recent project>/recap/DAY`; `research/PID/ID` → `p/PID/research/ID`; `projects/PID` → `p/PID/settings`; `diagnostics` → `health`.

- [ ] Implement; sidebar renders the right menu with `aria-current="page"`, project card (name, path, status words, Shared), and the project list at the Everything level. Header shows crumb + scope tag ("All projects" / "Only NAME").
- [ ] Unknown project id → Everything overview + notice "That project is no longer here."

### Task 5: Pages

**Files:**
- Modify: `internal/web/ui/app.js`

- [ ] Everything > Overview: status line; Continue panel (latest running or most recent session + its project's latest checkpoint; Pick up work / Open project); This week (sessions of the last 7 days from `/api/sessions?all=1&limit=200`, block class `short` < 20 min ≤ `mid` < 60 min ≤ `long`); Needs you (existing `attention()` items); Latest sessions (5). Empty state when no projects.
- [ ] Everything > Projects: table (name+path, status, what is saved, last session, sharing, Open) + Add a project (path field + Choose folder → folder picker dialog using `/api/folders`) + Import earlier sessions.
- [ ] Everything > Search everything (search with scope `*`), Instructions for all (global instructions only), Settings in tabs (New folders, Claude Code, Summaries, Cloud storage, Web research), Health.
- [ ] Project > Overview (stats, pick up, latest sessions, instruction counts), Pick up work, Recap, Sessions (history + "Bring in missing sessions from Claude Code"), Search, Instructions (project ones + note linking to Instructions for all), Research, Sharing (page form: storage tiles, own-bucket fields, name, passphrase with Show; when shared: Sync now, Show passphrase, Stop sharing), Project settings (what is saved + pause, recall mode, Move project, Delete).
- [ ] Update `.tools/smoke_ui.js` routes/answers to visit every page at both levels and press every button; `node .tools/smoke_ui.js internal/web/ui/app.js` → no exception.
- [ ] `go test ./internal/web` → PASS. Commit Tasks 3-5 together.

### Task 6: Docs and release record

- [ ] README: dashboard steps say Settings > Cloud storage, and the project's Sharing page.
- [ ] DECISIONS D132 (two-level dashboard, `folders` command, three routes), STATUS entry, full `bash scripts/check.sh` → CHECK PASSED. Commit.
