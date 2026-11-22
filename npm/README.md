# SessionKeep (npm package)

SessionKeep is a program that keeps developer memory for Claude Code on your own computer. This
package carries that program and sets it up. Node is used only to install it; the program
itself does not need Node to run.

It carries programs for Windows, macOS and Linux (x64 and arm64). MIT licensed; the licences of
the software compiled into the program are in `THIRD_PARTY_NOTICES.txt`.

## Install or update

    npm install -g sessionkeep

That one command:

1. puts the `sessionkeep` command on your PATH;
2. copies the program to its per-user folder and generates the Claude Code plugin there;
3. turns the plugin on in Claude Code by running Claude Code's own commands
   (`plugin marketplace add` and `plugin install`) for the account SessionKeep is set to.

Steps 2 and 3 are done by the package's install script. Recent npm versions print a notice
that it is "not yet covered by allowScripts" and run it anyway. To allow it without the notice:

    npm install -g sessionkeep --allow-scripts=sessionkeep

If your npm is set to block install scripts (`strict-allow-scripts` or `ignore-scripts`), the
command is installed but not set up: run `sessionkeep install --enable-plugin` once.

Restart Claude Code afterwards. npm hides what step 2 and 3 print; add `--foreground-scripts`
to see it, or run `sessionkeep doctor` afterwards. If setup could not finish, the next `sessionkeep`
command says why.

Nothing is remembered until you register a project (`sessionkeep project add . --capture
conversation`) or choose automatic mode (`sessionkeep setup --auto-add auto --level conversation`).

To install the command without any setup: set `SESSIONKEEP_SKIP_SETUP=1`, or use
`npm install -g --ignore-scripts`, and run `sessionkeep install` yourself later.

## Share projects through your own Cloudflare R2 (optional)

By default nothing leaves your computer. To exchange a project's memory with colleagues, you give
SessionKeep a storage bucket in a Cloudflare account you or your team own. There is no SessionKeep
server: each computer sends and receives bundles directly, and every bundle is encrypted on the
sending computer with a passphrase the people sharing that project agree on. Cloudflare bills R2
use to the account that owns the bucket.

### Where a shared project goes

Each shared project goes to one of these, chosen when you turn sharing on for it:

- **Personal storage**: your own bucket. Used unless you choose otherwise.
- **Shared storage**: a bucket you share with others, such as your company's.
- **A bucket of its own**: another bucket for this project only, such as one a client gave you.

Personal and shared storage are saved once on your computer, so sharing another project through
them needs only a name and a passphrase. A bucket of its own is kept with its project. So you can
keep your private projects in your own bucket, work on company projects through the company's
bucket, and give a client project the client's bucket, all at the same time.

### 1. In Cloudflare (once per bucket, by whoever owns the account)

1. In the Cloudflare dashboard, go to **Storage & databases > R2**. R2 has to be enabled
   (purchased) on the account before Cloudflare lets you create R2 tokens.
2. **Create bucket**, for example `sessionkeep-team`.
3. On the R2 **Overview** page, select **Manage** next to **API Tokens**, then
   **Create Account API token** (or **Create User API token**).
4. Permission: **Object Read & Write**, and **Apply to specific buckets only** with your bucket.
5. Create the token and copy the **Access Key ID** and **Secret Access Key** (the secret is shown
   only once). The confirmation page also shows the S3 endpoint,
   `https://<ACCOUNT_ID>.r2.cloudflarestorage.com`; it is on the R2 Overview page too.

For a bucket several people use, make one token per person. To take someone's access away,
delete their token. Colleagues do not need a Cloudflare account of their own to use your bucket.

Cloudflare's own instructions: https://developers.cloudflare.com/r2/api/tokens/

### 2. On each computer: from the dashboard

Run `sessionkeep dashboard` and open the address it prints. The menu on the left shows what
applies to all projects (Overview, Projects, Search everything, Instructions for all, Settings,
Health); opening a project switches it to that project's own pages.

1. **Settings**, tab **Cloud storage**, has two parts, **Personal storage** and
   **Shared storage**. Set up the ones you use: the storage address (the endpoint above), the
   bucket name, both keys and your name. When it says the bucket answered, the storage is saved.
   Nothing is sent yet. The keys are kept protected on this computer and never shown again.
