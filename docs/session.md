# Sessions

A `kei` session is the durable logical conversation that a runtime continues over time.

The current implementation keeps that state in memory inside `internal/agent.Session`. Persistence, resume, and compaction are not implemented yet. This document defines the boundary they should follow so those features do not make the session dependent on one provider, cache implementation, or frontend.

The central rule is:

> Session history is canonical. Model context is derived from it.

That distinction matters because long-running agents eventually need to compact old context, resume after process exit, change providers or models, and take advantage of provider caches without making any of those optimizations the source of truth.

## The model

Session handling has four conceptually separate layers:

```text
                durable
                  │
                  ▼
         ┌────────────────┐
         │    Session     │
         │   Transcript   │
         └───────┬────────┘
                 │ materialize
                 ▼
         ┌────────────────┐
         │ Context State  │
         │ / Checkpoints  │
         └───────┬────────┘
                 │ render
                 ▼
         ┌────────────────┐
         │Provider Request│
         └───────┬────────┘
                 │
       ┌─────────┼─────────┐
       ▼         ▼         ▼
 continuation  prompt    usage /
    handle      cache    telemetry
```

The layers have different lifetimes and guarantees.

- **Transcript** is the canonical conversation history.
- **Context state** is a materialized projection of that history for model inference.
- **Provider continuation state** is an optional provider-specific shortcut.
- **Prompt or KV cache state** is an ephemeral optimization.

Losing provider continuation or cache state must never make a session unreadable or logically unresumable.

## Session and runtime are different things

The current `internal/agent.Session` still contains both logical session state and runtime dependencies: provider, tools, commands, skills, controls, callbacks, workdir, a context builder, and conversation messages.

Persistent sessions should not serialize that object.

The durable session should contain logical conversation state and small pieces of metadata needed to identify it. Runtime state should be rebuilt when the session is opened from the current configuration, credentials, extension discovery, controls, provider selection, and frontend.

Conceptually:

```text
session.Session
    identity
    metadata
    transcript
    checkpoints

agent.Runtime
    provider
    tools
    commands
    controls
    context builder
    session store
    approval / frontend callbacks
```

This separation allows the same logical session to be resumed in a new process and, where the context can be represented portably, continued with a different model or provider.

A session is therefore not a snapshot of an execution environment.

## Transcript

The transcript is the source of truth and should be append-only.

It records conversation facts needed to reconstruct the logical interaction, such as:

```text
user message
assistant message and requested tool calls
tool result
user message
assistant message
...
```

Provider transport types are not the transcript schema. `provider.Message` is currently sufficient for the in-memory model loop, but it is a transport-oriented representation and should not become the durable session format by accident.

Runtime instructions are not transcript entries. The current `Session.Messages` slice contains user, assistant, and tool messages; system instructions are materialized separately by the context builder for each provider request.

Frontend lifecycle events are also not transcript entries. Events such as `tool_start`, `tool_end`, and streamed text chunks exist to project execution progress to a frontend; they are not independently part of the logical conversation.

### Tool execution durability

When persistence is introduced, an assistant message containing a tool call should become durable before the tool is executed.

The ordering should be:

```text
append assistant tool call
make it durable
execute tool
append tool result
```

If the process dies between execution and the tool result, the transcript can identify an interrupted tool call. A side-effecting tool must not be silently re-executed merely because its result is missing.

## Context is a projection

The provider should not receive "the session" directly. It should receive a context materialized from the session and the current runtime environment.

A useful conceptual decomposition is:

```text
Context = Base + Checkpoints + Tail + Volatile
```

### Base

The base contains long-lived model input that normally changes less often than conversation turns, for example:

- stable runtime instructions;
- AGENTS instructions;
- loaded skills;
- tool definitions.

The exact provider representation is transport-specific.

The current `internal/context.Builder` implements the first narrow form of this boundary. At session creation it assembles stable instructions from:

1. kei's small built-in coding-agent instruction;
2. `<workspace>/AGENTS.md`, when present;
3. the name and description catalog for discovered Agent Skills.

For each model turn, the builder materializes a provider-facing message slice by prepending those instructions as a system message to the current transcript tail. It also carries the currently visible tool schemas into the provider call. The builder does not yet model checkpoints, provider-native state, or compaction policy.

A `before_model` control may replace the instructions for that provider request. The replacement is request-scoped context; it does not rewrite `Session.Messages` or mutate the builder's stable base.

### Workspace instructions and Agent Skills

Natural-language project instructions live in the workspace-root `AGENTS.md`, not in `config.json`. Nested `AGENTS.md` scoping is not part of the current contract.

Agent Skills use the standard `SKILL.md` format. The current search roots, in precedence order, are:

1. `<workspace>/.agents/skills`
2. `~/.agents/skills`

