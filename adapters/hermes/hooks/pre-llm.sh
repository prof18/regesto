#!/usr/bin/env bash
# Hermes pre_llm_call launcher. The Go protocol handler emits exactly {} or
# {"context":"..."}; this launcher only locates the instance and
# guarantees an empty valid response if the engine is unavailable.
set -uo pipefail
REGESTO_ROOT="${REGESTO_KB_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
if output="$("$REGESTO_ROOT/bin/regesto-hook" hermes-pre-llm-v1 2>/dev/null)"; then
  printf '%s' "$output"
else
  printf '{}'
fi
