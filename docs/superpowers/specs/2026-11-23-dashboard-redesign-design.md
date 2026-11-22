# Dashboard redesign: everything, or one project

Date: 2026-11-23. Owner approved the look and structure in the clickable mockup
`docs/design/dashboard-mockup.html` ("I like the whole design, lets implement it properly").

## Why

The current dashboard keeps one "Project, on every page" selector in the sidebar, and every page
silently follows it. The owner could not tell whether Overview, Pick up work, Recap, History,
Search, Instructions, Research, Projects, Settings and Health were about one project or all of
them. The redesign makes the scope visible and structural: you are either looking at everything,
or you are inside one project.

## Goals

- Two levels, never mixed:
  - **Everything** (no project open): Overview, Projects, Search everything, Instructions for all,
    Settings, Health; the sidebar also lists the projects.
  - **Inside a project**: a project sidebar (back link, project card with status, then Overview,
    Pick up work, Recap, Sessions, Search, Instructions, Research, Sharing, Project settings).
    Every page there is about that project only.
- Every page shows its scope: a blue "All projects" or green "Only NAME" label, and a breadcrumb.
- Keep every feature the dashboard has today (list below), reachable in one obvious place.
- The look of the mockup: graphite sidebar, cool grey canvas, Bahnschrift headings and figures,
  Segoe UI text, green accent `#0f7b65`, amber for attention; sentence-case labels; the Overview's
  Continue panel and This week timeline.
- Add a project by typing/pasting a path **or** choosing a folder in a folder browser that works
  on Windows, macOS and Linux.
- Move a renamed project folder from Project settings (the command exists; the dashboard did not
  offer it).

## Non-goals

- No change to what is saved, the commands' behaviour, or the data model.
- No new build step, framework or dependency: plain `app.js`, `app.css`, `index.html` served by
  the Go program, as today.
- No web fonts or other network loads (Bahnschrift and Segoe UI ship with Windows; other systems
  fall back to their UI font).

## Anti-goals (would be a failure even if it works)

- A page whose scope is unclear.
- A feature from today that can no longer be found.
- A folder browser that only works on Windows, or that exposes file contents.

## Features carried over (where they go)

| Today | New place |
|---|---|
| Overview: stored counts, needs attention, latest sessions, pick up | Everything > Overview (status line, Continue panel, This week, Needs you, Latest sessions) |
| Projects: list, add, import earlier sessions | Everything > Projects |
| Project panel: what is saved, pause/resume, recall mode | Project > Project settings |
| Project panel: share / sync / show passphrase / stop sharing | Project > Sharing (a page, not dialogs; confirmations stay) |
| Project panel: sync with Claude Code (bring in missing sessions) | Project > Sessions |
| Project panel: delete saved conversations / whole project | Project > Project settings, Delete (still shows what would go and asks) |
| Pick up work, write checkpoint | Project > Pick up work |
| Recap (today / yesterday / week) | Project > Recap |
| History and one session (summary, show all, delete a piece) | Project > Sessions |
| Search (who said it; delete a piece) | Everything > Search everything, and Project > Search |
| Instructions (in use / retired, add, change, retire, things you repeat) | Everything > Instructions for all (global ones), Project > Instructions (project ones, with a note and link to the global ones) |
| Research (facts, check again, withdraw) | Project > Research |
| Settings: unregistered folders, never-saved folders, accounts, summaries, cloud storage | Everything > Settings, in tabs: New folders, Claude Code, Summaries, Cloud storage, Web research |
| Health | Everything > Health |

## Approaches considered

1. **Rewrite the page layer around two levels, keep the API** (chosen). Routes become
   `#/overview`, `#/projects`, `#/search`, `#/instructions`, `#/settings`, `#/health`, and
   `#/p/ID/<page>`. Page functions take the project from the route instead of the global `scope`.
   Existing API routes are reused; three small routes are added.
2. Keep today's pages and only restyle and relabel them. Cheapest, but the selector-driven scope,
   which is the actual problem, would remain.
3. A front-end framework or build step. Rejected: no runtime or build dependency for the core
   (CLAUDE.md), and the page is small enough without one.

## Decisions

- **Folder browser: SessionKeep's local server lists folders** (owner asked for cross-platform;
  a browser page cannot read full paths itself). New `GET /api/folders?path=P` returns the folder's
  subfolders (names only, sorted, hidden ones left out), whether each is a Git repository (has
  `.git`) or already a project, the parent, and "places": the home folder, Desktop and Documents
  when they exist, drive roots on Windows (the letters that exist), `/` on macOS and Linux, plus
  `/Volumes` (macOS) and `/mnt`, `/media` (Linux) when present. No file names or contents are
  returned. Excluded folders are marked. The native OS dialog was considered and rejected: it
  depends on PowerShell/osascript/zenity being present and can open behind the browser.
- **Move a project: `POST /api/projects/{id}/move`** with `{to, name}` runs `project move`.
- **Web research switch: `GET/POST /api/research/fetching`** runs `research status` and
  `research on --yes` / `off`.
- The sidebar's project list shows each project's status; the Everything level no longer has a
  project selector anywhere.
- Old links (`#/history`, `#/recap`, `#/resume`, `#/research`, `#/projects/ID`) redirect to the
  new places, using the project they named or the most recent one.
- The This week timeline is drawn from the sessions list (start time, project, length from start
  and end); block width is short / medium / long by duration.
- Needs you is built from what the program already reports: paused projects, health checks not
  working, team bundles waiting (team status), stale research, capture gaps.

## Left to the builder

- Exact component structure inside `app.js`, CSS organisation, empty and error states (in the
  dashboard's existing plain voice), and keyboard/accessibility details (labels for every control,
  visible focus, `aria-current` on the active menu item), within the existing UI tests.

## Testing

- Go tests for the three new routes (folders: lists subfolders, marks Git and projects, refuses a
  path that is not a folder, returns no files; move; research switch).
- `internal/web/ui_test.go` accessibility rules keep passing; extend them for the new navigation.
- `.tools/smoke_ui.js` (mock page, not a browser) updated to visit every route at both levels and
  press every button.
- Full `bash scripts/check.sh`. A real-browser look is the owner's (no automated browser runs).
