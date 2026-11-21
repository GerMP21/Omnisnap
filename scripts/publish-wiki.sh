#!/usr/bin/env bash
# Mirror docs/internals/ into a clone of the GitHub Wiki and commit the result.
# The wiki then holds exactly the pages of this commit: changed pages are
# updated, removed pages are deleted, and nothing else is kept. No commit is
# made when the wiki already matches.
#
# Usage: scripts/publish-wiki.sh <wiki-clone-dir> [source-sha]
# The caller clones the wiki and pushes (.github/workflows/wiki.yml).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
wiki="${1:?usage: publish-wiki.sh <wiki-clone-dir> [source-sha]}"
sha="${2:-$(git -C "$root" rev-parse --short HEAD)}"
source="$root/docs/internals"

if ! git -C "$wiki" rev-parse --git-dir >/dev/null 2>&1; then
  echo "publish-wiki: $wiki is not a git clone of the wiki" >&2
  exit 1
fi
if ! ls "$source"/*.md >/dev/null 2>&1; then
  echo "publish-wiki: no pages in $source" >&2
  exit 1
fi

# Remove every tracked file, then copy the pages in.
git -C "$wiki" ls-files -z | (cd "$wiki" && xargs -0 -r rm -f)
cp "$source"/*.md "$wiki"/
git -C "$wiki" add -A

if git -C "$wiki" diff --cached --quiet; then
  echo "publish-wiki: the wiki already matches docs/internals at $sha"
  exit 0
fi
git -C "$wiki" -c user.name="github-actions[bot]" \
  -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
  commit -q -m "Publish docs/internals from $sha"
echo "publish-wiki: committed docs/internals at $sha"
git -C "$wiki" --no-pager show --stat --format= HEAD
