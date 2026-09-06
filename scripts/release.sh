#!/bin/bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/release.sh vX.Y.Z

Build, sign, notarize, test, and package a Regesto release on macOS.
The finished artifacts are written to dist/vX.Y.Z/. Nothing is published.

Set REGESTO_SIGNING_IDENTITY when more than one Developer ID Application
identity is installed and the script cannot choose unambiguously.
EOF
}

die() {
  echo "release: $*" >&2
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

[ "$(uname -s)" = "Darwin" ] || die "release preparation must run on macOS"

for command_name in asc awk cmp codesign ditto git go gofmt grep jq python3 security shasum tar; do
  require_command "$command_name"
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

[ -z "$(git status --porcelain --untracked-files=all)" ] || \
  die "working tree is not clean; commit or remove local changes first"

if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
  die "tag $version already exists; releases are immutable"
fi

output_dir="$repo_root/dist/$version"
[ ! -e "$output_dir" ] || die "$output_dir already exists; inspect or move it before retrying"

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/regesto-release.XXXXXX")"
cleanup() {
  rm -rf -- "$work_dir"
}
trap cleanup EXIT

artifact_dir="$work_dir/artifacts"
build_dir="$work_dir/build"
notary_dir="$work_dir/notary"
mkdir -p "$artifact_dir" "$build_dir" "$notary_dir"

notes_file="$artifact_dir/release-notes.md"
awk -v want="## ${version#v}" '
  $0 == want { found = 1; next }
  found && /^## / { exit }
  found { print }
' CHANGELOG.md > "$notes_file"
[ -s "$notes_file" ] || die "CHANGELOG.md has no ## ${version#v} release section"
git rev-parse HEAD > "$artifact_dir/source-commit.txt"

identity="${REGESTO_SIGNING_IDENTITY:-}"
if [ -z "$identity" ]; then
  identities="$(security find-identity -v -p codesigning | sed -n 's/.*"\(Developer ID Application:.*\)"/\1/p')"
  identity_count="$(printf '%s\n' "$identities" | sed '/^$/d' | wc -l | tr -d ' ')"
  [ "$identity_count" -eq 1 ] || \
    die "found $identity_count Developer ID Application identities; set REGESTO_SIGNING_IDENTITY explicitly"
  identity="$identities"
fi

security find-identity -v -p codesigning | grep -Fq "\"$identity\"" || \
  die "signing identity is not valid or has no accessible private key: $identity"
asc auth status >/dev/null || die "asc authentication is not available; run asc auth doctor outside a sandbox"

echo "release $version"
echo "identity $identity"
echo
echo "==> formatting"
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "release: files are not gofmt'd:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> vet"
go vet ./...

echo "==> tests"
go test ./...

ldflags="-s -w -X github.com/prof18/regesto/internal/version.stamped=$version"
targets="darwin/arm64 darwin/amd64 linux/arm64 linux/amd64"

for target in $targets; do
  os="${target%/*}"
  arch="${target#*/}"
  target_dir="$build_dir/${os}_${arch}"
  binary="$target_dir/regesto"
  mkdir -p "$target_dir"

  echo "==> build $os/$arch"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "$ldflags" -o "$binary" ./cmd/regesto

  if [ "$os" = "darwin" ]; then
    echo "==> sign $os/$arch"
    codesign --force --timestamp --options runtime \
      --identifier com.prof18.regesto \
      --sign "$identity" \
      "$binary"
    codesign --verify --strict --verbose=2 "$binary"

    notary_zip="$notary_dir/regesto_${version}_${os}_${arch}.zip"
    notary_result="$notary_dir/regesto_${version}_${os}_${arch}.json"
    ditto -c -k --keepParent "$binary" "$notary_zip"

    echo "==> notarize $os/$arch"
    if ! asc notarization submit \
      --file "$notary_zip" \
      --wait \
      --timeout 1h \
      --output json > "$notary_result"; then
      cat "$notary_result" >&2
      die "notarization submission failed for $os/$arch"
    fi
    cat "$notary_result"

    notary_status="$(jq -r '.data.attributes.status // .status // empty' "$notary_result")"
    notary_id="$(jq -r '.data.id // .id // empty' "$notary_result")"
    if [ "$notary_status" != "Accepted" ]; then
      if [ -n "$notary_id" ]; then
        asc notarization log --id "$notary_id" || true
      fi
      die "notarization returned ${notary_status:-an unknown status} for $os/$arch"
    fi
    cp "$notary_result" "$artifact_dir/notarization_${os}_${arch}.json"

    # spctl's execute assessment is for app-like bundles and rejects a valid,
    # notarized bare CLI with "does not seem to be an app". The embedded
    # Developer ID signature plus Apple's Accepted response are the relevant
    # gates for this artifact type.
  fi

  archive="$artifact_dir/regesto_${version}_${os}_${arch}.tar.gz"
  COPYFILE_DISABLE=1 tar -czf "$archive" -C "$target_dir" regesto
  [ "$(tar -tzf "$archive")" = "regesto" ] || \
    die "$archive must contain exactly one file named regesto"
done

host_arch="$(uname -m)"
case "$host_arch" in
  arm64) native_arch="arm64" ;;
  x86_64) native_arch="amd64" ;;
  *) die "unsupported local architecture: $host_arch" ;;