Each immediate non-hidden child directory is a Skill candidate. A candidate without `SKILL.md` is skipped. Required `name` and `description` metadata are validated during discovery, and the Skill name must match its parent directory. When both roots contain the same Skill name, the workspace copy wins.

Only Skill names and descriptions are placed in the stable base instructions. Full Skill instructions remain progressively disclosed: the model can call `load_skill` to read a selected `SKILL.md`, then `read_skill_resource` to read referenced files under that Skill directory. Resource reads reject paths that escape the Skill root.

Those Skill readers are model-facing built-ins rather than extension processes, but they pass through the same session tool lifecycle events and control/approval path as ordinary tools.

### Checkpoints

A checkpoint represents previously materialized context for an older covered portion of the transcript.

Examples include:

- a portable summary produced by a compaction strategy;
- provider-native compacted state;
- an opaque provider representation that can be reused only with a compatible provider or model family.

A checkpoint is derived state, not conversation history.

### Tail

The tail is the uncovered recent transcript after the selected checkpoint state. It should normally grow by appending new turns rather than rewriting old ones.

The current implementation has no checkpoint storage, so the materialized tail is simply the complete in-memory `Session.Messages` slice.

### Volatile context

Volatile context exists only for the current request or runtime state. It may include temporary control output or other request-scoped information that should not be confused with durable conversation history.

Keeping these regions distinct preserves semantics and also gives provider adapters enough structure to make sensible cache decisions.

## Compaction

Compaction does not rewrite or delete the canonical transcript.

It creates a checkpoint that covers an older transcript range and can replace that range when materializing future model context.

```text
Transcript

A B C D E F G H I J
─────────┘
   covered by K1

Materialized context

Base | K1 | F G H I J
```

The original A-E entries remain available for inspection, recovery, re-compaction, or projection through another provider.

This is deliberately different from storing a `compaction` message inline in the transcript. Compaction may produce provider-native opaque state rather than human-readable summary text, and multiple compatible projections may eventually exist for the same transcript range. Derived execution state therefore belongs beside the transcript, not inside it.

A checkpoint should identify at least the transcript range it covers and the kind of projection it contains. Provider-native checkpoints may additionally need compatibility metadata such as provider and model identity.

The schema should not assume that all compaction is one summary string. A future context builder may use several immutable compacted segments before a recent verbatim tail. The first implementation does not need that strategy, but the checkpoint boundary should not make it impossible.

## Stable prefixes and provider caches

Prompt caching is an optimization over rendered provider input. It is not session state.

Many provider caches reward requests whose older prefix remains unchanged while new conversation content is appended. The context layout should therefore prefer this ordering when it preserves the intended semantics:

```text
least frequently changing

Base | Checkpoint | append-only Tail | Volatile

                         most frequently changing
```

This suggests a useful context invariant:

> Context should preserve stable prefixes when doing so does not change the logical request.

Compaction naturally creates a new context generation. A new checkpoint may invalidate cache reuse after the stable base, while subsequent turns again extend an append-only tail from that checkpoint.

The core should expose semantic structure such as base, checkpoints, tail, and volatile input. It should not expose provider-specific cache directives as session concepts. Provider adapters decide whether and where their transport can use cache breakpoints, cache keys, retained state, or no caching at all.

### Cache identity is not session identity

A session ID identifies a logical conversation. It is not necessarily the correct key for provider cache sharing.

Two requests may share a large stable base while belonging to different sessions, and one session may cross a compaction generation or change runtime input such that its reusable prefix changes. Cache identity therefore belongs to provider request construction, not the durable session identity.

### Continuations are not truth

A provider may offer a server-side continuation handle that avoids resending or reconstructing prior state. Such a handle may be persisted as an optimization, but a session must still be recoverable from its transcript and compatible checkpoints when that handle expires, disappears, or cannot be used with the selected provider.

Cold resume is a normal path, not an error case.

## Tool definitions and availability

Tool definitions can form part of a large stable provider prefix. A temporary policy decision should therefore not require redefining the entire tool catalog when the provider can express call availability separately.

Conceptually distinguish:

```text
Tool catalog
    stable definitions exposed by the runtime

Tool availability
    subset callable for the current model turn
```

Controls may change availability from turn to turn. A provider that supports dynamic tool choice can preserve the stable catalog and vary only availability. Providers without such a facility may continue to render a filtered tool set.

This distinction is an optimization boundary, not a reason to weaken controls. If the semantic tool definition itself changes, the provider request should reflect that change even when it invalidates a cache prefix.

## Instructions and overlays

The same principle applies to instructions.

Stable repository and agent instructions should be distinguishable from temporary per-turn overlays when they are semantically distinct. A control that genuinely replaces the system instruction changes the model request and should do so. A small request-scoped addition does not need to be modeled as mutation of the durable transcript.

The current builder already enforces the first part of that separation: stable instructions live outside `Session.Messages`, while a `before_model` replacement affects only the materialized provider request.

Provider adapters remain responsible for mapping those instruction regions into their native message or instruction format.

