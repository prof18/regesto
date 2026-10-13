// pi extension launcher. pi auto-discovers this file from
// ~/.pi/agent/extensions/; `regesto-install` links it there instead of editing
// a settings file. Payload construction stays here, but parsing and context
// framing are owned by `regesto hook pi-before-agent-start-v1`, not this
// extension.
import { spawn } from "node:child_process"
import { realpathSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent"

// The instance is wherever this file really lives, three levels up — the same
// self-locating rule the bin/ shims use, so no instance path is baked in and
// the deployed symlink may be moved with the knowledge base. REGESTO_KB_ROOT
// still wins, for testing against another instance.
function kbRoot(): string {
  const override = process.env.REGESTO_KB_ROOT
  if (override) {
    return override
  }
  const self = realpathSync(fileURLToPath(import.meta.url))
  return path.resolve(path.dirname(self), "..", "..", "..")
}

// One injection per process: the first prompt's agent run carries the
// context, and every later turn in that process already sees it in history.
// Resuming a session starts a new process and injects again — the same
// semantic Claude Code's SessionStart has on --continue. Sessions without a
// stable id (pi --no-session) share one "ephemeral" key, so they get at most
// one injection per process.
function sessionKey(ctx: ExtensionContext): string {
  try {
    return ctx.sessionManager.getSessionId() ?? "ephemeral"
  } catch {
    return "ephemeral"
  }
}

// A fast failure leaves the session unmarked, so the next prompt retries — the
// launcher fails open with empty output, and a transient failure should not
// cost the whole session. A timeout is different: a wedged launcher would add
// the full timeout to every remaining prompt, so one timeout retires the
// session instead.
type LaunchResult = { ok: boolean; stdout: string; timedOut: boolean }

// launch runs the protocol launcher without blocking the host's event loop:
// extensions live inside the pi process, and a synchronous spawn would freeze
// every session for up to the timeout. It never rejects; every failure path
// resolves a failed result so the hook can degrade to injecting nothing.
function launch(payload: string): Promise<LaunchResult> {
  return new Promise((resolve) => {
    const fail = (timedOut: boolean): LaunchResult => ({ ok: false, stdout: "", timedOut })
    let child
    try {
      child = spawn(path.join(kbRoot(), "bin", "regesto-hook"), ["pi-before-agent-start-v1"], {
        stdio: ["pipe", "pipe", "ignore"],
      })
    } catch {
      resolve(fail(false))
      return
    }
    let stdout = ""
    let settled = false
    const finish = (result: LaunchResult) => {
      if (!settled) {
        settled = true
        clearTimeout(timer)
        resolve(result)
      }
    }
    const timer = setTimeout(() => {
      child.kill("SIGKILL")
      finish(fail(true))
    }, 10_000)
    child.stdout.setEncoding("utf8")
    child.stdout.on("data", (chunk: string) => {
      stdout += chunk
    })
    child.on("error", () => finish(fail(false)))
    child.on("close", (code: number | null) => finish({ ok: code === 0, stdout, timedOut: false }))
    child.stdin.on("error", () => {})
    child.stdin.end(payload)
  })
}

export default function regestoExtension(pi: ExtensionAPI) {
  const injected = new Set<string>()

  pi.on("before_agent_start", async (event, ctx) => {
    const sessionId = sessionKey(ctx)
    if (injected.has(sessionId)) {
      return
    }
    const cwd = event.systemPromptOptions?.cwd ?? ctx.cwd
    const result = await launch(JSON.stringify({ sessionId, cwd }))
    // Never fail a prompt. A broken launcher must degrade to injecting
    // nothing — every layer of this design degrades gracefully, and a thrown
    // error here would surface on every turn.
    if (!result.ok) {
      if (result.timedOut) {
        injected.add(sessionId)
      }
      return
    }
    const context = result.stdout.trim()
    if (!context) {
      return
    }
    injected.add(sessionId)
    // The message is persistent: stored in the session and sent to the LLM as
    // part of this agent run, without a visible transcript block (display:
    // false keeps the TUI clean; the session file still records it).
    return {
      message: {
        customType: "regesto-context",
        content: context,
        display: false,
      },
    }
  })
}
