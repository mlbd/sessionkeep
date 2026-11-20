#!/usr/bin/env bash
# Builds the npm package file. Nothing is published; the result is a local file.
#
#   bash scripts/npm-pack.sh VERSION            program for this computer only
#   bash scripts/npm-pack.sh VERSION all        programs for Windows, macOS and Linux (x64 and arm64)
#   bash scripts/npm-pack.sh VERSION linux/amd64 darwin/arm64 ...
#
# Result: dist/sessionkeep-VERSION.tgz. Install it with: npm install -g dist/sessionkeep-VERSION.tgz
set -eu
cd "$(dirname "$0")/.."
version="${1:?usage: bash scripts/npm-pack.sh VERSION [all | os/arch ...]}"
shift
case "$version" in *[!0-9A-Za-z.+-]*|"") echo "not a version: $version" >&2; exit 2;; esac

if [ -x .tools/go/bin/go ] || [ -x .tools/go/bin/go.exe ]; then
  T="$(pwd)/.tools"
  export GOROOT="$T/go" GOPATH="$T/gopath" GOCACHE="$T/gocache" GOFLAGS=-modcacherw PATH="$T/go/bin:$PATH"
fi
export GOTOOLCHAIN=local CGO_ENABLED=0

if [ "$#" = 0 ]; then set -- "$(go env GOHOSTOS)/$(go env GOHOSTARCH)"; fi
if [ "$1" = all ]; then set -- windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; fi

stage=dist/npm/sessionkeep
rm -rf dist/npm
mkdir -p "$stage"
cp -r npm/bin npm/lib npm/postinstall.js npm/README.md "$stage/"
for t in "$@"; do
  os="${t%/*}"; arch="${t#*/}"; name=sessionkeep
  [ "$os" = windows ] && name=sessionkeep.exe
  mkdir -p "$stage/vendor/$os-$arch"
  GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X sessionkeep/internal/buildinfo.Version=$version" -o "$stage/vendor/$os-$arch/$name" ./cmd/sessionkeep
  echo "built $os-$arch"
done
# The version is the only thing that changes in package.json.
sed "s/\"version\": \"0.0.0-dev\"/\"version\": \"$version\"/" npm/package.json > "$stage/package.json"
grep -q "\"version\": \"$version\"" "$stage/package.json"
(cd "$stage" && npm pack --silent --pack-destination ../.. >/dev/null)
ls -l "dist/sessionkeep-$version.tgz"
