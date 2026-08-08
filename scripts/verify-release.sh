#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
if [[ -z "$version" ]]; then
  echo "usage: scripts/verify-release.sh X.Y.Z" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

expected_origin="https://github.com/Robben-Media/gogcli"
origin="$(git remote get-url origin)"
origin="${origin%/}"
origin="${origin%.git}"
if [[ "$origin" != "$expected_origin" && "$origin" != "git@github.com:Robben-Media/gogcli" ]]; then
  echo "unexpected origin: $origin" >&2
  echo "expected the Robben Media fork: $expected_origin" >&2
  exit 2
fi

changelog="CHANGELOG.md"
if ! rg -q "^## ${version} - " "$changelog"; then
  echo "missing changelog section for $version" >&2
  exit 2
fi
if rg -q "^## ${version} - Unreleased" "$changelog"; then
  echo "changelog section still Unreleased for $version" >&2
  exit 2
fi

notes_file="$(mktemp -t gogcli-release-notes)"
trap 'rm -f "$notes_file"' EXIT

awk -v ver="$version" '
  $0 ~ "^## "ver" " {print "## "ver; in_section=1; next}
  in_section && /^## / {exit}
  in_section {print}
' "$changelog" | sed '/^$/d' > "$notes_file"

if [[ ! -s "$notes_file" ]]; then
  echo "release notes empty for $version" >&2
  exit 2
fi

release_body="$(gh release view "v$version" --repo Robben-Media/gogcli --json body -q .body)"
if [[ -z "$release_body" ]]; then
  echo "GitHub release notes empty for v$version" >&2
  exit 2
fi

assets_count="$(gh release view "v$version" --repo Robben-Media/gogcli --json assets -q '.assets | length')"
if [[ "$assets_count" -eq 0 ]]; then
  echo "no GitHub release assets for v$version" >&2
  exit 2
fi

release_run_id="$(gh run list --repo Robben-Media/gogcli -L 20 --workflow release.yml --json databaseId,conclusion,headBranch -q ".[] | select(.headBranch==\"v$version\") | select(.conclusion==\"success\") | .databaseId" | head -n1)"
if [[ -z "$release_run_id" ]]; then
  echo "release workflow not green for v$version" >&2
  exit 2
fi

ci_ok="$(gh run list --repo Robben-Media/gogcli -L 1 --workflow ci --branch main --json conclusion -q '.[0].conclusion')"
if [[ "$ci_ok" != "success" ]]; then
  echo "CI not green for main" >&2
  exit 2
fi

make ci
make build
./bin/gog --version

echo "Release v$version verified (Robben origin, CI, GitHub release notes/assets, local build)."
