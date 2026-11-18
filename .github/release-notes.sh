#!/usr/bin/env bash
#
# release-notes.sh prints CHANGELOG.md's notes for a version, from its
# "## [X.Y.Z] - YYYY-MM-DD" (or "## X.Y.Z") section, and fails unless they
# have both an "### English" and an "### 简体中文" part, each with a bullet.
# Run it before pushing a tag; the release workflow runs it too.
#
# Usage: release-notes.sh <vX.Y.Z | X.Y.Z>
set -euo pipefail

version="${1:?usage: release-notes.sh <vX.Y.Z>}"
version="${version#v}"
cd "$(dirname "$0")/.."

notes="$(awk -v v="$version" '/^## /{h = $2; gsub(/^\[|\].*$/, "", h); p = (h == v); next} p' CHANGELOG.md)"
if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
	echo "CHANGELOG.md has no ## [$version] section" >&2
	exit 1
fi
for lang in English 简体中文; do
	part="$(printf '%s\n' "$notes" | awk -v l="### $lang" '/^### /{p = ($0 == l); next} p')"
	if ! printf '%s\n' "$part" | grep -qE '^[[:space:]]*[-*] [^[:space:]]'; then
		echo "CHANGELOG.md's $version notes need an \"### $lang\" part with at least one bullet" >&2
		exit 1
	fi
done
printf '%s\n' "$notes"
