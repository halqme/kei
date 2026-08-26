# Documentation

`kei` keeps the root README small. This directory is the detailed reference for the runtime, its public seams, and its development model.

## Start here

If you want to use `kei`:

- [Connect](connect.md) — authenticate a provider and start a session.
- [Configuration](configuration.md) — provider targets, model aliases, extension roots, controls, and config lookup.
- [Sessions](session.md) — what happens to prompts, slash commands, tool calls, controls, and streamed output.
- [ACP](acp.md) — expose a session to an ACP client.

If you want to extend `kei`:

- [Extensions](extension/index.md) — layout, search roots, precedence, namespacing, and executable resolution.
- [Tools](extension/tool.md) — `tools.json` schema and execution semantics.
- [Slash commands](extension/slash-command.md) — `commands.json` schema and direct human invocation.
- [Controls](extension/control.md) — generic policy processes and the current control protocol.

If you want to change `kei` itself:

- [Architecture](architecture.md) — what belongs in the harness and what should stay outside it.
- [Development](development.md) — package map, test strategy, verification, and common cross-cutting changes.
- [`AGENTS.md`](../AGENTS.md) — concise repository workflow for coding agents.

## Vocabulary

A few terms recur throughout the documentation:

**Harness / control plane** is the Go process. It coordinates provider calls, session state, discovery, routing, process execution, controls, and frontend adapters.

**Connection target** is one named entry in the ordered `providers` list. It combines a provider type with model/API configuration.

**Provider** is the model API boundary implementing a model turn.

**Extension** is a namespace and distribution unit containing explicit declarations and, optionally, executable files.

**Tool** is a model-facing operation. The canonical identity is `<extension>.<tool>`.

**Slash command** is a human-facing operation. The canonical identity is `<extension>:<command>` and users invoke it as `/<extension>:<command>`.

**Skill** is guidance for how to work with capabilities. It is not an executable capability itself.

**Control** is an external policy hook that can alter or stop model/tool execution.

**Frontend** is the user/client transport around a session, currently the built-in REPL or ACP.

## Stability

`kei` is pre-release. Configuration and internal interfaces can still change, but extension declaration formats, names, discovery semantics, and execution behavior should be treated as user-maintained contracts: changes to them need tests, docs, and explicit migration intent.
