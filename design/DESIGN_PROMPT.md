# Prompt for designing the SessionKeep dashboard

Copy everything below the line into Claude Design.

---

Design the dashboard for **SessionKeep**, a developer tool. I want 5 design directions that differ in **layout, navigation and interaction**, not only in colour. If two directions would look the same in greyscale wireframes, they are not different enough. Show every screen listed below for each direction, at desktop width, with realistic content.

## What SessionKeep is

SessionKeep is a local memory for people who code with Claude Code (Anthropic's coding assistant in the terminal). It runs on the developer's own computer and saves what the developer and Claude say to each other, per project, so a later session can pick up where the last one stopped. Nothing leaves the computer. There is no account, no cloud, no team features.

The dashboard is a local web page (opened with the command `sessionkeep dashboard`). The developer opens it a few times a week to do four things:

1. See whether saving is working and whether anything needs attention.
2. Read back or search an earlier conversation ("what did we decide about rate limiting?").
3. Pick up work: see where a project stood when they stopped, and whether the code changed since.
4. Manage what Claude is told and what is saved (standing instructions, researched facts, projects, privacy).

## Who uses it

One software engineer or web developer, alone, on their own machine. They live in an editor and a terminal. They are comfortable with dense information and keyboard use. They distrust tools that hide what is stored, so the dashboard must be plain about what is saved, what is not, and what a button will do before it does it.

## What is wrong with the current dashboard

- Every page is the same vertical list of rows with a status dot. Nothing stands out, and the most important thing on a page looks like the least important.
- The first page is a long scroll of unrelated sections (projects, sessions, counters, settings forms, accounts).
- Each page has its own project dropdown, so you choose the project again on every page.
- Reading a long conversation is tiring: list, click, new page, back, click the next one.
- Settings forms sit in the middle of content pages.
- It looks like a generic admin page, not like a tool made for developers.

## Screens and what each must contain

**1. Overview**
- Things that need attention, each with one action: captured events not yet stored ("Store them now"), a researched fact that is out of date or disputed, a project whose code changed since its last checkpoint, a project that is paused, a Claude Code account where SessionKeep is turned off.
- For each project: is it saving, paused or off.
- "Pick up where you stopped": the latest checkpoint per project (date, Git branch, the "next" line of the note, and whether the code has moved since).
- Latest sessions: project, start time, the first thing the developer asked.
- Counts of what is stored: projects, sessions, pieces of text, events, gaps on record.
- Empty state: no project added yet, with one action, "Add your first project".

**2. Projects**
- List of projects: name, folder path, state (Saving / Paused / Off), how much is saved. The four levels are: Conversation; Conversation and tool output; Activity only; Nothing.
- Per project: change the level, pause or resume, delete saved conversations, delete the whole project. Deleting always shows first what would be removed (counts) and asks for confirmation.
- Add a project: folder path, level, optional name.
- Import earlier Claude Code sessions: first a preview with numbers per project, then a confirm button.

**3. History**
- List of sessions, newest first, filterable by project: project, start time, number of pieces of text, running or ended, first prompt.
- One session opened: an optional summary written by a model (clearly marked as a summary, not what was said), then the conversation in order. Three kinds of turn: You, Claude, and tool use (one quiet line such as "Claude used Bash"). Each turn has a time and a "Delete this piece" action. A turn can carry the note "1 secret was removed before saving".
- Conversations are long (40 to 200 turns), with code and file paths in them.

**4. Search**
- Words to look for; filter by project and by who said it (me, Claude, a tool); option for exact phrase.
- Results: who, when, which project, the matching text, "Show all of it", and a way to open the session at that place.
- States: no results; only a partial match; some events are not stored yet and could not be searched.

**5. Instructions**
- Standing instructions that Claude receives word for word at the start of every session, for example "Never push to main. Open a pull request and wait for me."
- Each has a scope (one project or every project), a state (in use or retired) and a history of earlier wordings with dates and reasons.
- Actions: add (with a "read it once more, then confirm" step), change the wording, retire, see earlier wording.

**6. Research**
- Facts Claude looked up on the web, kept per project. Each has: a subject, a statement, the source address, the exact quoted words from that page, the date it was checked, and the date it should be checked again.
- States: Checked and current / Out of date / Disputed (the page no longer contains the quote) / Not checked (no source) / Withdrawn.
- A fact can apply to a package version (for example `chi@4`). If the project uses another major version, a clear warning: "Not about this project's version: the fact is about chi 4, this project uses chi v5.1.0 (go.mod)".
- Actions: "Check the page again", "Withdraw".
- A summary line with the count in each state.

**7. Pick up work**
- Per project: the last checkpoint (the note, the time, the Git branch, the number of uncommitted files), then either "The code is exactly as it was" or a list of what changed since ("2 new commits on vite-migration").
- The last few things said in that project.
- A form to write a new checkpoint: what is done, what comes next, what is blocking.

**8. Settings**
- Claude Code accounts on this computer, each with SessionKeep on or off and a button to switch it.
- Session summaries: on or off, which account, which model (Sonnet, Opus, Haiku), and how many requests were used today (for example 4 of 20).
- What happens in a folder that is not a project: remind me / ignore it / add it automatically.
- Folders that are never saved (list, add, remove).

**9. Health**
- Six checks, each Working or Not working with one line of detail: program version, where data is kept, settings file, installed copy, Claude Code connection, database engine.

## Sample content to use

- Projects: `storefront-api` (saving), `docs-site` (saving, added automatically), `billing-worker` (paused), `client-portal` (off).
- A session: "The checkout webhook retries forever when the payment provider answers 409. Find out why and fix it." Claude: "Found it. shouldRetry treats every answer outside 200–299 as temporary, so a 409 is retried until the queue gives up."
- An instruction: "Money is stored as whole cents in an integer. Never use floating point for amounts."
- A fact: "A failed webhook is retried for up to 3 days, with growing pauses." Quote: "We attempt to deliver a webhook for up to three days with an exponential back off."
- A checkpoint: "Done: a 409 ends the retry loop, with a test. Next: alert when the dead-letter queue holds more than 50 events. Blocking: nothing. Not pushed yet."

## Rules the design must keep

- Plain words on every label and button. A button says exactly what happens ("Delete saved conversations", not "Remove"). No jargon such as "ingest", "spool" or "evidence".
- A state is never shown by colour alone; it always has a word next to it.
- Anything that deletes shows what would go and asks first.
- Text that a model wrote (a summary) must look different from text that was actually said.
- What the developer typed, what Claude said, and file paths or code must be easy to tell apart.
- Light and dark themes for every direction.
- Works with the keyboard alone; visible focus.
- No web fonts, icon sets or images loaded from the internet. System fonts and inline SVG only, because the page must work offline.
- Usable down to a narrow window (about 400 px), but desktop is the main case.

## What I want back

For each of the 5 directions:

1. A name and two sentences on the idea behind it and who it suits best.
2. How navigation works, and where the project is chosen.
3. All 9 screens, plus one opened session, one delete confirmation and the Overview empty state.
4. What it does better and worse than the other four.

Make the directions differ in things like these: where navigation lives (side, top, command palette, tabs per project); whether lists open in place, in a side pane or on a new page; how dense the screen is; whether the project or the task is the main axis; how a long conversation is read; what the first screen puts first. Vary colour and type as well, but they must not be the only difference.
