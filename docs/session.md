# Sessions

A `kei` session is the logical conversation that a runtime continues over time.

The current implementation now separates that logical state into `internal/session.State` and executes it through `internal/agent.Runtime`. Both remain in memory: persistence, resume, checkpoints, and compaction are not implemented yet. This document defines the boundaries those features should follow so session state does not become dependent on one provider, cache implementation, frontend, or process lifetime.

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

- **Session state** is the logical conversation identity, associated metadata, and canonical transcript.
- **Context state** is a materialized projection of that history for model inference.
- **Provider continuation state** is an optional provider-specific shortcut.
- **Prompt or KV cache state** is an ephemeral optimization.

Losing provider continuation or cache state must never make a session unreadable or logically unresumable.

## Session state and runtime are different things

The current code now makes this separation explicit.

`internal/session.State` contains only logical session-associated state:

```text
session.State
    ID
    Workspace
    Transcript
```

`internal/agent.Runtime` contains the execution dependencies used to continue that state:

```text
agent.Runtime
    State *session.State
    provider
    tools
    commands
    skills
    controls
    context builder
    approval callback
    frontend/event callback
```

The runtime requires a session state, but the session state does not depend on the runtime.

That direction is deliberate. A future session store may persist the state after a durable schema is defined, while a newly opened runtime should rebuild provider clients, credentials, extension discovery, controls, Skills, context construction, and frontend callbacks from the current environment.

`State` itself is not yet a stable serialized format. In particular, the transcript still contains opaque in-memory content values. The current split establishes ownership and lifetime; it does not commit the project to a storage encoding prematurely.

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

`internal/transcript` owns this in-memory logical history. Its entries distinguish user, assistant, and tool roles plus provider-independent tool-call identity, name, arguments, and result linkage. It deliberately has no system role: runtime instructions are context, not conversation history.

Provider transport types are not the transcript schema. `provider.Message`, `provider.ToolCall`, and `provider.FunctionCall` remain transport-oriented request/response representations inside `internal/provider`. The agent runtime converts provider results into logical transcript entries. In the other direction, the context builder preserves logical transcript entries in `context.Request`; provider adapters render those entries into their current transport representation only after the provider boundary.

The first transcript seam is intentionally not a persistence format. `Entry.Content` remains an opaque in-memory value so this refactor does not invent a durable multimodal/content schema before persistence actually requires one. A future session store must define serialization and compatibility explicitly rather than assuming the current Go field shape is stable storage.

Runtime instructions are not transcript entries. Instructions are materialized separately by the context builder for each provider request.

Frontend lifecycle events are also not transcript entries. Events such as `tool_start`, `tool_end`, and streamed text chunks exist to project execution progress to a frontend; they are not independently part of the logical conversation.

### Tool execution durability

When persistence is introduced, an assistant entry containing a tool call should become durable before the tool is executed.

The ordering should be:

```text
append assistant tool call
make it durable
execute tool
append tool result
```

If the process dies between execution and the tool result, the transcript can identify an interrupted tool call. A side-effecting tool must not be silently re-executed merely because its result is missing.

## Context is a projection

The provider should not receive "the session" directly. It should receive a context materialized from the transcript and the current runtime environment.

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

The current `internal/context.Builder` implements the first narrow form of this boundary. At runtime creation it assembles stable instructions from:

1. kei's small built-in coding-agent instruction;
2. `<workspace>/AGENTS.md`, when present;
3. the name and description catalog for discovered Agent Skills.

For each model turn, the builder returns a `context.Request` that keeps the selected instructions, logical transcript tail, and currently visible tool schemas as separate fields. It does not translate those regions into provider messages. The provider layer performs the current transport rendering after it receives the request. The builder does not yet model checkpoints, provider-native state, or compaction policy.

A `before_model` control may replace the instructions for that provider request. The replacement is request-scoped context; it does not rewrite the transcript or mutate the builder's stable base.

### Workspace instructions and Agent Skills

