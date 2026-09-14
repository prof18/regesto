#!/usr/bin/env bash
# Antigravity PreInvocation launcher. Stdout must be one JSON object; a
# top-level "injectSteps" array of ephemeralMessage steps is added to the
# conversation trajectory before the model is called. Payload parsing and
# context framing are owned by `regesto hook antigravity-pre-invocation-v1`,
# not this shell launcher.
#
# `regesto-install` registers this automatically under the "regesto" hook
# name in ~/.gemini/config/hooks.json:
#
#   { "regesto": { "PreInvocation": [
#       { "type": "command",
#         "command": "<kb-root>/adapters/antigravity/hooks/pre-invocation.sh" }
#   ] } }
#
# That customization directory is shared by the Antigravity harness
# surfaces, so the CLI and the IDE read the same registration.
set -uo pipefail

# The instance is wherever this script lives, three levels up — the same
# self-locating rule the bin/ shims use, so no instance path is baked in.
# REGESTO_KB_ROOT still wins, for testing against another instance.
REGESTO_ROOT="${REGESTO_KB_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"

# Never fail a model call. A broken hook must degrade to an empty JSON
# object — injecting nothing — every layer of this design degrades
# gracefully (DESIGN §12), and a non-zero exit here would surface as an
# error on every single invocation. Capturing the output first means a
# future engine that writes partial stdout before failing can never
# concatenate it with the fallback object.
if output="$("$REGESTO_ROOT/bin/regesto-hook" antigravity-pre-invocation-v1 2>/dev/null)"; then
  printf '%s' "$output"
else
  printf '{}'
fi
