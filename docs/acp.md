# ACP

`kei acp` exposes the same logical session/runtime behavior used by the built-in CLI through an Agent Client Protocol adapter over stdin/stdout.

The adapter is intentionally thin. ACP is a frontend/wire contract, not the internal architecture of the agent loop.

Internally, an ACP protocol session ID maps to an active `internal/agent.Runtime`. That runtime owns a pointer to `internal/session.State`, which contains the logical session ID, workspace metadata, and transcript. ACP does not define a second session model.

## Start the server

```sh
./build/kei acp
```

The same connection-target flags as `kei run` are available:

```sh
./build/kei acp -provider claude -m fast
./build/kei acp -config ./path/to/config.json
```

The server reads newline-delimited JSON-RPC messages from stdin and writes newline-delimited JSON-RPC responses/notifications to stdout.

## Implemented methods

The current MVP handles:

### `initialize`

Returns protocol version `1`, agent information for `kei`, and advertises `loadSession: false`.

### `session/new`

Accepts a `cwd`, allocates an in-process session ID such as `kei-1`, and builds a fresh `session.State` plus `agent.Runtime` through the runtime factory.

The cwd becomes the state's workspace path. Workspace-local configuration/discovery and child-process working directories are therefore scoped to that logical ACP session rather than shared as one global process registry.

After runtime creation, `kei` sends an `available_commands_update` containing discovered slash commands.

### `session/prompt`

Collects text prompt blocks, passes the resulting text to `Runtime.Prompt`, forwards lifecycle/stream events as `session/update` notifications, and replies with `stopReason: "end_turn"`.

If the provider emitted text deltas, each delta is projected as an ACP `agent_message_chunk`. If no text delta was emitted, the completed runtime output is sent once as an `agent_message_chunk` after `Prompt` returns.

This fallback matters because provider transports currently differ in how incrementally they emit text.

### `session/cancel`

Cancels the active prompt context for the given protocol session when one exists. The cancellation flows through provider calls and `exec.CommandContext`-based child execution where the active context is used.

Unknown JSON-RPC methods with an ID receive `-32601 method not found`. Invalid/unknown session prompt requests use `-32602` style errors. Runtime-factory failures use `-32603`.

## Session updates

The ACP adapter translates generic runtime events rather than making the agent layer emit ACP objects directly.

Provider text deltas:

```text
Runtime.OnEvent("assistant_message_chunk", ...)
    -> session/update
       sessionUpdate = "agent_message_chunk"
```

Other current lifecycle events such as `tool_start`, `tool_end`, `command_start`, and `command_end` are projected through a `tool_call_update` wrapper containing the generic event kind and payload.

The exact ACP projection can evolve without changing the underlying runtime event names.

## Slash-command advertisement

A newly created runtime inspects its discovered command registry and sends:

```text
available_commands_update
```

Each command includes its qualified name and description. When a descriptor has `input_hint`, the ACP command entry also includes that hint.

For an extension command declared as `inspect` in extension `astrolabe`, the advertised command name is:

```text
astrolabe:inspect
```

The ACP client is responsible for presenting that as a slash-command UI as appropriate.

## Current limits

The ACP server is deliberately an MVP and currently has several narrow edges:

- session loading/resume is not implemented (`loadSession: false`);
- protocol session IDs map only to active in-process runtimes; there is no session store;
- permission/approval round-trips are not yet a robust ACP-native workflow;
- non-text prompt block types are ignored by `session/prompt`;
- child-process stdout/stderr are not incrementally streamed through ACP;
- generic tool/command lifecycle events are projected through a simple update wrapper rather than a richer stable ACP-specific model.

These are adapter limitations, not reasons to move ACP concepts into `internal/session` or `internal/agent`.

## Development rule

When adding an ACP feature, first ask whether it is:

1. a projection of logical state or runtime behavior that already exists; or
2. a genuinely missing core concept.

Prefer the first path. Keep ACP parsing, method names, and wire shapes in `internal/acp`; keep logical conversation state in `internal/session`/`internal/transcript`; keep provider-independent execution orchestration in `internal/agent`.
