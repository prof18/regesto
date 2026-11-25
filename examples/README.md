# Example facts

These facts describe an invented project, *aurora* (a small web service), with a
realistic controlled vocabulary, so a new instance can start populated rather than empty.
`regesto init --examples` copies `facts/` into `knowledge/facts/`.

The examples show the shape of a good fact and are not meant to be kept. Delete them once
you have written a few of your own; nothing depends on them.

Between them they demonstrate:

- all four claim types, with the prefixes `dec-`, `pref-`, `fact-`, and `pat-`;
- both scopes, `global` and `project:aurora`;
- a supersession pair on one `(subject, relation)`: `dec-http-port-8080` retires
  `dec-http-port-8000`, and the older file stays on disk with its body intact;
- `source: human` claims alongside agent-written ones, because lint does not let an
  agent-written claim cross that trust boundary on its own;
- a `**Why:**` line on every claim, so a reader a year later can still see the reason
  behind it.

They establish this vocabulary: `(git-commits, message-style)`, `(ci-runners,
architecture)`, `(http-server, port)`, `(auth, session-transport)`, and
`(database-migrations, direction)`. Aim for pairs like these. Each pair is narrow enough
that a future contradicting claim lands on the same pair, and broad enough that it does
not repeat the whole sentence.
