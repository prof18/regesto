#!/usr/bin/env bash
# GitHub Copilot CLI SessionStart launcher. Stdout must be one JSON object;
# a top-level "additionalContext" string is injected into the conversation
# before the first turn. Payload parsing and context framing are owned by
# `regesto hook copilot-session-start-v1`, not this shell launcher.
#
# `regesto-install` registers this automatically in
# ~/.copilot/hooks/regesto.json:
#
#   { "hooks": { "SessionStart": [
#       { "type": "command",
#         "command": "<kb-root>/adapters/copilot/hooks/session-start.sh" }
#   ] } }
#
# The same file is also read by Copilot in VS Code (Agent Host).
set -uo pipefail

# The instance is wherever this script lives, three levels up — the same
# self-locating rule the bin/ shims use, so no instance path is baked in.
# REGESTO_KB_ROOT still wins, for testing against another instance.
REGESTO_ROOT="${REGESTO_KB_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"

# Never fail a session start. A broken hook must degrade to an empty JSON
# object — injecting nothing — every layer of this design degrades
# gracefully (DESIGN §12), and a non-zero exit here would surface as an
# error on every single session. Capturing the output first means a future
# engine that writes partial stdout before failing can never concatenate it
# with the fallback object.
if output="$("$REGESTO_ROOT/bin/regesto-hook" copilot-session-start-v1 2>/dev/null)"; then
  printf '%s' "$output"
else
  printf '{}'
fi
