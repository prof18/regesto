#!/usr/bin/env bash
# Claude Code SessionStart hook (PLAN 1.c). Stdout is added to the model's
# context before the first prompt — verified 2026-07-27 with a sentinel string
# in an isolated session (PLAN section 0 verify list, item 2).
#
# Install by adding to ~/.claude/settings.json:
#
#   "hooks": {
#     "SessionStart": [
#       { "hooks": [ { "type": "command",
#                      "command": "~/regesto-kb/adapters/claude/hooks/session-start.sh" } ] }
#     ]
#   }
#
# The hook receives a JSON payload on stdin including workspace.current_dir; we
# read cwd from it so the project resolves against the session's directory
# rather than wherever the hook happens to run.
set -uo pipefail

REGESTO_ROOT="${REGESTO_KB_ROOT:-$HOME/regesto-kb}"

payload="$(cat)"
dir=""
if command -v jq >/dev/null 2>&1; then
  dir="$(printf '%s' "$payload" | jq -r '.workspace.current_dir // .cwd // empty' 2>/dev/null)"
fi
[[ -z "$dir" || ! -d "$dir" ]] && dir="$PWD"

# Never fail a session start. A broken hook must degrade to injecting nothing —
# every layer of this design degrades gracefully (DECISION §12), and a
# non-zero exit here would surface as an error on every single session.
"$REGESTO_ROOT/bin/regesto-context" --dir "$dir" 2>/dev/null || exit 0
