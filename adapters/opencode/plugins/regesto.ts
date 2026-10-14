// OpenCode plugin launcher. OpenCode auto-discovers this file from
// ~/.config/opencode/plugins/; `regesto-install` links it there instead of
// editing a settings file. Payload construction stays here, but parsing and
// context framing are owned by `regesto hook opencode-chat-message-v1`, not
// this plugin.
//
// One file serves both host generations: OpenCode 2 reads the default
// export's id and setup() and ignores server(); OpenCode 1 (1.18.29+) calls
// server() for its hook map. Neither generation gets a host-package import —
// the linked launcher must load standalone, with no node_modules beside it,
// so every host type below is structural.
import { spawn } from "node:child_process"
import { realpathSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

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

// inject runs the launcher for a session and returns the context to inject,
// or "" when nothing should be injected. Both host generations share it so
// the dedup and fail-open rules stay identical. Never fail a message on a
// broken launcher — every layer of this design degrades gracefully, and a
// thrown error would surface on every turn.
async function inject(sessionID: string, directory: string | undefined, worktree: string | undefined): Promise<string> {
  const result = await launch(
    JSON.stringify({
      sessionID: sessionID,
      directory: directory,
      worktree: worktree,
    }),
  )
  if (!result.ok) {
    if (result.timedOut) {
      injected.add(sessionID)
    }
    return ""
  }
  const context = result.stdout.trim()
  if (context) {
    injected.add(sessionID)
  }
  return context
}

// --- OpenCode 2 ---
//
// The prompt-admission event carries no location, and the setup context's
// location is the plugin instance's, not the session's — a global plugin
// loads once per location. The session record is the authoritative source
// for where this session runs; the instance location is a fail-open
// fallback.
type PromptEvent = {
  readonly sessionID: string
  prompt: { text: string }
}
type PluginContext = {
  location: { directory?: string }
  session: {
    get(query: { sessionID: string }): Promise<{ location?: { directory?: string } } | undefined>
    hook(name: "prompt", callback: (event: PromptEvent) => Promise<void> | void): Promise<unknown>
  }
}

async function sessionDirectory(ctx: PluginContext, sessionID: string): Promise<string | undefined> {
  try {
    const dir = (await ctx.session.get({ sessionID }))?.location?.directory
    if (dir) {
      return dir
    }
  } catch {
    // Fall back to the instance location below.
  }
  return ctx.location.directory
}

// --- OpenCode 1 ---
type ChatMessageInput = { sessionID: string }
type ChatMessageOutput = {
  message: { id: string }
  parts: { id: string; sessionID: string; messageID: string; type: string; text: string }[]
}
type ServerInput = { directory?: string; worktree?: string }

export default {
  id: "regesto",

  // OpenCode 2: intercept prompt admission. Edits to the draft become the
  // canonical persisted user input, so appending here keeps the same
  // first-message injection server() performs — the context rides the user's
  // message and later turns already see it in history. Appending never
  // disturbs attachment mention offsets.
  async setup(ctx: PluginContext) {
    await ctx.session.hook("prompt", async (event) => {
      // OpenCode 2 does not admit a prompt whose preparation fails, so an
      // unexpected error here would block the message outright. Swallow it
      // instead: a broken launcher must degrade to injecting nothing, never
      // to failing the turn.
      try {
        if (injected.has(event.sessionID)) {
          return
        }
        const directory = await sessionDirectory(ctx, event.sessionID)
        if (!directory) {
          // Without a location the launcher cannot scope the context; treat it
          // like a fast failure and retry on the next message.
          return
        }
        const context = await inject(event.sessionID, directory, directory)
        if (context && event.prompt) {
          event.prompt.text = (event.prompt.text ?? "") + "\n\n" + context
        }
      } catch {
        // Inject nothing and leave the session unmarked: the next message
        // retries, same as a fast launcher failure.
      }
    })
  },

  // OpenCode 1 (1.18.29+): the chat.message hook appends a context part to
  // the first user message of each session. The directory and worktree are
  // captured when the host loads the plugin, as before.
  async server({ directory, worktree }: ServerInput = {}) {
    return {
      "chat.message": async (input: ChatMessageInput, output: ChatMessageOutput) => {
        if (injected.has(input.sessionID)) {
          return
        }
        const context = await inject(input.sessionID, directory, worktree)
        if (!context) {
          return
        }
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
  },
}
