#!/usr/bin/env bash
# Writes the licence notices of everything compiled into the given SessionKeep programs:
# the Go standard library and each module the programs list (`go version -m`). The licence
# files are copied word for word from the module cache, so run it with the same Go setup that
# built the programs (scripts/npm-pack.sh does).
#
#   bash scripts/notices.sh OUT_FILE PROGRAM [PROGRAM ...]
set -eu
out="${1:?usage: bash scripts/notices.sh OUT_FILE PROGRAM [PROGRAM ...]}"
shift
[ "$#" -gt 0 ] || { echo "no programs given" >&2; exit 2; }
modcache="$(go env GOMODCACHE)"
goroot="$(go env GOROOT)"

# Module paths in the cache write each capital letter as "!" and the lower-case letter.
escape() { printf '%s' "$1" | sed 's/[A-Z]/!&/g' | tr 'A-Z' 'a-z'; }

mods=$(for p in "$@"; do go version -m "$p"; done | awk '$1 == "dep" { print $2 "@" $3 }' | sort -u)

{
  echo "SessionKeep includes the software listed below. Each part is under its own licence,"
  echo "reproduced here as its authors published it."
  echo
  echo "================================================================================"
  echo "Go standard library and runtime ($(go env GOVERSION))"
  echo "================================================================================"
  cat "$goroot/LICENSE"
  for m in $mods; do
    dir="$modcache/$(escape "${m%@*}")@${m#*@}"
    files=$(ls "$dir" 2>/dev/null | grep -i -E '^(licen[cs]e|copying|notice|patents)' || true)
    if [ -z "$files" ]; then echo "no licence file found for $m in $dir" >&2; exit 1; fi
    for f in $files; do
      echo
      echo "================================================================================"
      echo "$m - $f"
      echo "================================================================================"
      cat "$dir/$f"
    done
  done
} > "$out"
echo "notices for $(printf '%s\n' "$mods" | wc -l | tr -d ' ') modules written to $out"
