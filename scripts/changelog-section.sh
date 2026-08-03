#!/usr/bin/env bash
set -euo pipefail

# Extract one version's section from CHANGELOG.md.
#
# Usage: scripts/changelog-section.sh <version> [changelog-path]
# Example: scripts/changelog-section.sh 1.0.0 > release-notes.md
#
# CLAUDE.md states that CHANGELOG.md is the source of truth for user-facing
# release notes. Until 1.0 that was not actually true of the published GitHub
# release, whose body GoReleaser generated from commit subjects — which for a
# release like 1.0 would have led with "remove the learner and contributor
# command surface", exactly the framing issue #128 rules out. This script is
# what makes the documented claim true: the release workflow feeds its output
# to `goreleaser release --release-notes`.
#
# Prints the section body (everything after the `## [VERSION]` heading, up to
# the next `## [` heading) to stdout. Exits non-zero with a message on stderr
# if the section is missing or empty, so a release fails loudly rather than
# publishing blank notes.

VERSION="${1:?usage: changelog-section.sh <version> [changelog-path]}"
CHANGELOG="${2:-CHANGELOG.md}"

if [[ ! -f "$CHANGELOG" ]]; then
  echo "changelog-section: $CHANGELOG not found" >&2
  exit 1
fi

# awk over sed here because the section boundary is "the next H2 that opens a
# version heading", which needs state rather than a line pattern.
section=$(awk -v version="$VERSION" '
  # Opening heading for the requested version.
  $0 ~ ("^## \\[" version "\\]") { collecting = 1; next }
  # Any subsequent version heading closes the section.
  collecting && /^## \[/ { exit }
  collecting { print }
' "$CHANGELOG")

# Strip leading and trailing blank lines.
section=$(printf '%s\n' "$section" | sed -e '/./,$!d' | sed -e :a -e '/^\n*$/{$d;N;};/\n$/ba')

if [[ -z "$section" ]]; then
  echo "changelog-section: no content found under '## [$VERSION]' in $CHANGELOG" >&2
  echo "changelog-section: add a versioned section before releasing." >&2
  exit 1
fi

printf '%s\n' "$section"
