# AGENTS.md

## Project

`kei` (継) is a small Unix-native harness for coding agents.

The core idea is not to build an extensible application by adding a large plugin subsystem. `kei` stays small, delegates capabilities to ordinary OS processes, and becomes extensible as a consequence of Unix-style composition.

When making changes, preserve that direction. Prefer a small stable process boundary over adding another abstraction to the core.

## Architecture

The Go process is the control plane. Its responsibilities should remain limited to things such as:

- LLM provider communication
- the agent loop and session state
- extension discovery
- tool and slash-command routing
- process execution, supervision, cancellation, and timeouts
- control hooks
- frontend adapters such as ACP

Concrete capabilities should normally live outside the harness as processes. Examples include language-specific code analysis, LSP integration, browser automation, Git workflows, formatters, static analyzers, and OS-specific sandboxing.

Do not move a capability into the core merely because implementing it in Go is convenient.

## Execution model

`kei` coordinates a session; it is not the host for extension capabilities. A normal model turn is:

1. The session asks a provider for one completed model result.
2. If the result contains a tool call, the session resolves the descriptor, applies controls, and launches the declared process.
3. The process result is added to the conversation and the session asks the provider for the next result.
4. A frontend projects session results and lifecycle events to the client.

A discovered slash command bypasses model turns and runs its process directly. The current `internal/provider.Provider` interface is stream-oriented: `Stream` may emit provider events while it returns a completed `Result`. Codex consumes its Responses SSE transport incrementally; other provider transports may still buffer and emit one text event. Tool, slash-command, and control execution currently collect child-process output until exit. This is an implementation status, not a Unix design rule.

The process boundary can carry incremental output, but streaming is an optional behavior that must be represented in provider, session, and frontend contracts. Do not conflate Unix process composition with either streaming or buffering.

## Connections and configuration

A connection target is a named provider configuration, not merely a provider type. `Config.Providers` is an ordered list; the first entry is used when no override is supplied. When the list is empty, session startup derives candidates from available authenticated provider types in the stable order returned by `provider.List` and uses the first candidate.

Session startup may create the initial user configuration with those candidates. Read-only inspection commands should not create or rewrite configuration files. Credentials and API keys must remain in the auth store or environment, never in generated configuration.

There is no implicit provider-type fallback. Missing or invalid provider types must produce an explicit error and available-provider guidance.

## Unix process boundary

Treat the process boundary as the primary extension ABI:

- executable
- argv
- stdin
- stdout
- stderr
- exit status
- signals
- environment
- working directory

This is an invocation ABI, not a promise of live output. Request/response and streaming processes are both compatible with it; choose the behavior required by the capability and its descriptor/protocol.

Tool implementations may use any language. Do not introduce a requirement that extensions use Go, Node.js, or any other particular runtime.

Prefer existing CLI programs over reimplementing their behavior inside `kei`.

## Extensions

An extension is a distribution and namespace unit, not in-process code injection.

Extension roots are discovered in precedence order:

1. `<workspace>/.kei/extensions/`
2. `$XDG_DATA_HOME/kei/extensions/`
3. each `$XDG_DATA_DIRS` entry with `/kei/extensions/` appended
4. extra roots from `extension_dirs`

An extension has the form:

```text
extensions/<id>/
├── tools.json       # optional
├── commands.json    # optional
├── tools/           # optional executables
└── commands/        # optional executables
```

A higher-precedence extension shadows a lower-precedence extension with the same ID **as a whole**. Do not partially merge tools or commands across copies of the same extension.

Keep `tools.json` and `commands.json` explicit. Do not infer descriptors from `--help`, man pages, shell completions, or other human-oriented CLI output as part of normal discovery.

### Tool identity

Tool identity is namespaced by the extension:

```text
<extension>.<tool>
```

Provider-facing names may use another representation such as `<extension>_<tool>` when required by provider naming constraints. Treat that representation as transport-specific; the qualified `extension.tool` identity is canonical inside `kei`.

### Slash commands

Slash commands are namespaced as:

```text
/<extension>:<command>
```

Only discovered commands should be intercepted. Do not reserve every string beginning with `/`; unknown slash-prefixed input must remain valid user input and continue through the ordinary prompt path.

Slash commands are a user-facing route to capabilities. They are not inherently prompts and should not be implemented by blindly rewriting every command into model text.

## Executable resolution and cwd

For extension declarations:

- commands without path separators, e.g. `rg`, are resolved through `PATH`
- relative commands, e.g. `./tools/symbol`, are resolved relative to the extension root
- the spawned process itself runs with the session workspace as its working directory

Preserve this distinction. It allows an extension to own its executable while naturally operating on the active repository.

## Tools, commands, skills, and controls

Keep these concepts separate:

- **Tool**: an agent-facing operation selected through a provider tool call.
- **Tool descriptor**: the explicit schema and process invocation contract for a tool.
- **Slash command**: a human-facing named route to a process, intercepted before the normal model prompt path.
- **Skill**: guidance about when, why, and in what sequence to use capabilities; it is not itself a tool or executable process.
- **Control**: a policy hook that can affect model or tool execution without defining the capability itself.

