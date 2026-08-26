# kei / 継

`kei` is a small Unix-native harness for coding agents.

It is the control plane around a model, an agent session, and ordinary OS processes. `kei` discovers explicit extension declarations, routes model tool calls and human slash commands, supervises child processes, and exposes sessions to frontends such as ACP clients. It is not an in-process plugin runtime, a package manager, or a custom UI.

## Principles

- Keep the harness small and boring.
- Use processes as the capability boundary.
- Standardize boundaries, not implementation languages.
- Reuse existing CLI tools instead of reimplementing them.
- Group related tools and slash commands as distribution units, not runtime plugins.
- Keep tools (what can be done), commands (human shortcuts), skills (how to work), and controls (how the agent may behave) separate.
- Keep UI outside the runtime; ACP is the primary interactive frontend boundary.
- Prefer explicit declarations over inferring an agent interface from human-oriented CLI help.

## Concepts and execution model

`kei` keeps these roles separate:

| Concept | Responsibility |
| --- | --- |
| Harness / control plane | Owns the agent loop, session state, provider calls, discovery, routing, process supervision, and frontend adapters. |
| Connection target | A named, ordered provider configuration containing API settings and model choices. The first configured target is used when no override is supplied. |
| Provider | The model API boundary. The current interface completes one model turn and returns one result. |
| Extension | A distribution and namespace unit containing explicit tool and command declarations, plus optional executables. |
| Tool | An agent-facing operation selected by a model tool call. |
| Tool descriptor | The explicit schema and process invocation contract for a tool. |
| Slash command | A human-facing named route to a process, intercepted before the normal model prompt path. |
| Skill | Guidance about when, why, and in what sequence to use capabilities; it is not itself a tool or process. |
| Control | A policy hook that can allow, deny, ask for approval, change the system prompt, or hide tools. |
| Frontend | A user or client transport such as the REPL or ACP; it is not the session model. |

A typical model turn looks like this:

```text
client -> frontend -> agent session -> provider -> model
                          |              |
                          |              +-> tool call -> tool process -> tool result
                          +-> controls -> control process -> decision
```

A slash command takes a separate path: `kei` resolves the discovered command and invokes its process without rewriting the command as model text.

### Process boundaries and streams

The Unix process boundary defines how a capability is invoked: executable, arguments, stdin, stdout, stderr, exit status, signals, environment, and working directory. It does not mandate a buffering strategy. A child process may use request/response I/O or stream incrementally over pipes.

Providers expose `Provider.Stream`, which may emit text chunks while it builds a completed result. The Codex Responses transport consumes SSE; the CLI prints those chunks directly and ACP forwards them as `agent_message_chunk`. The other provider transports currently emit their completed text as one chunk. Tool, slash-command, and control execution still collect stdout/stderr until the child exits.

## Build

```sh
go build -o ./build/kei ./cmd/kei
```

## Extensions

An extension is a directory containing zero or more tool and slash-command declarations plus any executables owned by that extension.

```text
extensions/
└── astrolabe/
    ├── tools.json
    ├── commands.json
    ├── tools/
    │   ├── symbol
    │   └── references
    └── commands/
        └── inspect
```

`tools/` and `commands/` are conventions, not special executable formats. Files in them may be written in Go, Rust, Swift, TypeScript, Python, Haskell, shell, or anything else that can be executed by the host OS.

An extension does not need to ship executables at all. It may only describe existing commands such as `git`, `rg`, `gh`, or `jq`.

### Search locations

`kei` searches extension roots in this precedence order:

1. `<workspace>/.kei/extensions/`
2. `$XDG_DATA_HOME/kei/extensions/`, normally `~/.local/share/kei/extensions/`
3. each entry of `$XDG_DATA_DIRS` with `/kei/extensions/` appended, normally `/usr/local/share/kei/extensions/` and `/usr/share/kei/extensions/`
4. additional roots from `extension_dirs` in configuration

Shadowing happens at the extension level. If `<workspace>/.kei/extensions/astrolabe/` exists, an `astrolabe` extension from a lower-precedence user or system directory is ignored as a whole. Tools and commands are not partially merged across copies of the same extension.

This also gives package managers a conventional layout:

```text
<prefix>/bin/foo
<prefix>/share/kei/extensions/foo/tools.json
<prefix>/share/kei/extensions/foo/commands.json
```

