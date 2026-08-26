# kei / 継

`kei` is a small Unix-native harness for coding agents.

A coding agent does not need to be a monolith. Models already speak structured tool calls; Unix already knows how to compose programs; editors and terminals already know how to be interfaces. `kei` is the thin layer that joins those pieces without trying to replace them.

```text
                 ┌──────────── frontend ────────────┐
                 │           REPL / ACP             │
                 └────────────────┬─────────────────┘
                                  │
                            agent session
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
                 provider                   controls
                    │                           │
                   model                       │
                    │                           │
                 tool call ────────> ordinary process
```

The interesting boundary in `kei` is the process boundary. A tool can be `rg`, `git`, a Swift executable, a Haskell program, an LSP bridge, or something written specifically for the agent. `kei` only needs an explicit descriptor that says what the capability is and how to invoke it.

That makes extension boring in a useful way: files, JSON, processes, stdin/stdout, exit status, signals, cwd, and the package manager you already use.

## What kei owns

`kei` owns the control plane: provider communication, session state, discovery and routing, child-process supervision, policy hooks, and frontend adapters.

It deliberately does **not** try to own every capability around that control plane. There is no in-process plugin runtime, no kei-specific package manager, and no giant custom TUI. Higher-level modes such as Plan or YOLO are expected to emerge from generic controls, prompts, visible tools, and frontend behavior rather than branches hard-coded into the agent loop.

## Extensions are declarations, not plugins

An extension is a namespace and distribution unit:

```text
.kei/extensions/astrolabe/
├── tools.json
├── commands.json
├── tools/
│   ├── symbol
│   └── references
└── commands/
    └── inspect
```

`tools.json` exposes operations to the model. `commands.json` exposes operations to the human as slash commands. Both may point at extension-owned executables or existing commands on `PATH`.

A tool becomes `astrolabe.symbol`; a slash command becomes `/astrolabe:inspect`. The implementation behind either name remains an ordinary process.

## A small core, many possible agents

`kei` is intended to be the substrate rather than the finished opinionated coding environment. The same runtime can be paired with different extension sets, controls, skills, models, and ACP clients without turning each combination into a new fork of the core.

This is the design constraint behind most of the project:

> standardize the seams; leave the pieces replaceable.

## Try it

```sh
go build -o ./build/kei ./cmd/kei

./build/kei login codex
./build/kei run
```

For a non-interactive prompt:

```sh
./build/kei run -p 'Inspect this repository and summarize its architecture.'
```

To see what the current workspace contributes:

```sh
./build/kei extensions
./build/kei tools
./build/kei commands
```

`kei acp` exposes the same session machinery to ACP clients over stdin/stdout.

## Documentation

The README is intentionally the front door rather than the manual.

- [Documentation map](docs/index.md)
- [Architecture and design boundaries](docs/architecture.md)
- [Connecting providers and authenticating](docs/connect.md)
- [Configuration reference](docs/configuration.md)
- [Sessions and execution model](docs/session.md)
- [Extensions](docs/extension/index.md)
  - [Tools](docs/extension/tool.md)
  - [Slash commands](docs/extension/slash-command.md)
  - [Controls](docs/extension/control.md)
- [ACP frontend](docs/acp.md)
- [Development guide](docs/development.md)

## Status

`kei` is pre-release and intentionally small. The current implementation already has provider selection, authentication, model/tool turns, explicit extension discovery, slash commands, process controls, streaming provider events, a REPL, and an ACP adapter. Some boundaries are deliberately still narrow: child-process output is collected until exit, ACP permission round-trips are incomplete, persistent tool services are not part of the process contract, and controls are configured separately rather than declared by extensions.

Those gaps are not invitations to turn the core into a framework. New mechanisms should earn their place by making the seams more general, not by absorbing the things on either side of them.
