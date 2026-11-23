#!/usr/bin/env bash
# Writes release notes for tag $1 to stdout.
# "What's new" comes from the bullet list in the tagged commit's message
# (wrapped continuation lines are joined), falling back to commit subjects
# since the previous tag. Download instructions follow.
set -euo pipefail

TAG="$1"
REPO="${GITHUB_REPOSITORY:-HamzaBaghdadi/quarry}"
PREV=$(git describe --tags --abbrev=0 "${TAG}^" 2>/dev/null || true)

bullets=$(git log -1 --format=%b "$TAG" | grep -v "Co-Authored-By" | awk '
  /^- / { if (b != "") print b; b = $0; next }
  /^  +[^ ]/ && b != "" { sub(/^ +/, " "); b = b $0; next }
  { if (b != "") print b; b = "" }
  END { if (b != "") print b }
')

if [ -z "$bullets" ] && [ -n "$PREV" ]; then
  bullets=$(git log --no-merges --format="- %s" "$PREV..$TAG")
fi
[ -z "$bullets" ] && bullets="- Maintenance release"

cat <<EOF
## What's new

$bullets

## Download

| Platform | File |
| --- | --- |
| Windows (installer) | \`quarry-windows-amd64-installer.exe\` |
| Windows (portable) | \`quarry-windows-amd64.exe\` |
| macOS (Apple Silicon & Intel) | \`Quarry-macos-universal.zip\` |
| Linux — Ubuntu, Debian, Mint, Pop!_OS | \`quarry-linux-amd64.deb\` (double-click to install) |
| Linux — Fedora | \`quarry-linux-amd64.rpm\` (double-click to install) |
| Linux — any distro, no install | \`quarry-linux-amd64.AppImage\` |

Quarry updates itself: when a new version is published you'll see **Update** in the title bar.

**macOS:** the app isn't notarized yet. After unzipping, run \`xattr -cr Quarry.app\` once (or right-click → Open) before first launch.
**Linux:** the .deb and .rpm install everything they need automatically. For the AppImage: right-click → Properties → allow executing as a program, then double-click.
EOF

if [ -n "$PREV" ]; then
  printf '\n**Full Changelog**: https://github.com/%s/compare/%s...%s\n' "$REPO" "$PREV" "$TAG"
fi