esac

native_binary="$build_dir/darwin_${native_arch}/regesto"
[ "$("$native_binary" version)" = "regesto $version" ] || \
  die "native release binary reports the wrong version"

smoke_root="$work_dir/smoke"
mkdir -p "$smoke_root/bin" "$smoke_root/home/.claude/projects/release/memory" "$smoke_root/home/.codex/memories"
cp "$native_binary" "$smoke_root/bin/regesto"

printf '%s\n' 'claude release preface' > "$smoke_root/home/.claude/CLAUDE.md"
printf '%s\n' 'codex release preface' > "$smoke_root/home/.codex/AGENTS.md"

export PATH="$smoke_root/bin:$PATH"
export HOME="$smoke_root/home"

"$native_binary" init --dir "$smoke_root/kb" --machine release --examples
"$smoke_root/kb/bin/regesto-index"
"$smoke_root/kb/bin/regesto-search" --scope aurora | grep -q dec-http-port-8080

cp "$HOME/.claude/CLAUDE.md" "$smoke_root/claude-before.md"
cp "$HOME/.codex/AGENTS.md" "$smoke_root/codex-before.md"
regesto --config "$smoke_root/kb/config.toml" install --dry-run --json > "$smoke_root/install-dry.json"
cmp "$HOME/.claude/CLAUDE.md" "$smoke_root/claude-before.md"
cmp "$HOME/.codex/AGENTS.md" "$smoke_root/codex-before.md"
[ ! -e "$HOME/.claude/settings.json" ]
[ ! -e "$HOME/.claude/skills/regesto-search" ]
[ ! -e "$smoke_root/kb/.state/integrations" ]

regesto --config "$smoke_root/kb/config.toml" install --json > "$smoke_root/install.json"
regesto --config "$smoke_root/kb/config.toml" install --dry-run --json > "$smoke_root/install-current.json"
regesto --config "$smoke_root/kb/config.toml" doctor --json > "$smoke_root/doctor.json"
regesto --config "$smoke_root/kb/config.toml" doctor --json > "$smoke_root/doctor-repeat.json"
cmp "$smoke_root/doctor.json" "$smoke_root/doctor-repeat.json"

printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"release-smoke","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  '{"jsonrpc":"2.0","id":3,"method":"resources/list","params":{}}' \
  '{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"regesto://index"}}' \
  | regesto --config "$smoke_root/kb/config.toml" mcp > "$smoke_root/mcp.jsonl"

python3 - "$smoke_root/install-dry.json" "$smoke_root/install.json" \
  "$smoke_root/install-current.json" "$smoke_root/doctor.json" "$smoke_root/mcp.jsonl" <<'PY'
import json
import sys

dry, applied, current, doctor, transcript = sys.argv[1:]
with open(dry, encoding="utf-8") as stream:
    dry_report = json.load(stream)
assert dry_report["dry_run"] is True and "result" not in dry_report
with open(applied, encoding="utf-8") as stream:
    applied_report = json.load(stream)
assert applied_report["result"]["applied"] > 0
with open(current, encoding="utf-8") as stream:
    current_report = json.load(stream)
assert all(item["action"] in {"current", "skip", "manual"} for item in current_report["plan"]["items"])
with open(doctor, encoding="utf-8") as stream:
    doctor_report = json.load(stream)
configured = {item["id"]: item for item in doctor_report["integrations"] if item["configured"]}
assert set(configured) == {"claude", "codex"}
for item in configured.values():
    assert item["status"] == "ok"
    assert item["capabilities"]["skills"]["status"] == "ok"
    assert item["capabilities"]["instructions"]["status"] == "ok"
    assert item["capabilities"]["trust"]["status"] == "ok"
    assert all(artifact["action"] == "current" for artifact in item["artifacts"])
assert configured["claude"]["capabilities"]["hooks"][0]["status"] == "ok"
assert configured["codex"]["capabilities"]["hooks"][0]["status"] == "unsupported"
assert all(check["status"] == "ok" for check in doctor_report["checks"])
with open(transcript, encoding="utf-8") as stream:
    messages = [json.loads(line) for line in stream]
assert [message["id"] for message in messages] == [1, 2, 3, 4]
assert all(message["jsonrpc"] == "2.0" and "result" in message for message in messages)
assert messages[0]["result"]["protocolVersion"] == "2025-06-18"
assert [tool["name"] for tool in messages[1]["result"]["tools"]] == [
    "regesto_search", "regesto_get_fact", "regesto_resolve_project", "regesto_write_fact"
]
assert messages[2]["result"]["resources"][0]["uri"] == "regesto://index"
assert "# INDEX" in messages[3]["result"]["contents"][0]["text"]
PY

(
  cd "$artifact_dir"
  shasum -a 256 regesto_*.tar.gz > checksums.txt
  shasum -a 256 -c checksums.txt
)

mkdir -p "$repo_root/dist"
mv "$artifact_dir" "$output_dir"

echo
echo "Prepared $version in $output_dir"
echo "Nothing was published. Inspect the archives and release notes, then run:"
echo "  scripts/publish-release.sh $version"
