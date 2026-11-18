#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

export REVTRAIL_RG="${REVTRAIL_RG:-$(command -v rg)}"
export REVTRAIL_GOPLS="${REVTRAIL_GOPLS:-$(command -v gopls)}"
test -x "$REVTRAIL_RG"
test -x "$REVTRAIL_GOPLS"
if [[ "${OS:-}" == "Windows_NT" ]]; then
  export REVTRAIL_RG="$(cygpath -m "$REVTRAIL_RG")"
  export REVTRAIL_GOPLS="$(cygpath -m "$REVTRAIL_GOPLS")"
fi

unformatted="$(gofmt -l app.go internal locales scripts)"
if [[ -n "$unformatted" ]]; then
  echo "gofmt -w these files:" >&2
  echo "$unformatted" >&2
  exit 1
fi

CGO_ENABLED=0 go test -count=1 -timeout=5m ./...
go tool mygo vet .