2. Open the project (from **Projects** or the list in the menu), then its **Sharing** page.
   Choose where it is kept (personal, shared, or a bucket of its own, whose address and keys you
   then type on the same page), a shared project name (lower-case letters, digits and dashes)
   and a passphrase of at least 8 characters. The first person makes the name and passphrase up.
3. **Sync now** on the Sharing page receives your colleagues' work and then sends yours. Sync
   only happens when you press it. The page says which storage and bucket the project uses.

A colleague joins a project you share by doing the same on their computer with the **same
bucket** (their own keys to it), the **same shared project name** and the **same passphrase**.
Their own personal storage can be a different bucket; it is not touched.

### 2. On each computer: from the command line (alternative)

Keys and passphrases are read only from the environment, never from the command line. Run the
`connect` lines inside the project's folder (or add `--project ID_OR_PATH`).

macOS / Linux:

    read -rs SESSIONKEEP_TEAM_ACCESS_KEY; read -rs SESSIONKEEP_TEAM_SECRET_KEY
    export SESSIONKEEP_TEAM_ACCESS_KEY SESSIONKEEP_TEAM_SECRET_KEY
    sessionkeep team storage --endpoint https://<ACCOUNT_ID>.r2.cloudflarestorage.com --bucket my-bucket --from "Your Name"
    read -rs SESSIONKEEP_TEAM_PASSPHRASE; export SESSIONKEEP_TEAM_PASSPHRASE
    sessionkeep team connect --name my-project
    sessionkeep team sync          # shows what would be received and sent
    sessionkeep team sync --yes    # does it

Windows PowerShell:

    $env:SESSIONKEEP_TEAM_ACCESS_KEY = Read-Host "Access Key ID"
    $env:SESSIONKEEP_TEAM_SECRET_KEY = Read-Host "Secret Access Key"
    sessionkeep team storage --endpoint https://<ACCOUNT_ID>.r2.cloudflarestorage.com --bucket my-bucket --from "Your Name"
    $env:SESSIONKEEP_TEAM_PASSPHRASE = Read-Host "Passphrase"
    sessionkeep team connect --name my-project
    sessionkeep team sync
    sessionkeep team sync --yes

That uses the personal storage. For the others (set the keys in the environment first, as above):

    sessionkeep team storage --storage shared --endpoint URL --bucket company-bucket   # save the shared storage
    sessionkeep team connect --name company-app --storage shared                       # share through it
    sessionkeep team connect --name client-site --endpoint URL --bucket client-bucket  # a bucket of its own

After that the keys and passphrase are saved, so later syncs need only
`sessionkeep team sync --yes`. `sessionkeep team list` shows each storage and which projects use
it; `sessionkeep team` lists every form.

### Good to know

- What is sent for a shared project: its instructions, checkpoints and the text of its sessions
  (`sessionkeep team push --no-sessions` leaves the sessions out). Code is not sent; use Git for that.
- One bucket can hold many shared projects. Each has its own name and passphrase, and one
  project's passphrase cannot open another's. Someone with keys to the bucket can see that other
  projects' bundles exist (their names are hashed, and their sizes show), but cannot read them.
- The passphrase cannot be changed afterwards, and without it the bundles cannot be read. Keep it
  somewhere safe. To lock someone out of future work, delete their token and share again under
  a new shared name with a new passphrase. What they already received stays with them.
- **If you forget a passphrase:** a computer still connected to the project keeps syncing with
  the copy stored there, and can show it again: **Show passphrase** on the project's Sharing page (it asks
  first) or `sessionkeep team passphrase --yes`. A colleague who shares the project can also
  tell you. If nobody has it any more, nothing can open the bundles already sent: there is no
  reset. Share the project again under a new shared name with a new passphrase; the first sync
  sends everything again, and your own saved memory is never affected.
- Forgetting a saved storage leaves the projects already shared through it working; SessionKeep
  never deletes anything from a bucket.
- Cloudflare R2 is the service this has been used with. Other S3-compatible services may work
  (`--region` sets the region; R2 uses `auto`).

## Remove

    sessionkeep uninstall          (your data is kept unless you add --purge-data)
    npm uninstall -g sessionkeep

`sessionkeep uninstall` prints the Claude Code command that removes the plugin there.