An extension that owns helper executables can instead install them under its own directory:

```text
<prefix>/share/kei/extensions/foo/
├── tools.json
├── commands.json
├── tools/
│   └── analyze
└── commands/
    └── review
```

No kei-specific package manager is required. Extensions may arrive through Homebrew, Nix, `go install`, Cargo, npm, pipx, a repository checkout, or any other mechanism.

## Tools

`tools.json` declares one or more agent-facing tools.

```json
{
  "tools": [
    {
      "name": "search_text",
      "description": "Search text in the workspace.",
      "input_schema": {
        "type": "object",
        "properties": {
          "pattern": {"type": "string"},
          "path": {"type": "string", "default": "."}
        },
        "required": ["pattern"]
      },
      "command": "rg",
      "args": ["--line-number", "{pattern}", "{path}"],
      "effects": ["filesystem.read"]
    }
  ]
}
```

A tool's local name is namespaced by its extension. For an extension named `unix`, the tool above is identified by kei as:

```text
unix.search_text
```

For LLM providers that require a restricted function-name alphabet, kei exposes it as:

```text
unix_search_text
```

The namespace conversion is a transport detail; kei's stable identity remains `extension.tool`.

Placeholders map input fields to command arguments. `{name}` is required; `{name?}` is omitted when absent. JSON Schema defaults are applied before argument expansion. A native agent-oriented command may set `"stdin": "json"` and read the complete input object itself.

A command name such as `rg` is resolved through `PATH`. A relative executable such as `./tools/symbol` is resolved relative to the extension root, while the child process itself runs with the workspace as its current working directory. This lets an extension ship its own executable while naturally operating on the repository being edited.

## Slash commands

`commands.json` declares human-facing slash commands.

```json
{
  "commands": [
    {
      "name": "status",
      "description": "Show the concise Git working tree status.",
      "command": "git",
      "args": ["status", "--short"]
    },
    {
      "name": "inspect",
      "description": "Inspect an item with an extension-owned command.",
      "input_hint": "symbol name",
      "command": "./commands/inspect",
      "args": ["{arguments?}"]
    }
  ]
}
```

Commands are exposed using the extension namespace:

```text
/unix:status
/astrolabe:inspect Foo
```

`{arguments}` passes the complete text after the command name as one argument; `{arguments?}` makes it optional. A command may set `"stdin": "text"` to receive that text on stdin instead.

Slash commands are intercepted by kei before the normal model prompt path. They are therefore a user-facing route to ordinary Unix processes, not prompts disguised as a special syntax.

When running over ACP, kei advertises discovered commands with `available_commands_update`, so compatible clients can surface them as slash-command completions.

## Existing CLI tools

One executable may back many agent tools and commands. For example, an extension can expose selected parts of `gh` without wrapping the entire CLI in a single unconstrained tool:

```text
gh executable
├── github.pr_view
├── github.pr_diff
├── github.issue_view
└── /github:status
```

This is intentionally explicit. `kei` does not attempt to synthesize descriptors from `--help`; human-oriented help output does not reliably encode the semantics, side effects, or useful agent-facing granularity of a CLI.

## CLI Commands & Discovery

The example config adds `./examples/extensions` as a low-priority development extension root:

```sh
# View overall CLI help or subcommand help
./kei help
./kei help run
./kei help models

# Inspect discovered extensions, tools, and slash commands
./kei extensions -config examples/config.example.json
./kei tools -config examples/config.example.json
./kei commands -config examples/config.example.json

# View configured and available connection targets, models, and aliases
./kei models -config examples/config.example.json
./kei models -config examples/config.example.json -json

# Execute a tool or slash command directly
./kei exec -config examples/config.example.json -input '{"pattern":"Unix-native"}' unix.search_text
./kei run -config examples/config.example.json -p '/unix:status'
```

## Connections, Models & Authentication

`kei` cleanly separates **connection configuration** (safe for Git / dotfiles in `~/.config/kei/config.json` or `<workspace>/.kei/config.json`) from **credentials & tokens** (saved to `$XDG_STATE_HOME/kei/auth.json` / `~/.local/state/kei/auth.json`). A provider type such as OpenAI or Anthropic is not itself a selected connection target; a target is a named entry in the ordered `providers` list.

### 1. Authentication (`kei login`)