Natural-language project instructions live in the workspace-root `AGENTS.md`, not in `config.json`. Nested `AGENTS.md` scoping is not part of the current contract.

Agent Skills use the standard `SKILL.md` format. The current search roots, in precedence order, are:

1. `<workspace>/.agents/skills`
2. `~/.agents/skills`

Each immediate non-hidden child directory is a Skill candidate. A candidate without `SKILL.md` is skipped. Required `name` and `description` metadata are validated during discovery, and the Skill name must match its parent directory. When both roots contain the same Skill name, the workspace copy wins.

Only Skill names and descriptions are placed in the stable base instructions. Full Skill instructions remain progressively disclosed: the model can call `load_skill` to read a selected `SKILL.md`, then `read_skill_resource` to read referenced files under that Skill directory. Resource reads reject paths that escape the Skill root.

Those Skill readers are model-facing built-ins rather than extension processes, but they pass through the same runtime tool lifecycle events and control/approval path as ordinary tools.

### Checkpoints

A checkpoint represents previously materialized context for an older covered portion of the transcript.

Examples include:

- a portable summary produced by a compaction strategy;
- provider-native compacted state;
- an opaque provider representation that can be reused only with a compatible provider or model family.

A checkpoint is derived state, not conversation history.

### Tail

The tail is the uncovered recent transcript after the selected checkpoint state. It should normally grow by appending new turns rather than rewriting old ones.

The current implementation has no checkpoint storage, so `context.Request.Tail` contains all entries returned by the in-memory transcript.

### Volatile context

Volatile context exists only for the current request or runtime state. It may include temporary control output or other request-scoped information that should not be confused with durable conversation history.

The current request type does not add an empty `Volatile` field merely for future symmetry. Request-scoped instruction replacement is represented by the instructions actually selected for that request. A distinct volatile region should be added only when current behavior needs one.

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

The current `context.Request.Tools` field contains only the visible tool schemas for that turn; catalog-versus-availability is not yet represented separately because no current provider path consumes that distinction.

This distinction is an optimization boundary, not a reason to weaken controls. If the semantic tool definition itself changes, the provider request should reflect that change even when it invalidates a cache prefix.

## Instructions and overlays

The same principle applies to instructions.

Stable repository and agent instructions should be distinguishable from temporary per-turn overlays when they are semantically distinct. A control that genuinely replaces the system instruction changes the model request and should do so. A small request-scoped addition does not need to be modeled as mutation of the durable transcript.

The current builder already enforces the first part of that separation: stable instructions live outside the transcript, while a `before_model` replacement affects only `context.Request.Instructions` for that model call.

Provider adapters remain responsible for mapping those instruction regions into their native message or instruction format.

## Provider boundary

The provider-facing request model now belongs to `internal/context`:

```go
type Request struct {
    Instructions string
    Tail         []transcript.Entry
    Tools        []map[string]any
}
```

The provider interface consumes that request as one unit:

```go
Generate(ctx, request, callback) (Result, error)
```

This is intentionally narrower than the eventual conceptual `Base + Checkpoints + Tail + Volatile` layout. Only distinctions that exist in current behavior are represented now. Checkpoints, provider continuation state, volatile overlays, telemetry options, and tool catalog/availability can extend the request or adjacent provider state when those features are actually implemented.

The important boundary is dependency direction: `internal/context` no longer depends on `internal/provider`. The context builder materializes model-facing semantics; the provider package decides how those semantics become Anthropic, Gemini, OpenAI-compatible, or Codex wire objects.

The existing transports still share a message-oriented rendering helper before entering their older transport functions. That is an implementation detail inside `internal/provider`, not the runtime/context contract. A provider can consume request regions more directly when cache breakpoints, continuation handles, or native compaction make that useful without changing the agent loop.

The core should continue to avoid implementing a universal AST for every provider-native object. Portable conversation state belongs in the transcript/context model; opaque provider-native continuation or compaction state should remain provider-owned data carried through an explicit seam.

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

The current ACP server processes requests concurrently and keeps active runtimes in memory. Persistent session work must therefore add per-session serialization rather than relying only on the server map mutex.