Do not put CLI manuals into Skills when a descriptor can express the invocation interface.

Do not hard-code higher-level modes such as Plan, YOLO, Review, or Research into the core. The core should expose general control points; higher-level behavior should be composable outside it.

## Controls

Controls are process-oriented. They may participate in events such as `before_model`, `before_tool`, and `after_tool` and return decisions such as `allow`, `deny`, or `ask`.

Keep the control mechanism generic. A feature request framed as a new "mode" should first be considered as a composition of controls, prompts, visible tools, and frontend state rather than a new branch in the agent loop.

Metadata such as tool effects is useful for policy and UX but is not a security boundary. Strong isolation belongs in OS-level sandboxing or dedicated sandbox helper processes.

## ACP and frontends

ACP is the primary interactive frontend boundary. The harness should not accumulate a large custom TUI.

Keep ACP isolated behind an adapter. Internal session, tool, and control models must not become ACP-specific merely because ACP exposes a similar concept.

When ACP supports a frontend feature, prefer projecting existing internal state or extension declarations into ACP rather than duplicating that concept in the core.

Examples:

- discovered slash commands -> `available_commands_update`
- tool and command lifecycle -> `session/update`
- approval requests -> ACP client permission UI when supported
- session output -> `session/update`

The adapter emits lifecycle notifications and a completed agent response for each prompt. Providers that emit `StreamEvent` text deltas are projected as `agent_message_chunk` updates; the explicit provider, session, and frontend contracts—not the update name alone—define whether output is streamed. Additional incremental output requires extending those contracts.

ACP is a transport/frontend contract, not the internal architecture.

## Go implementation style

The Go core should be deliberately boring.

Prefer:

- the standard library where practical
- small packages with obvious ownership
- plain structs and interfaces
- `context.Context` for cancellation and lifetimes
- `os/exec` and explicit process boundaries
- simple JSON structures at external boundaries
- deterministic discovery and ordering
- explicit errors over hidden fallback behavior

Avoid adding dependencies or framework layers unless they remove substantial complexity that is already present.

Do not add abstraction in anticipation of hypothetical future backends. Implement the smallest interface required by current independent use cases.

## Package responsibilities

Current package boundaries are intentional:

```text
cmd/kei              CLI entry point
internal/acp          ACP frontend adapter
internal/agent        agent/session orchestration
internal/command      slash-command descriptors and execution
internal/config       configuration loading
internal/control      generic control process integration
internal/extension    extension discovery and namespacing
internal/provider     LLM provider boundary
internal/tool         tool descriptors, registry, and execution
```

Before creating a new package, check whether the functionality belongs in an external process instead.

Avoid import cycles and avoid making `internal/agent` a dumping ground for extension-specific behavior.

## Compatibility and configuration

This project is pre-release; breaking changes to the configuration schema are acceptable when they make the connection model explicit. Treat extension declaration formats and canonical qualified names as user-maintained interfaces.

The configuration contract is:

- `providers` is an ordered list of named connection targets.
- The first configured target is selected when no override is supplied.
- An empty list falls back to available authenticated provider types in stable order.
- Session startup may generate a user configuration; inspection commands must remain read-only.

Do not silently reinterpret existing descriptor fields.

When changing discovery behavior, shadowing, executable resolution, namespaces, workspace semantics, provider selection, or configuration generation, add tests covering precedence, ordering, fallback, and non-overwrite boundaries.

## Testing

Run before considering a change complete:

```sh
go test ./...
go build ./cmd/kei
```

For changes to extension discovery, add or update tests for at least:

- workspace/user/system precedence
- extension-level shadowing
- deterministic discovery
- qualified tool/command names

For changes to process execution, test relevant combinations of:

- PATH-resolved commands
- extension-relative executables
- workspace cwd
- stdin mode
- argument placeholders and defaults
- timeout/cancellation
- stderr and non-zero exits

For provider selection and configuration generation, test:

- first configured target ordering
- available-provider fallback ordering
- generated config path and permissions
- existing config is not overwritten
- explicit missing paths are not created

For ACP changes, keep protocol parsing tests separate from agent behavior where possible.

## Scope discipline

Keep the project focused on being a harness.

Before adding a feature to the core, ask:

1. Can an existing Unix command already do this?
2. Can this be an extension-owned process?
3. Can this be expressed as a descriptor or control instead of core behavior?
4. Is the proposed protocol addition required by multiple real implementations?

If the answer points outside the core, keep it outside the core.

Do not build a kei-specific package manager unless concrete requirements cannot be met by existing distribution mechanisms such as Homebrew, Nix, Cargo, npm, pipx, or ordinary filesystem installation.

Do not add descriptor generation from `--help` as a default or required workflow.

Do not build a full TUI when ACP can delegate the UI to a client.

Above all: keep `kei` small enough that capabilities can evolve independently of it.