Tokens and API keys are stored securely in `~/.local/state/kei/auth.json` without putting secrets into configuration files.

```sh
# View available auth providers
./kei login

# OpenAI Codex (ChatGPT subscription OAuth PKCE / Device code)
./kei login codex
./kei login codex --device

# Store API keys locally
./kei login anthropic
./kei login gemini
./kei login openai
```

### 2. Configuration

`providers` is an ordered list of named connection targets. The first entry is used when no `-provider` override is supplied. If the list is empty, `kei` detects available authenticated provider types and uses the first one in stable order.

A normal agent session with no existing configuration creates a user configuration file containing the detected available targets. Edit the order or target fields to choose a connection explicitly. Credentials and API keys are never written to this file.

```json
{
  "providers": [
    {
      "name": "openai",
      "type": "openai",
      "model": "gpt-5.6",
      "models": ["gpt-5.6", "gpt-5.5", "gpt-5.5-mini"]
    },
    {
      "name": "claude",
      "type": "anthropic",
      "model": "claude-3-7-sonnet-20250219",
      "models": ["claude-3-7-sonnet-20250219", "claude-3-5-haiku-20241022"]
    },
    {
      "name": "codex",
      "type": "codex",
      "model": "gpt-5.5",
      "models": ["gpt-5.5", "gpt-5.5-mini"]
    },
    {
      "name": "gemini",
      "type": "gemini",
      "model": "gemini-2.5-flash",
      "models": ["gemini-2.5-pro", "gemini-2.5-flash"]
    },
    {
      "name": "local",
      "type": "ollama",
      "model": "llama3.3"
    }
  ],
  "models": {
    "fast": "gpt-5.5-mini",
    "smart": "claude-3-7-sonnet-20250219"
  }
}
```

### 3. Inspecting Models (`kei models`)

```sh
./kei models
./kei models -json
```

### 4. Run with Connection Target / Model Selection

```sh
# Use the first configured target, or the first available target when none is configured
./kei run

# Single prompt non-interactive run
./kei run -p 'Inspect repository'

# Switch model or alias on the fly
./kei run -m fast -p 'Inspect repository'
./kei run -m gpt-5.5 -p 'Inspect repository'

# Switch connection target by its configured name
./kei run -provider claude -p 'Inspect repository'
./kei run -provider codex -m gpt-5.5-mini -p 'Inspect repository'
```

Before starting an agent session, `kei run` verifies that the selected connection target has credentials. If no targets are configured, it falls back to authenticated provider types in stable order. If the selected target is not authenticated, it stops with a `kei login <provider>` hint instead of opening a session that can only fail on its first request. Local `ollama` targets do not require login.

Extension discovery is workspace-scoped. ACP sessions therefore discover `<session cwd>/.kei/extensions` independently rather than sharing one process-global registry.

## ACP

```sh
./kei acp -config examples/config.example.json
```

The ACP adapter is deliberately thin and isolated from the internal session model. The MVP implements `initialize`, `session/new`, `session/prompt`, `session/cancel`, `session/update`, and advertises slash commands after session creation. It projects tool and command lifecycle events plus provider text chunks to ACP; child-process output is still returned after completion. Replacing the adapter with a maintained ACP SDK should not affect the agent/tool core.

## Controls

Controls are currently ordinary processes configured separately in `config.json`. `kei` sends an event as JSON on stdin and reads a decision as JSON on stdout. They can participate in `before_model`, `before_tool`, and `after_tool` without the harness knowing concepts such as Plan or YOLO.

A decision may contain:

```json
{
  "action": "allow",
  "system_prompt": "optional replacement",
  "hidden_tools": ["unix.search_text"]
}
```

`action` may be `allow`, `deny`, or `ask`. Higher-level modes are compositions built outside the core. Moving related controls into extension directories can be added later without changing the process-oriented control model.

## Status

This is an intentionally small MVP. Provider responses use a streaming contract: Codex, the CLI, and ACP can forward incremental model text, while the other provider transports remain buffered and emit one chunk. Tool, command, and control output is still returned after completion. Connection selection uses the first configured target, or the first available provider type when no targets are configured; a normal session creates the initial user configuration automatically. Missing pieces include incremental child-process output forwarding, robust ACP permission round-trips, persistent tool services, cwd/workspace isolation beyond process working directories, sandbox helpers, and a stable extension-level control declaration format.
