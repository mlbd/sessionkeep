# Portable workspaces and encrypted synchronization

## 1. Objectives and non-goals (R16–R18)

Development stays local. A developer can checkpoint authorized projects, turn off desktop, restore to laptop and continue with files, uncommitted work, memory, plans, research, graph and agent context. Cloud is storage, not a remote execution environment. Optional bring-your-own-cloud (BYOC), no hosted account required. First implement recovery snapshots, not a live distributed filesystem. Full bidirectional sync and team collaboration are later phases, not assumed simple extensions.

## 2. Provider choices

| Option | Initial decision |
|---|---|
| Local encrypted archive / export | Supported first; migration and recovery foundation |
| S3-compatible storage (R2, S3, B2 candidates) | First cloud adapter after compatibility test |
| Restic | Candidate for established encrypted deduplicating backup, opt-in external binary; validate licensing and credential handling |
| Google Drive/OneDrive/Dropbox | Later adapter or vetted rclone integration; OAuth and distribution requirements separate |
| Syncthing | Optional external transport, not core; needs another online copy and conflict management |
| Git remote | Recommended source history transport if configured; no automatic push by default |
| Self-hosted server | Later for multi-device access control; never mandatory |

Do not sync active SQLite WAL/SHM files, live Git worktrees via ordinary folder sync, agents' OAuth stores or OS-specific binaries. Git itself does not capture uncommitted/untracked data; workspace snapshot must include selected dirty state independently. Provider setup performed once with narrowly scoped credentials, OS key store and explicit permission to upload company source. Network quotas and prices verified at setup, not hard-coded. No mandatory vendor infrastructure.

## 3. Workspace manifest and snapshot creation

Manifest version + schema, project UUID, device ID, checkpoint ID/parent, creation time, source OS, tool versions, Git repo/worktree identity, HEAD/ref, tracked/untracked selected file manifest (relative path, digest, size, mode, resource type), staged/unstaged state and deletions, included data categories, exclusions, safe DB snapshot or logical record export, pending captured events, graph/index version, selected native session data, task checkpoint, environment manifest, encrypted object references and authenticated integrity metadata. Preserve file names and permissions carefully; check Windows reserved names, case collisions, unicode normalization, max lengths, executable bit and line endings at destination. Symlinks/reparse points excluded by default, never follow outside roots. Avoid submodule surprises; represent submodules explicitly; do not download dependencies without approval.

Local checkpoint state machine: draft → preflight → capture → local_verified → encrypt/upload → provisional_remote → published → restore_verified (or failed/incomplete). Capture at explicit safe point; file manifest hash before/after copy, detect races and report inconsistent files, retry or ask user to quiesce; no false guarantee of atomic multi-file project snapshot. Include only completed DB backup and durable pending spool; reconcile IDs on restore. Use SQLite backup/consistent logical export with integrity check. Optional local project DB exports via explicit adapter (Postgres dump etc.) with permission, not blindly copying live files. Save environment versions and lockfiles; no node_modules, build caches, running processes or plaintext secrets by default. No bulk upload without preview including paths/categories/estimated size and exclusions.

## 4. Encryption and remote commit

Prefer audited encrypted backup format and verified library (e.g. Restic) rather than custom cryptography. Phase spike must define versioned encryption, key derivation/recovery, authenticated manifests, objects and key separation; security review before public cloud release. Private key/recovery secret never stored in plaintext next to remote ciphertext. Do not claim zero-knowledge if provider metadata, endpoint or size leaks remain. Local plaintext may exist in authorized app storage; OS disk encryption recommended.

Remote adapter contract (not code): `putImmutable`, `get`, `stat`, `list`, `deleteOwned`, `conditionalUpdate`, `capabilities`, `verify`. Upload immutable authenticated content/manifest, verify digests and availability, then publish pointer atomically with conditional create/update when provider supports it. Test conditional write semantics (including ambiguous retries and conflicts); do not use ETag as content hash. If provider lacks safe CAS, restrict to single-writer snapshots and immutable history, no automatic shared latest overwrite. Stage incomplete snapshots invisible as published; orphan cleanup only after grace period and verified references. Retain multiple generations, configurable space/retention, explicit purge. No cloud restore success until actual verified download and local restore has passed; distinguish uploaded from restore-verified.

## 5. Restore and device handoff

1. Enroll authorized destination using recovery secret and provider credentials separately; never copy native agent auth files.
2. List authenticated snapshot IDs, versions and timestamps; user picks project and destination. Download to isolated staging with size limits and expected digest, redact/validate as applicable, verify encryption integrity and manifest.
3. Detect case/path/symlink issues and conflicts, required free disk, existing dirty repository, DB schema mismatch and permissions. Refuse path traversal/zip bombs and overwriting live data. Restore new directory by default. For existing project require preview and local pre-restore backup; merge Git via review not force checkout/reset.
4. Restore selected tracked and untracked files, approved Git state and SessionKeep DB via supported import, idempotently. Apply pending events without duplicating messages; derived graph optionally rebuild. Confirm exact file and evidence counts plus exclusions; atomic directory move when safe; rollback if incomplete.
5. Detect required Node/Go/Python/package manager/database tools and versions from manifests (`package.json`, lockfiles, `.tool-versions`, documented scripts). Offer dry run and explain commands; never autoexecute untrusted build/install scripts or blindly source env configs. User approves installs and authentication on new OS.
6. Attempt exact native agent session resume only with tested format/host/version/path compatibility and explicit opt-in. Default semantic continuation includes original permitted evidence, instructions, current task, latest plan, research and dirty-state manifest. Native restore failure does not block work; show limitation.
7. Travel status: `ready` requires verified published checkpoint, required data all included and known incomplete items disclosed. Restore rehearsal validates before travel, then home PC can remain off; data after last successful snapshot unavailable. Network required for cloud agent models and remote storage.

## 6. Scheduled sync and conflicts (later)

Manual default; optional 15-min incremental checkpoint, on Git commit/session end, startup pull preview, idle network window; configurable frequency, metered network, battery, bandwidth and retention. No surprise uploads. Data changes only; use verified backup dedup or immutable delta records rather than full upload every hook. Track last successful sync and outstanding operations. SessionEnd not guaranteed, scheduler reconciles.

For later bidirectional memory sync, individual record changes carry operation UUID, entity version/base revision, origin device and stable logical sequence; order and dedup independent of wall-clock. Immutable conversations merge by unique event ID; divergent instruction/fact/project settings create explicit conflict with both versions; task and review state merge via valid transitions or user resolution; deletions tombstones prevent resurrection with retention/compaction rules. Project code merges via Git or versioned patch with conflict preview. Single-writer travel handoff preferred initial; concurrent device edits create recoverable forks, never overwrite. Device revocation removes future access where provider supports it; cannot erase previously downloaded data, rotate keys/credentials when necessary. Team trust/ACL/key distribution and shared working tree are separate later specs.

## 7. Acceptance

Create synthetic workspace with untracked, staged/unstaged changes, deletes, repo metadata, several memory records and pending spool. Backup while app running, interrupt upload and resume, verify no visible broken snapshot. Download on second isolated machine with first offline; compare manifest digests, dirty state, recall and environment gap report. Simulate wrong password, tampered object, missing file, revoked creds, disk full, two competing writers/CAS failure, concurrent edit, clock skew, symlink/path traversal, Windows/macOS case conflict, schema downgrade and deletion tombstone. Never publish data to real cloud until approved. Real R2/S3 smoke and cross-OS restore require documented test and provider terms; no claim of support based on mock alone.
