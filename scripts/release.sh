#!/usr/bin/env bash
set -euo pipefail

# Release script for Andamio CLI
# Usage: ./scripts/release.sh [version]
# Example: ./scripts/release.sh 0.2.0
#
# If no version given, bumps the patch version from the latest tag.

cd "$(git rev-parse --show-toplevel)"

# Get latest tag
LATEST_TAG=$(git tag --sort=-v:refname | head -n 1 2>/dev/null || echo "")

if [[ -n "${1:-}" ]]; then
  VERSION="$1"
elif [[ -z "$LATEST_TAG" ]]; then
  VERSION="0.1.0"
else
  # Auto-bump patch version
  CURRENT="${LATEST_TAG#v}"
  IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT"
  PATCH=$((PATCH + 1))
  VERSION="$MAJOR.$MINOR.$PATCH"
fi

TAG="v$VERSION"

echo "Preparing release: $TAG"
echo ""

# Preflight checks
echo "Preflight checks:"

# Clean working tree
if [[ -n "$(git status --porcelain)" ]]; then
  echo "  ✗ Working tree is not clean. Commit or stash changes first."
  git status --short
  exit 1
fi
echo "  ✓ Working tree clean"

# On main branch
BRANCH=$(git branch --show-current)
if [[ "$BRANCH" != "main" ]]; then
  echo "  ✗ Not on main branch (on: $BRANCH)"
  exit 1
fi
echo "  ✓ On main branch"

# Up to date with remote
git fetch origin main --quiet
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)
if [[ "$LOCAL" != "$REMOTE" ]]; then
  echo "  ✗ Local main differs from origin/main. Push or pull first."
  exit 1
fi
echo "  ✓ In sync with origin/main"

# Tag doesn't already exist
if git tag | grep -q "^${TAG}$"; then
  echo "  ✗ Tag $TAG already exists"
  exit 1
fi
echo "  ✓ Tag $TAG is available"

# CHANGELOG.md entry check — heading-match (not Unreleased-body inspection) per plan decision.
if [[ ! -f CHANGELOG.md ]]; then
  echo "  ✗ CHANGELOG.md missing at repo root"
  echo "    Create one before releasing (see https://keepachangelog.com/)."
  exit 1
fi
if grep -q "^## \[${VERSION}\]" CHANGELOG.md; then
  echo "  ✓ CHANGELOG entry found for $VERSION"
else
  echo "  ! No CHANGELOG entry matching '## [$VERSION]' found"
  echo "    Expected heading format: ## [$VERSION] - YYYY-MM-DD"
  echo "    Move content from '## [Unreleased]' into a new versioned heading,"
  echo "    or proceed if this release genuinely has no user-facing change."
  read -p "    Continue without a CHANGELOG entry for $VERSION? [y/N] " -n 1 -r
  echo ""
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "    Cancelled."
    exit 1
  fi
fi

# Build test
echo "  → Testing build..."
go build -ldflags "-X main.version=$VERSION" -o /dev/null ./cmd/andamio
echo "  ✓ Build succeeds"

echo ""
echo "Ready to release $TAG"
echo ""
echo "This will:"
echo "  1. Create and push tag $TAG"
echo "  2. GitHub Actions will build binaries for macOS/Linux/Windows"
echo "  3. Binaries published to GitHub Releases"
echo ""
read -p "Proceed? [y/N] " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Cancelled."
  exit 0
fi

git tag -a "$TAG" -m "Release $TAG"
git push origin "$TAG"

echo ""
echo "Tag $TAG pushed. GitHub Actions will handle the rest."
echo "Watch the release: https://github.com/Andamio-Platform/andamio-cli/actions"
echo "Binaries will appear at: https://github.com/Andamio-Platform/andamio-cli/releases/tag/$TAG"
