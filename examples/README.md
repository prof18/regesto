# Example facts

An invented project — *aurora*, a small web service — with a realistic controlled
vocabulary, so a new instance can start populated rather than empty. `regesto init
--examples` copies `facts/` into `knowledge/facts/`.

They exist to show the shape of a good fact, not to be kept. Delete them once you have
written a few of your own; nothing depends on them.

Between them they demonstrate:

- all four types — `dec-`, `pref-`, `fact-`, `pat-`
- both scopes — `global`, and `project:aurora`
- a supersession pair on one `(subject, relation)`: `dec-http-port-8000` retired by
  `dec-http-port-8080`, the older one kept on disk with its body intact
- `source: human` alongside agent-written claims, which is the trust boundary lint
  refuses to cross on its own
- a `**Why:**` line on every claim, which is what makes a fact worth keeping a year later

The vocabulary they establish — `(git-commits, message-style)`, `(ci-runners,
architecture)`, `(http-server, port)`, `(auth, session-transport)`,
`(database-migrations, direction)` — is the kind to aim for: narrow enough that a future
contradicting claim lands on the same pair, broad enough that it is not the whole sentence.
