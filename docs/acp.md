# ACP

`kei acp` exposes the same `internal/agent.Session` used by the built-in CLI through an Agent Client Protocol adapter over stdin/stdout.

The adapter is intentionally thin. ACP is a frontend/wire contract, not the internal architecture of the agent loop.

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

Accepts a `cwd`, allocates an in-process session ID such as `kei-1`, and builds a fresh `agent.Session` through the session factory.

The cwd is important: workspace-local configuration/discovery and child-process working directories are scoped to that ACP session rather than shared as one global process registry.

After session creation, `kei` sends an `available_commands_update` containing discovered slash commands.

### `session/prompt`

Collects text prompt blocks, passes the resulting text to `Session.Prompt`, forwards lifecycle/stream events as `session/update` notifications, and replies with `stopReason: "end_turn"`.

If the provider emitted text deltas, each delta is projected as an ACP `agent_message_chunk`. If no text delta was emitted, the completed session output is sent once as an `agent_message_chunk` after `Prompt` returns.

This fallback matters because provider transports currently differ in how incrementally they emit text.

### `session/cancel`

Cancels the active prompt context for the given session when one exists. The cancellation flows through provider calls and `exec.CommandContext`-based child execution where the active context is used.

Unknown JSON-RPC methods with an ID receive `-32601 method not found`. Invalid/unknown session prompt requests use `-32602` style errors. Session-factory failures use `-32603`.

## Session updates

The ACP adapter translates generic session events rather than making the agent layer emit ACP objects directly.

Provider text deltas:

```text
Session.OnEvent("assistant_message_chunk", ...)
    -> session/update
       sessionUpdate = "agent_message_chunk"
```

Other current lifecycle events such as `tool_start`, `tool_end`, `command_start`, and `command_end` are projected through a `tool_call_update` wrapper containing the generic event kind and payload.

The exact ACP projection can evolve without changing the underlying session event names.

## Slash-command advertisement

A newly created session inspects its discovered command registry and sends:

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
- permission/approval round-trips are not yet a robust ACP-native workflow;
- non-text prompt block types are ignored by `session/prompt`;
- child-process stdout/stderr are not incrementally streamed through ACP;
- generic tool/command lifecycle events are projected through a simple update wrapper rather than a richer stable ACP-specific model;
- session IDs and session storage live in the ACP server process.

These are adapter limitations, not reasons to move ACP concepts into `internal/agent`.

## Development rule

When adding an ACP feature, first ask whether it is:

1. a capability or state that already exists in the session and only needs projection; or
2. a genuinely missing session/core concept.

Prefer the first path. Keep ACP parsing, method names, and wire shapes in `internal/acp`; keep provider-independent orchestration in `internal/agent`.
