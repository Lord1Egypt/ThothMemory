#!/usr/bin/env bash
# Resume only drafts; never replace a differing asset or a published release.
set -euo pipefail
tag=${1:?tag}; out=${2:-dist}
if state=$(gh release view "$tag" --json isDraft --jq .isDraft 2>/dev/null); then
  [[ $state == true ]] || { echo 'Published versions are immutable' >&2; exit 1; }
  existing=$(mktemp -d); trap 'rm -rf "$existing"' EXIT
  count=$(gh release view "$tag" --json assets --jq '.assets | length')
  if [[ $count -gt 0 ]]; then gh release download "$tag" --dir "$existing"; fi
  for asset in "$out"/*; do
    name=${asset##*/}
    if [[ -f $existing/$name ]]; then cmp "$asset" "$existing/$name"; else gh release upload "$tag" "$asset"; fi
  done
else
  gh release create "$tag" "$out"/* --verify-tag --draft --title "ThothMemory $tag — Full, Mini & UltraLite" --notes-file docs/RELEASE_0.2.1.md
fi
uploaded=$(gh release view "$tag" --json assets --jq '.assets[].name')
for asset in "$out"/thothmemory-* "$out"/thothmini.sh "$out"/SHA256SUMS "$out"/PLATFORMS.md "$out"/RELEASE_NOTES.md; do
  printf '%s\n' "$uploaded" | grep -Fxq "${asset##*/}"
done
