#!/bin/bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/publish-release.sh vX.Y.Z

Publish an already prepared release from dist/vX.Y.Z/, then trigger the
secret-free prof18/homebrew-tap updater. This creates external GitHub state.
EOF
}

die() {
  echo "publish: $*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  usage
  exit 0
fi

[ "$#" -eq 1 ] || {
  usage >&2
  exit 2
}

version="$1"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  die "version must look like v0.4.1"
fi

[ "$(uname -s)" = "Darwin" ] || die "release publication must run on macOS"

for command_name in awk cmp codesign gh git shasum tar; do
  require_command "$command_name"
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

[ -z "$(git status --porcelain --untracked-files=all)" ] || \
  die "working tree is not clean; publish only committed release sources"
[ "$(git branch --show-current)" = "main" ] || die "publish from the main branch"

release_repo="${REGESTO_GITHUB_REPOSITORY:-prof18/regesto}"
tap_repo="${REGESTO_HOMEBREW_TAP_REPOSITORY:-prof18/homebrew-tap}"
output_dir="$repo_root/dist/$version"
notes_file="$output_dir/release-notes.md"
checksums_file="$output_dir/checksums.txt"

[ -d "$output_dir" ] || die "missing $output_dir; run scripts/release.sh $version first"
[ -s "$notes_file" ] || die "missing release notes: $notes_file"
[ -s "$checksums_file" ] || die "missing checksums: $checksums_file"

expected_targets="darwin_arm64 darwin_amd64 linux_arm64 linux_amd64"
verify_root="$(mktemp -d "${TMPDIR:-/tmp}/regesto-publish.XXXXXX")"
cleanup() {
  rm -rf -- "$verify_root"
}
trap cleanup EXIT

for target in $expected_targets; do
  archive="$output_dir/regesto_${version}_${target}.tar.gz"
  [ -f "$archive" ] || die "missing release archive: $archive"
  [ "$(tar -tzf "$archive")" = "regesto" ] || \
    die "$archive must contain exactly one file named regesto"

  target_dir="$verify_root/$target"
  mkdir -p "$target_dir"
  tar -xzf "$archive" -C "$target_dir"
  if [[ "$target" == darwin_* ]]; then
    codesign --verify --strict --verbose=2 "$target_dir/regesto"
  fi
done

(
  cd "$output_dir"
  shasum -a 256 -c checksums.txt
)

current_notes="$verify_root/release-notes.md"
awk -v want="## ${version#v}" '
  $0 == want { found = 1; next }
  found && /^## / { exit }
  found { print }
' CHANGELOG.md > "$current_notes"
cmp -s "$notes_file" "$current_notes" || \
  die "CHANGELOG.md changed after preparation; prepare $version again"

gh auth status >/dev/null
git fetch origin main --tags

head_sha="$(git rev-parse HEAD)"
[ "$head_sha" = "$(git rev-parse origin/main)" ] || \
  die "local main is not the published origin/main; push or update it before releasing"

if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
  [ "$(git rev-list -n 1 "$version")" = "$head_sha" ] || \
    die "tag $version exists but does not point at the current commit"
fi

if gh release view "$version" --repo "$release_repo" >/dev/null 2>&1; then
  die "GitHub release $version already exists; releases are immutable"
fi

echo "Publishing $version from $head_sha"
gh release create "$version" \
  "$output_dir"/regesto_*.tar.gz \
  "$checksums_file" \
  --repo "$release_repo" \
  --target "$head_sha" \
  --title "$version" \
  --notes-file "$notes_file"

echo "Triggering the secret-free Homebrew tap updater"
if ! gh workflow run update-regesto.yml --repo "$tap_repo"; then
  echo "publish: $version is published, but the tap updater did not start" >&2
  echo "publish: retry with: gh workflow run update-regesto.yml --repo $tap_repo" >&2
  exit 1
fi

echo "Published $version and queued the Homebrew tap update."