## Workspace semantics

The workspace path is currently session-associated metadata stored as `session.State.Workspace`.

The active runtime uses it as:

- the cwd of tool and slash-command child processes;
- the workdir included in control events and used by control child processes as cwd;
- the root from which ACP session creation discovers workspace-local extensions;
- the root used to read `AGENTS.md` and discover project Agent Skills when constructing the runtime.

Recording the workspace path in logical state does not mean runtime discovery results are durable. Reopening a future persisted session should use the recorded workspace as the starting point, then rediscover current instructions, Skills, tools, commands, controls, and provider configuration rather than deserialize old runtime registries.

## Current implementation

Today `internal/session.State` owns the session ID, workspace path, and in-memory `transcript.Transcript`. `internal/agent.Runtime` owns the dependencies and callbacks needed to execute turns against that state. `internal/context.Builder` owns model-context materialization, and `internal/provider` owns transport rendering.

For ordinary user text, the runtime:

1. requires a `session.State`;
2. appends a user entry to `State.Transcript`;
3. collects the extension-tool and Skill-reader schemas visible to the runtime;
4. applies `before_model` controls using `State.ID` and `State.Workspace`;
5. asks the context builder to materialize `Instructions`, logical `Tail`, and visible `Tools` into a `context.Request`;
6. calls `Provider.Generate` with that request;
7. lets the provider layer render the request into its current wire representation;
8. converts the completed provider result into an assistant transcript entry;
9. returns when there are no tool calls;
10. otherwise executes tool calls and appends tool-result entries before repeating.

Runtime instructions are never appended to the transcript. Provider tool-call transport fields are also not stored wholesale: the transcript keeps the logical call ID, name, arguments, and result linkage needed to reconstruct the conversation.

The loop is capped at 32 model turns for one `Prompt` call.

Discovered slash commands are executed directly before ordinary model input handling and are not appended to model history. Unknown slash-prefixed text remains normal prompt input.

Tool execution errors are converted into tool-result text so the model can react on the next turn. Provider failures, malformed tool arguments, unknown requested tools, control failures, and approval plumbing failures stop the prompt.

The runtime emits frontend lifecycle events through `OnEvent`:

```text
command_start
command_end
tool_start
tool_end
assistant_message_chunk
```

ACP currently maps protocol session IDs to active in-memory `agent.Runtime` values and advertises no session-loading capability. The logical state is reachable through each runtime's `State`; there is no session store yet.

## Implementation direction

The context-builder, transcript, session/runtime, and provider-request seams now exist. Persistence can be introduced without first committing the session model to provider transport details:

1. define the durable transcript/content encoding needed by storage and add a persistent session store plus resume path;
2. add per-session serialization for concurrent frontends/runtimes;
3. expose provider usage and cache telemetry;
4. add checkpoint storage;
5. extend `context.Request` and the context builder to select compatible checkpoints and the uncovered transcript tail;
6. let providers use request regions and optional provider-owned state for cache/continuation optimizations where supported;
7. implement the first simple compaction strategy.

Do not make session persistence depend on a compaction algorithm, and do not make compaction depend on a particular provider cache API.

## Invariants

The intended session design can be summarized by these invariants:

1. A session is logical conversation state, not provider/runtime state.
2. Runtime dependencies can be rebuilt around the same logical session state.
3. The transcript is canonical and append-only.
4. Provider transport structs are not canonical transcript state.
5. Runtime instructions are not transcript history.
6. Context is a materialized projection of transcript plus runtime environment.
7. Provider adapters receive structured model-context regions before transport rendering.
8. Compaction produces replaceable checkpoints rather than rewritten history.
9. Provider continuation and prompt caches are accelerators, never sources of truth.
10. Context should evolve as stable prefix, infrequently changing checkpoint state, append-only recent tail, and volatile request input.
11. A session has one linear mutation stream at a time.

These rules should remain true even as the storage format, compaction strategy, provider APIs, or frontend capabilities evolve.
