#!/usr/bin/env bash
# Local verification gate. Run from the repository root: bash scripts/check.sh
# Uses the project-local Go toolchain in .tools/go when present, otherwise the Go on PATH.
set -u
cd "$(dirname "$0")/.." || exit 1
fail=0
step() { printf '\n== %s\n' "$1"; }

if [ -x .tools/go/bin/go ] || [ -x .tools/go/bin/go.exe ]; then
  T="$(pwd)/.tools"
  export GOROOT="$T/go" GOPATH="$T/gopath" GOCACHE="$T/gocache" GOFLAGS=-modcacherw PATH="$T/go/bin:$PATH"
fi
export GOTOOLCHAIN=local
go version || { echo "FAIL: Go not found"; exit 1; }

step "private material must not be tracked or staged"
tracked=$(git ls-files -- _private .tools 2>/dev/null; git diff --cached --name-only -- _private .tools 2>/dev/null)
wouldadd=$(git add --dry-run -A 2>/dev/null | grep -E "'(_private|\.tools)/" || true)
if [ -n "$tracked$wouldadd" ]; then echo "FAIL: private or tool files reachable by git:"; echo "$tracked$wouldadd"; fail=1; else echo ok; fi

step "no personal paths or user names in files git would add"
user="${USERNAME:-${USER:-}}"
if [ -n "$user" ]; then
  hits=$(git ls-files -co --exclude-standard -z | xargs -0 grep -lI -e "Users[\\\\/]\+$user" -e "/home/$user" 2>/dev/null || true)
  if [ -n "$hits" ]; then echo "FAIL: personal path found in:"; echo "$hits"; fail=1; else echo ok; fi
else echo "skipped: no user name in environment"; fi

step "SQLite driver pin (ADR-0001)"
want_sqlite=v1.62.0; want_libc=v1.80.1
got_sqlite=$(go list -m -f '{{.Version}}' modernc.org/sqlite 2>/dev/null); got_libc=$(go list -m -f '{{.Version}}' modernc.org/libc 2>/dev/null)
if [ "$got_sqlite" = "$want_sqlite" ] && [ "$got_libc" = "$want_libc" ]; then echo "ok ($got_sqlite, libc $got_libc)"; else echo "FAIL: modernc.org/sqlite=$got_sqlite libc=$got_libc; want $want_sqlite and $want_libc"; fail=1; fi

step "gofmt"
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then echo "FAIL: not gofmt-clean:"; echo "$unformatted"; fail=1; else echo ok; fi

step "go vet"
CGO_ENABLED=0 go vet ./... || fail=1

step "go test"
CGO_ENABLED=0 go test -count=1 ./... || fail=1

step "go test -race"
if [ "$(CGO_ENABLED=1 go env CGO_ENABLED)" = "1" ] && command -v gcc >/dev/null 2>&1; then
  CGO_ENABLED=1 go test -race -count=1 ./... || fail=1
else
  echo "NOT RUN: the race detector needs cgo and a C compiler, which this machine does not have"
fi

step "build (CGO-free) and smoke"
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -o bin/sessionkeep.exe ./cmd/sessionkeep && ./bin/sessionkeep.exe version && ./bin/sessionkeep.exe doctor || fail=1

echo
if [ "$fail" = 0 ]; then echo "CHECK PASSED"; else echo "CHECK FAILED"; fi
exit "$fail"
