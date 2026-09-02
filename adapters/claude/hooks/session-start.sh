#!/usr/bin/env bash
# Claude Code SessionStart launcher. Stdout is added to the
# model's context before the first prompt. Payload parsing and context framing
# are owned by `regesto hook claude-session-start-v1`, not this shell launcher.
#
# `regesto-install` registers this automatically. By hand, add to
# ~/.claude/settings.json:
#
#   "hooks": {
#     "SessionStart": [
#       { "hooks": [ { "type": "command",
#                      "command": "<kb-root>/adapters/claude/hooks/session-start.sh" } ] }
#     ]
#   }
#
set -uo pipefail

# The instance is wherever this script lives, three levels up — the same
# self-locating rule the bin/ shims use, so no instance path is baked in.
# REGESTO_KB_ROOT still wins, for testing against another instance.
REGESTO_ROOT="${REGESTO_KB_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"

# Never fail a session start. A broken hook must degrade to injecting nothing —
# every layer of this design degrades gracefully (DESIGN §12), and a
# non-zero exit here would surface as an error on every single session.
"$REGESTO_ROOT/bin/regesto-hook" claude-session-start-v1 2>/dev/null || exit 0
