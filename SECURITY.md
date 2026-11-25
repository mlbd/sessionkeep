# Security policy

SessionKeep stores what is said in Claude Code sessions, so a flaw in it can expose sensitive
conversations, project details or keys. Reports are welcome and taken seriously.

## Supported versions

Only the latest version published on npm (`npm view sessionkeep version`) receives fixes.
Update with `npm install -g sessionkeep`.

## Reporting a vulnerability

Please report privately, not in a public issue:

1. Open the repository's **Security** tab and choose **Report a vulnerability**
   (GitHub private vulnerability reporting).
2. Include the SessionKeep version (`sessionkeep version`), your operating system, what an
   attacker could do, and the smallest safe way to reproduce it.

Do not include real secrets, real conversation text or working credentials; made-up values are
enough. Do not test against other people's computers, accounts or buckets.

You can expect an acknowledgement within a week. Once a fix is published, the report can be
disclosed; credit is given unless you prefer otherwise.

## What SessionKeep is designed to guarantee

- Nothing is captured in a folder until the user registers it with a capture level, and
  excluded folders are never captured.
- Secrets of known shapes (keys and tokens of common services, private keys, passwords in
  connection strings, commands and `.env`-style names) are redacted before anything is written to
  disk, and the output of a tool that read a file which commonly holds secrets (`.env`, key
  files, credentials) is not kept. A secret with no label and no known shape can still be
  stored; redaction is a safety net, not a guarantee.
- The dashboard listens on the loopback address only and requires a one-time sign-in link.
- No telemetry and no network use by default. Summaries, web research and cloud sharing are off
  until the user turns them on.
- Shared bundles are encrypted on the sending computer with the project's passphrase
  ([age](https://age-encryption.org)) before upload. Storage keys and passphrases are encrypted
  for the signed-in Windows user (Data Protection API); on macOS and Linux they are kept as plain
  text in a file only the user can read.
- The local database is not encrypted. Anyone who can read the user's files, or a backup of
  them, can read what was saved; full-disk encryption protects a lost or stolen computer.
- An AI agent cannot approve a lasting change for the user. An instruction Claude saves, or one
  received from a teammate, is given to no session until the user approves it on the dashboard
  or in their own terminal; changing or deleting instructions and data, syncing, and turning on
  summaries, web fetching or automatic recall are refused when Claude runs the command. What
  Claude is told says who wrote each instruction, and marks a teammate's text as theirs.

The last point stops the commands an agent is commonly allowed to run. It is not a barrier
against a program that already runs as the user, which can change anything the user can.

A way around any of these is a vulnerability. The design and threat model are in
[docs/07-SECURITY_PRIVACY.md](docs/07-SECURITY_PRIVACY.md).