## Provider boundary

The current provider interface is:

```go
Stream(ctx, messages, tools, callback) (Result, error)
```

The context builder currently has to flatten its materialized context back into that legacy pair of message/tool arguments before the provider adapter sees it. The interface therefore still loses checkpoint compatibility, stable-region identity, provider-native state, and request-level telemetry structure.

Persistent sessions and compaction will likely require the provider boundary to accept a request-level representation rather than a bare message slice. The core should still avoid implementing a universal AST for every provider-native object. Portable conversation state belongs in the context model; opaque provider-native continuation or compaction state should remain provider-owned data carried through an explicit seam.

This is a design direction, not the current implementation contract.

## Provider telemetry

Cache-aware compaction should be driven by observed behavior rather than assumptions about one provider's current pricing or cache policy.

Provider results should eventually expose portable usage telemetry where available, such as:

```text
input tokens
cached input tokens
cache-write tokens
output tokens
```

The runtime can then measure cache hit rate, uncached input growth, compaction frequency, and latency before adopting more complex compaction strategies.

The first compaction implementation should remain simple. The architecture should make better strategies measurable and replaceable rather than embedding one speculative policy into the session format.

## Concurrency

A logical session is a linear turn stream.

Only one model turn should mutate a session at a time. Parallel work that requires independent conversation branches should use separate sessions or a future explicit fork mechanism rather than concurrent mutation of one transcript.

This rule also gives persistence, checkpoint coverage, tool-call recovery, and provider prefix reuse a deterministic ordering.

The current ACP server processes requests concurrently and keeps sessions in memory. Persistent session work must therefore add per-session serialization rather than relying only on the server map mutex.

## Workspace semantics

The workspace remains runtime context associated with the session.

Currently `Session.Workdir` is:

- the cwd of tool and slash-command child processes;
- included in control events and used by control child processes as cwd;
- the root from which ACP sessions discover workspace-local extensions;
- the root used to read `AGENTS.md` and discover project Agent Skills when constructing the session runtime.

A persistent session may record its workspace path as metadata, but reopening the session should rediscover current instructions, Skills, tools, commands, and controls rather than deserialize old runtime registries.

## Current implementation

Today `internal/agent.Session` owns an in-memory `Messages` slice and directly drives the model/tool loop, while `internal/context.Builder` owns provider-context materialization.

For ordinary user text it:

1. appends the user message to `Session.Messages`;
2. collects the extension-tool and Skill-reader schemas visible to the runtime;
3. applies `before_model` controls to the stable instructions and tool availability for the current turn;
4. asks the context builder to materialize provider messages and tools from the runtime instructions plus `Session.Messages`;
5. calls `Provider.Stream`;
6. appends the completed assistant message to `Session.Messages`;
7. returns when there are no tool calls;
8. otherwise executes tool calls, appends tool results, and repeats.

The materialized system message is not appended to `Session.Messages`.

The loop is capped at 32 model turns for one `Prompt` call.

Discovered slash commands are executed directly before ordinary model input handling and are not appended to model history. Unknown slash-prefixed text remains normal prompt input.

Tool execution errors are converted into tool-result text so the model can react on the next turn. Provider failures, malformed tool arguments, unknown requested tools, control failures, and approval plumbing failures stop the prompt.

The session emits frontend lifecycle events through `OnEvent`:

```text
command_start
command_end
tool_start
tool_end
assistant_message_chunk
```

ACP currently creates only in-memory sessions and advertises no session-loading capability.

## Implementation direction

A first context-builder seam now exists, but session persistence should still be introduced by forming the remaining seams before adding compaction policy:

1. separate durable session state from the agent runtime;
2. hide the current `provider.Message` slice behind a transcript abstraction without prematurely making the provider transport type the durable schema;
3. evolve the provider request boundary so materialized base/checkpoint/tail/volatile regions and optional provider state do not have to be flattened too early;
4. add a persistent session store and resume path;
5. expose provider usage and cache telemetry;
6. add checkpoint storage;
7. extend the context builder to select compatible checkpoints and uncovered transcript tail;
8. implement the first simple compaction strategy.

Do not make session persistence depend on a compaction algorithm, and do not make compaction depend on a particular provider cache API.

## Invariants

The intended session design can be summarized by these invariants:

1. A session is logical conversation state, not provider state.
2. The transcript is canonical and append-only.
3. Runtime instructions are not transcript history.
4. Context is a materialized projection of transcript plus runtime environment.
5. Compaction produces replaceable checkpoints rather than rewritten history.
6. Provider continuation and prompt caches are accelerators, never sources of truth.
7. Context should evolve as stable prefix, infrequently changing checkpoint state, append-only recent tail, and volatile request input.
8. A session has one linear mutation stream at a time.

These rules should remain true even as the storage format, compaction strategy, provider APIs, or frontend capabilities evolve.
