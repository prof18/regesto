// OpenCode plugin launcher. OpenCode auto-discovers this file from
// ~/.config/opencode/plugins/; `regesto-install` links it there instead of
// editing a settings file. Payload construction stays here, but parsing and
// context framing are owned by `regesto hook opencode-chat-message-v1`, not
// this plugin.
import { spawn } from "node:child_process"
import { realpathSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"
import type { Plugin } from "@opencode-ai/plugin"

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

// One injection per session: the first user message carries the context, and
// every later message in that session already sees it in history.
const injected = new Set<string>()

// A fast failure leaves the session unmarked, so the next message retries —
// the launcher fails open with empty output, and a transient failure should
// not cost the whole session. A timeout is different: a wedged launcher would
// add the full timeout to every remaining message, so one timeout retires
// the session instead.
type LaunchResult = { ok: boolean; stdout: string; timedOut: boolean }

// launch runs the protocol launcher without blocking the host's event loop:
// plugins live inside the OpenCode server process, and a synchronous spawn
// would freeze every session for up to the timeout. It never rejects; every
// failure path resolves a failed result so the hook can degrade to injecting
// nothing.
function launch(payload: string): Promise<LaunchResult> {
  return new Promise((resolve) => {
    const fail = (timedOut: boolean): LaunchResult => ({ ok: false, stdout: "", timedOut })
    let child
    try {
      child = spawn(path.join(kbRoot(), "bin", "regesto-hook"), ["opencode-chat-message-v1"], {
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

export default (async ({ directory, worktree }) => {
  return {
    "chat.message": async (input, output) => {
      if (injected.has(input.sessionID)) {
        return
      }
      const result = await launch(
        JSON.stringify({
          sessionID: input.sessionID,
          directory: directory,
          worktree: worktree,
        }),
      )
      // Never fail a message. A broken launcher must degrade to injecting
      // nothing — every layer of this design degrades gracefully, and a
      // thrown error here would surface on every turn.
      if (!result.ok) {
        if (result.timedOut) {
          injected.add(input.sessionID)
        }
        return
      }
      const context = result.stdout.trim()
      if (!context) {
        return
      }
      injected.add(input.sessionID)
      // OpenCode validates parts before persisting the user message; a bare
      // {type, text} object fails that validation and aborts the whole
      // message, so every persisted field is filled in here.
      output.parts.push({
        id: "prt" + crypto.randomUUID().replaceAll("-", ""),
        sessionID: input.sessionID,
        messageID: output.message.id,
        type: "text",
        text: context,
      })
    },
  }
}) satisfies Plugin
