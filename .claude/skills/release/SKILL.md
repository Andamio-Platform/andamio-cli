# Release Andamio CLI

Cut a new release with preflight checks and automated binary publishing.

## Prerequisites

- On `main` branch with clean working tree
- All changes committed and pushed to origin
- GitHub Actions configured (`.github/workflows/release.yml`)

## Process

### 1. Pre-release Check

Before releasing, verify:

```bash
# Build succeeds
go build -o andamio ./cmd/andamio
./andamio --version

# Working tree is clean
git status

# On main and synced
git branch --show-current
git fetch origin main
git log --oneline origin/main..HEAD  # should be empty
```

### 2. Cut the Release

Run the release script:

```bash
./scripts/release.sh          # Auto-bumps patch version (0.1.0 → 0.1.1)
./scripts/release.sh 0.2.0    # Or specify a version
```

The script will:
1. Run preflight checks (clean tree, on main, synced, build passes)
2. Show what it will do and ask for confirmation
3. Create an annotated git tag
4. Push the tag to origin
5. GitHub Actions takes over: GoReleaser cross-compiles for 6 targets and publishes to GitHub Releases

### 3. Verify

After pushing the tag:

```bash
# Watch the GitHub Actions run
gh run list --workflow=release.yml --limit 1

# Once complete, check the release
gh release view v0.1.0
```

Binaries will be at: `https://github.com/Andamio-Platform/andamio-cli/releases/tag/v{VERSION}`

### 4. Post-release

- Update andamio-docs CLI installation page if the version reference changed
- Announce in relevant channels

## Versioning

- `v0.x.y` — Pre-1.0, breaking changes allowed between minor versions
- Patch bumps (`0.1.0` → `0.1.1`) for bug fixes and small additions
- Minor bumps (`0.1.0` → `0.2.0`) for new command groups or significant features
- Tags must be semver with `v` prefix: `v0.1.0`, `v0.2.0`

## Targets

GoReleaser produces binaries for:
- `darwin_arm64` (macOS Apple Silicon)
- `darwin_amd64` (macOS Intel)
- `linux_amd64`
- `linux_arm64`
- `windows_amd64`
- `windows_arm64`

Plus `checksums.txt` for verification.
