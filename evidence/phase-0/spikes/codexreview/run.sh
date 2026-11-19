#!/usr/bin/env bash
# Phase 0 spike P0-06, second approved run. Synthetic repository only.
# Usage: run.sh <work dir outside the user profile> <fixtures dir with schema.json and prompt.txt>
set -u
C="$1"; FX="$2"
rm -rf "$C"; mkdir -p "$C/repo" "$C/job"; cd "$C/repo" || exit 1
cat > billing.py <<'PY'
import sqlite3

def page_items(items, page, size):
    """Return page `page` (1-based) of items."""
    start = page * size          # seeded defect: should be (page - 1) * size
    return items[start:start + size]

def find_invoice(conn: sqlite3.Connection, customer: str):
    # seeded defect: SQL built by string concatenation
    return conn.execute("SELECT id, total FROM invoices WHERE customer = '" + customer + "'").fetchall()
PY
printf '# synthetic fixture\nRequirement R1: page_items(items, 1, 10) returns the first ten items.\nRequirement R2: find_invoice must be safe for untrusted customer strings.\n' > README.md
git init -q && git add -A && git -c user.name=fixture -c user.email=fixture@example.invalid commit -qm "synthetic fixture" 2>/dev/null
digest() { find . -type f -not -path "./.git/*" -print0 | sort -z | xargs -0 sha256sum | sha256sum | cut -c1-16; git status --porcelain | wc -l; git rev-parse --short HEAD; }
echo "before: $(digest | tr '\n' ' ')"
START=$(date +%s)
codex exec --sandbox read-only --json --ignore-user-config --output-schema "$(cygpath -w "$FX/schema.json")" \
  -o "$(cygpath -w "$C/job/result.json")" --ephemeral --ignore-rules -C "$(cygpath -w "$C/repo")" - \
  < "$FX/prompt.txt" > "$C/job/events.jsonl" 2> "$C/job/stderr.txt"
echo "exit=$? secs=$(( $(date +%s) - START ))"
echo "after:  $(digest | tr '\n' ' ')"
ls "$C/repo"
echo "--- result"; cat "$C/job/result.json" 2>/dev/null; echo
echo "--- commands"
python -I - "$(cygpath -w "$C/job/events.jsonl")" <<'PY'
import json, sys, collections
c = collections.Counter()
for l in open(sys.argv[1], encoding="utf-8", errors="replace"):
    try:
        e = json.loads(l)
    except Exception:
        c["unparsed"] += 1; continue
    it = e.get("item") if isinstance(e.get("item"), dict) else {}
    c[(e.get("type"), it.get("type"))] += 1
    if e.get("type") == "turn.completed": print("usage", e.get("usage"))
    if it.get("type") == "command_execution" and e.get("type") == "item.completed":
        print("cmd", (it.get("command") or "")[-120:].replace("\n", " "), "| exit", it.get("exit_code"), "|", (it.get("aggregated_output") or "")[:140].replace("\n", " "))
print(dict(c))
PY
