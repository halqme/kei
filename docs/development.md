# Development

This guide expands the repository workflow in `AGENTS.md` for people changing the Go control plane.

## Repository shape

```text
cmd/kei              CLI and REPL entry points
internal/acp          ACP server and wire projection
internal/agent        runtime/model/tool orchestration
internal/auth         credential store and provider auth flows
internal/command      slash-command descriptors and execution
internal/config       configuration schema and persistence
internal/context      model-context materialization from runtime instructions and transcript tail
internal/control      external control process chain
internal/extension    extension discovery and namespacing
internal/provider     provider interface and transports
internal/session      logical session identity, workspace metadata, and transcript ownership
internal/skill        Agent Skills discovery and on-demand loading
internal/tool         tool descriptors, registry, and execution
internal/transcript   provider-independent logical conversation history
examples               example configuration and extension declarations
docs                   public design and reference documentation
```

The package split is part of the design. Prefer adding behavior to the package that already owns its contract rather than creating a generic helper layer or routing everything through `internal/agent`.

## Local tasks

The `.justfile` provides the standard repository tasks:

```sh
just test
just vet
just build
```

They map to:

```sh
go test -count=1 ./...
go vet ./...
go build -o ./build/kei ./cmd/kei
```

Use focused tests while iterating, for example:

```sh
go test ./internal/session
go test ./internal/transcript
go test ./internal/context
go test ./internal/agent
go test ./internal/acp
go test ./cmd/kei
```

Run `gofmt` on modified Go files before the full verification pass.

## Modern Go guidance

The repository includes `.agents/skills/use-modern-go/SKILL.md`. Coding agents changing Go should follow that skill and query the bundled Modern Go Guidelines helper for the relevant file before editing. Human contributors can also use the wrapper when checking version-specific Go guidance.

The helper is development guidance; it is not a runtime dependency of `kei`.

## Testing by contract

Tests should live as close as practical to the contract they prove.

For extension discovery, test roots, precedence, whole-extension shadowing, stable ordering, hidden directories, descriptor parsing, and qualified names.

For Agent Skills, test the project/user root precedence, required `SKILL.md` metadata contract, progressive-disclosure catalog, and confinement of referenced resource reads to the Skill root.

For workspace instructions and context materialization, test root `AGENTS.md` composition, the absent-file case, and the boundary that keeps runtime instructions out of the transcript. Nested instruction scoping is not part of the current contract.

For transcript behavior, test logical ordering and the provider-independent information needed to reconstruct user, assistant, tool-call, and tool-result history. Do not turn transport fields into transcript fields merely because one provider exposes them.

For session/runtime ownership, test the actual boundary: `session.State` carries logical state, while `agent.Runtime` requires that state and carries execution dependencies. Persistence encoding is not part of this contract yet.

For tools, test schema defaults, required/optional placeholders, array expansion, stdin JSON, timeout behavior, PATH lookup, extension-relative executables, workspace cwd, stderr, and non-zero exits as relevant to the change.

For slash commands, test invocation parsing separately from process execution. Unknown slash-prefixed text is runtime behavior and should remain a prompt when no discovered command matches.

For configuration, test lookup order, explicit versus implicit paths, creation permissions, non-overwrite behavior, ordered provider resolution, aliases, and errors.

For provider transports, keep HTTP/JSON/SSE translation tests inside `internal/provider`. Agent tests should focus on provider-independent orchestration.

For controls, test chain ordering and decision accumulation/short-circuit behavior separately from runtime reactions to `allow`, `deny`, and `ask`.

For ACP, test JSON-RPC parsing and ACP projection in `internal/acp`; avoid encoding ACP-specific assumptions into session, provider, or transcript packages.

## Cross-package paths worth tracing first

Some changes begin in one package but redefine a seam.

### Descriptor changes

A tool descriptor field can affect:

```text
tools.json
  -> internal/tool Descriptor
  -> internal/extension loading
  -> provider tool schema
  -> tool execution
  -> kei tools / kei exec
  -> examples and docs
```

A slash-command descriptor field follows the analogous path through `internal/command` and frontend command advertisement.

### Provider streaming changes

```text
provider transport
  -> provider.Stream / StreamEvent
  -> context rendering
  -> agent Runtime.OnEvent
  -> REPL and/or ACP projection
```

Do not infer streaming guarantees from one provider implementation. The provider interface must represent any guarantee relied upon by agent/frontends.

### Session-state changes

```text
session.State
  -> agent.Runtime
  -> context materialization
  -> CLI/ACP runtime creation
  -> future store/resume boundary
```

Do not put a provider client, extension registry, control chain, context builder, credential, or frontend callback into logical session state. A runtime should be reconstructible around the state.

### Workspace changes

Workspace/cwd behavior crosses session metadata, extension search roots, root `AGENTS.md`, project Skill discovery, context construction, process working directories, CLI runtime creation, and ACP session creation. Treat changes there as a single contract and test all affected paths.

### Naming changes

Tool and command names appear in extension loading, registries, model-facing conversion, direct CLI execution, REPL help, ACP command advertisement, controls, examples, and docs. Changing separators or normalization is a migration, not a local refactor.

## Documentation expectations

Update docs with behavior changes, not as a cleanup after the code has already diverged.

- `docs/configuration.md` owns config/auth-facing contracts.
- `docs/session.md` owns logical session state, runtime ownership, transcript/context materialization, workspace instructions, and Agent Skills semantics.
- `docs/extension/*` owns declarations/discovery/execution contracts.
- `docs/acp.md` owns ACP behavior.
- `docs/architecture.md` owns rationale and core-versus-external boundaries.

The root README should remain an introduction. `AGENTS.md` should remain a workflow file. Detailed reference belongs here under `docs/`.

## Scope review

A code change can be correct and still be the wrong change for `kei` if it grows the control plane around one concrete capability.

Before creating a package or dependency, check whether an executable plus descriptor is enough. Before creating a mode enum, check whether controls/prompts/tool visibility are enough. Before creating a UI framework, check whether ACP should carry the feature to a client.

This is not a ban on core features. It is the project's dependency direction: capabilities depend on the harness boundary; the harness should not depend on every capability.
