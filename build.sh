#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

version="$(tr -d '[:space:]' < VERSION)"
numeric="${version%%-*}.0"
commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then commit="$commit-dirty"; fi
date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
repository="$( [ -f REPOSITORY ] && tr -d '[:space:]' < REPOSITORY || true )"
ldversion="-X sessiondesk/internal/buildinfo.Version=$version -X sessiondesk/internal/buildinfo.Commit=$commit -X sessiondesk/internal/buildinfo.Date=$date -X sessiondesk/internal/buildinfo.Repository=$repository"

npm --prefix frontend ci --no-audit --no-fund
npm --prefix frontend run build
go test ./...
go vet ./...
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64 --product-version "$numeric" --file-version "$numeric"
mkdir -p bin
go build -trimpath -ldflags "$ldversion" -o bin/session-desk .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui $ldversion" -o bin/session-desk.exe .
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $ldversion" -o bin/session-desk-worker ./cmd/session-desk-worker
node scripts/notices.mjs
cd bin
zip -q -r "session-desk-windows-amd64-v$version.zip" session-desk.exe session-desk-worker LICENSE.txt THIRD_PARTY_NOTICES.md licenses
if command -v makensis >/dev/null; then
  makensis -V2 -INPUTCHARSET UTF8 "-DVERSION=$version" "-DVERSION4=$numeric" "-DBIN=$PWD" "-DOUTFILE=$PWD/session-desk-$version-setup.exe" ${repository:+"-DWEBSITE=https://github.com/$repository"} ../installer/session-desk.nsi
fi
