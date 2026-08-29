# Architecture

`kei` is a coordination layer around replaceable pieces. Its architecture is intentionally biased toward Unix process composition rather than an application-style plugin ecosystem.

## The boundary that matters

The primary extension ABI is the operating-system process boundary:

- executable
- arguments
- stdin
- stdout
- stderr
- exit status
- signals and cancellation
- environment
- working directory

That boundary is deliberately language-neutral. An extension executable can be implemented in Go, Rust, Swift, Python, Haskell, shell, or anything else the host can execute. It can also simply be an existing program already available through `PATH`.

The process ABI is about invocation, not buffering. The current tool, slash-command, and control executors collect stdout/stderr until the child exits. That is an implementation status, not a claim that Unix processes must be request/response only.

## What the Go core should own

The core should own coordination that cannot sensibly be delegated to one capability process:

- model/provider communication
- logical conversation and session state
- context materialization and workspace instructions
- Agent Skills discovery and progressive disclosure
- extension discovery and namespace resolution
- routing model tool calls and human slash commands
- process lifecycle, timeout, cancellation, cwd, and error plumbing
- generic control hooks
- frontend adapters such as ACP
- configuration and authentication plumbing needed to connect those pieces

The core should generally not absorb a concrete capability merely because writing it in Go would be convenient. Code search, Git workflows, LSP analysis, browser automation, formatters, static analyzers, and platform-specific sandbox implementations are better candidates for extension-owned processes unless they expose a reusable coordination requirement.

## Explicit declarations

`kei` does not treat arbitrary human-oriented CLIs as self-describing agent tools. A CLI's `--help` output usually cannot express enough about input shape, side effects, intended granularity, defaults, or safe exposure to a model.

Extensions therefore declare the agent-facing contract explicitly in `tools.json` and the human-facing command contract in `commands.json`.

This keeps a useful distinction:

```text
existing executable
      │
      ├── one explicit tool descriptor
      ├── another explicit tool descriptor
      └── one explicit slash command
```

One executable can back many narrowly scoped agent operations without exposing the whole CLI as an unconstrained function.

## Namespace and distribution

An extension is not executable code loaded into the Go process. It is a namespace and a unit that can be installed or shadowed as a whole.

The same extension ID found in a higher-precedence root wins entirely over lower-precedence copies. `kei` does not merge `tools.json` from one copy with `commands.json` from another. This makes workspace overrides predictable and keeps an installed extension internally coherent.

Distribution is intentionally outside the runtime. A package manager may place descriptors under a conventional XDG data path, an extension may be checked into `.kei/extensions`, or a configuration can point at another root. No kei-specific package manager is required.

## Instructions and Skills use existing file contracts

Project-specific agent instructions live in the workspace-root `AGENTS.md`. They are composed into stable runtime context rather than duplicated as natural-language configuration in `config.json` or stored in conversation history.

Skills use the Agent Skills `SKILL.md` contract and standard `.agents/skills` locations. `kei` discovers and validates the metadata it needs for routing, advertises only names and descriptions initially, and loads full Skill instructions or referenced resources on demand. It does not introduce a `skills.json` schema or a kei-specific Skill frontmatter extension.

This file-oriented boundary keeps instructions and Skills portable across agent clients while leaving process-backed capabilities in the extension system.

## Separate concepts on purpose

`kei` keeps four concepts distinct:

- **Tool** — something the model can call.
- **Slash command** — something the human invokes directly.
- **Skill** — guidance for how an agent should reason about or sequence capabilities.
- **Control** — policy applied around model/tool execution.

Collapsing these concepts tends to make the core more opinionated than necessary. A tool schema should describe invocation, not become a manual. A skill should not be required just to expose argv. A control should not define the capability it regulates.

## Higher-level modes are compositions

Plan, YOLO, Review, Research, or similar modes are intentionally not first-class branches in the agent runtime loop.

A mode can often be described as a composition of:

- system/context changes
- a visible/hidden tool set
- approval policy
- one or more control processes
- frontend state or presentation
- skills

The core should expose general seams that make these compositions possible. It should only learn a mode-specific concept when multiple real implementations demonstrate that a more general seam is missing.

## Session state, runtime, and frontend

These are separate layers.

`internal/session.State` owns logical conversation identity, workspace metadata, and the canonical transcript. It should remain meaningful after one process or provider client disappears.

`internal/agent.Runtime` owns the execution environment used to continue that state: provider, discovered tools and commands, Skills, controls, context construction, approval callbacks, frontend event callbacks, and an optional session store. A runtime should be rebuildable around the same logical state.

A frontend turns an active runtime/session pair into a user experience. The built-in REPL and ACP adapter should therefore project the same underlying concepts rather than defining separate agent semantics. ACP names and wire objects should stay confined to `internal/acp` wherever possible.

Streaming belongs to the provider/runtime path rather than canonical session state. `internal/provider.Provider.Generate` returns a completed result while optionally emitting stream events. The runtime forwards text-delta events to its event callback; frontends decide how to render those events. A transport supporting incremental text does not require canonical session state to become transport-specific.

`session.State` remains an in-memory ownership model rather than the durable file schema. `internal/session.FileStore` uses a separate versioned append-only record format containing session metadata and portable transcript facts. Runtime/provider objects, extension registries, controls, context builders, credentials, and frontend callbacks are never serialized into that format.

Persistent CLI sessions are opt-in through a named session ID. Reopening one loads its logical state first, then rebuilds the runtime from current configuration and discovery using the persisted workspace. ACP remains in-memory until its protocol-level load-session path is implemented.

## Context and provider requests

Context semantics belong above provider transport details.

`internal/context.Builder` materializes the current model request as separate instructions, logical transcript tail, and visible tools. `internal/provider.Provider.Generate` receives that `context.Request` and decides how to render it for the selected transport.

The dependency direction is intentional:

```text
session transcript + runtime environment
                │
                ▼
        internal/context
          context.Request
                │
                ▼
       internal/provider
         transport rendering
                │
                ▼
         provider API
```

`internal/context` must not depend on provider wire structs. Conversely, provider-native continuation handles, cache directives, compacted opaque state, or transport telemetry should not become canonical session or transcript fields.

The current request contains only distinctions that already exist in the runtime: instructions, tail, and visible tools. Do not add placeholder fields for checkpoints, volatile overlays, cache identity, or continuation state merely to make the type look complete. Extend the boundary when an implemented feature needs the distinction.

This separation allows different providers to exploit their native caching or continuation mechanisms without forcing those concepts into the agent loop or durable session model.

## Controls are policy, not isolation

Controls currently receive JSON events over stdin and may return decisions over stdout. They can allow, deny, ask for approval, replace the system prompt, or hide tools.

Tool `effects` are useful inputs to such policy, but they are metadata. They are not a security boundary. Strong isolation belongs in operating-system mechanisms or dedicated sandbox/helper processes.

## Design test for new features

Before adding something to the core, ask in order:

1. Can an existing Unix program already do it?
2. Can an extension-owned process do it?
3. Can a descriptor expose the missing capability?
4. Can a generic control express the policy?
5. Can a skill express the workflow?
6. Can the frontend own the UX?
7. Is there still a coordination problem the core must solve?

Only the last category is an obvious fit for the harness.

The goal is not minimal code at any cost. The goal is a small stable center whose surroundings can evolve independently.
