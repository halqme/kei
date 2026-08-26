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
- conversation and session state
- workspace instruction composition
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

Project-specific agent instructions live in the workspace-root `AGENTS.md`. They are composed into the session system prompt rather than duplicated as natural-language configuration in `config.json`.

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

Plan, YOLO, Review, Research, or similar modes are intentionally not first-class branches in the session loop.

A mode can often be described as a composition of:

- system-prompt changes
- a visible/hidden tool set
- approval policy
- one or more control processes
- frontend state or presentation
- skills

The core should expose general seams that make these compositions possible. It should only learn a mode-specific concept when multiple real implementations demonstrate that a more general seam is missing.

## Session core versus frontend

A session owns model history and orchestration. A frontend turns that session into a user experience.

The built-in REPL and ACP adapter should therefore project the same underlying concepts rather than defining separate agent semantics. ACP names and wire objects should stay confined to `internal/acp` wherever possible.

This matters for streaming too: `internal/provider.Provider.Stream` returns a completed result while optionally emitting stream events. The agent forwards text-delta events to its own event callback; frontends decide how to render those events. A transport supporting incremental text does not require the session model itself to become transport-specific.

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
