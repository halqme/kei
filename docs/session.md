# Sessions

A `kei` session is the logical conversation that a runtime continues over time.

The central rule is:

> Session history is canonical. Model context is derived from it.

A session must remain understandable without provider continuation handles, prompt caches, frontend state, or the process that originally created it.

## Layers

Session handling separates four kinds of state:

```text
                durable
                  │
                  ▼
         ┌────────────────┐
         │ Session State  │
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

- **Session state** is logical identity, workspace metadata, and canonical transcript history.
- **Context state** is a projection selected for a model call.
- **Provider continuation state** is an optional provider-specific shortcut.
- **Prompt/KV cache state** is an ephemeral optimization.

Losing the last two must never make the session logically unreadable or unresumable.

## Session state and runtime

`internal/session.State` owns only logical session-associated state:

```text
session.State
    ID
    Workspace
    Transcript
```

`internal/agent.Runtime` owns the environment used to continue that state:

```text
agent.Runtime
    State *session.State
    optional session Store
    provider
    tools
    commands
    skills
    controls
    context builder
    approval callback
    frontend/event callback
```

The dependency points one way: a runtime needs session state, but session state does not contain provider clients, extension registries, credentials, controls, context builders, or frontend callbacks.

Cold resume therefore loads logical state first and rebuilds the runtime from the current environment.

## Persistent CLI sessions

Persistence is opt-in for the built-in CLI:

```sh
kei run --session review-task
kei run --session review-task -p "continue the review"
```

`--session <id>` means:

- load the named session when it already exists;
- otherwise create it using the current working directory as its workspace;
- persist every later transcript entry;
- on later runs, use the persisted workspace and rediscover current instructions, Skills, extensions, controls, provider configuration, and credentials.

Without `--session`, `kei run` keeps the previous ephemeral behavior.

Session IDs are file-safe identifiers containing ASCII letters, digits, `.`, `_`, or `-`, with a maximum length of 128 characters. Path-like IDs such as `../other` are rejected.

The default session directory is:

```text
$XDG_STATE_HOME/kei/sessions
```

when `XDG_STATE_HOME` is an absolute path, otherwise:

```text
~/.local/state/kei/sessions
```

Each named session is stored as `<id>.jsonl` with owner-only file permissions.

ACP sessions remain in-memory and still advertise no session-loading capability. Their protocol-level load/resume contract should be added separately rather than inferred from the CLI flag.

## Durable format

The durable format is deliberately separate from the Go shape of `session.State` and `transcript.Entry`.

A version-1 file is append-only JSON Lines. The first record contains identity and workspace metadata:

```json
{"type":"session","version":1,"id":"review-task","workspace":"/workspace/project"}
```

Later records are logical transcript entries:

```json
{"type":"entry","role":"user","content":"review this"}
{"type":"entry","role":"assistant","content":"checking","tool_calls":[{"id":"call-1","name":"search","arguments":"{\"query\":\"foo\"}"}]}
{"type":"entry","role":"tool","tool_call_id":"call-1","content":"result"}
```

Version 1 stores only the portable facts kei currently needs:

- role: `user`, `assistant`, or `tool`;
- text content;
- assistant tool-call ID, name, and argument string;
- tool-result linkage through `tool_call_id`.

Provider transport fields are not persisted.

`transcript.Entry.Content` remains `any` in memory because it is not itself the storage schema. The version-1 store accepts string content and treats `nil` assistant content as empty text. Any other content type is rejected instead of being silently converted to JSON. A future multimodal/content feature must define an explicit durable representation and compatibility path.

This version field is a compatibility boundary. Future storage changes should migrate or explicitly reject older records rather than interpreting the current Go structs as an implicit schema.

## Append ordering and tool durability

The transcript is canonical and append-only. For a persistent runtime, each new logical entry is written and synced through `session.Store.Append` before the in-memory transcript advances.

The normal ordering is therefore:

```text
persist user entry
append user entry in memory
call model
persist assistant entry
append assistant entry in memory
execute requested tool
persist tool result
append tool result in memory
```

For tool calls, the important boundary is:

```text
assistant requests tool
        │
        ▼
persist assistant tool call
        │
        ▼
append to in-memory transcript
        │
        ▼
execute tool
```

If persistence of the assistant tool call fails, the runtime stops before executing the tool.

If the process dies after a side-effecting tool executes but before its result becomes durable, a resumed transcript can contain the assistant tool call without a result. That is an interrupted call, not permission to silently execute the tool again. Recovery policy for such calls should be explicit when automatic resume workflows are added.

Slash commands are not model conversation entries, so invoking a slash command does not append a durable transcript record. Frontend lifecycle events and streamed chunks are also runtime projections rather than transcript facts.

## Transcript

`internal/transcript` owns in-memory logical history. Its entries deliberately have no system role.

Runtime instructions, repository `AGENTS.md`, Skill catalog text, controls, and other model-request context are not conversation history. The runtime converts completed provider results into transcript entries, while `internal/context` later selects transcript entries for the next model request.

The transcript stores provider-independent tool-call identity/name/arguments/result linkage rather than copying `provider.Message` or provider-native response objects.

## Context is a projection

The provider does not receive a session directly. `internal/context.Builder` materializes a model-facing request from the current transcript and runtime environment.

Conceptually:

```text
Context = Base + Checkpoints + Tail + Volatile
```

The current implementation only needs three explicit request regions:

```go
type Request struct {
    Instructions string
    Tail         []transcript.Entry
    Tools        []map[string]any
}
```

### Base instructions

At runtime creation, stable instructions are composed from:

1. kei's small built-in coding-agent instruction;
2. `<workspace>/AGENTS.md`, when present;
3. names and descriptions for discovered Agent Skills.

Only Skill catalog metadata is placed in the stable instructions. Full `SKILL.md` content and referenced resources remain progressively disclosed through the Skill tools.

A `before_model` control may replace instructions for one request. That replacement changes `context.Request.Instructions`; it does not rewrite the transcript or persisted session history.

### Tail

`context.Request.Tail` is the logical transcript region sent verbatim for the current model request.

There are no checkpoints yet, so the tail currently contains the whole loaded transcript. A cold-resumed named session therefore reconstructs provider context from portable history rather than depending on the previous provider process.

### Tools

`context.Request.Tools` currently contains the tool schemas visible for that turn after controls are applied.

A future optimization may distinguish a stable tool catalog from per-turn availability, but that distinction should only be added when a provider path can use it.

### Volatile context

There is no placeholder `Volatile` field today. Request-scoped behavior is represented by the actual selected request values. Add a separate volatile region when a real feature needs it.

## Provider boundary

The provider interface consumes the structured request as one unit:

```go
Generate(ctx, request, callback) (Result, error)
```

`internal/context` does not depend on provider wire structs. `internal/provider` decides how instructions, transcript entries, and tools become Anthropic, Gemini, OpenAI-compatible, or Codex objects.

Current transports may internally render the request into the older `provider.Message` shape, but that is not the session/context contract. A provider can later consume context regions directly when native cache breakpoints, continuation handles, or compacted state make that useful.

Provider-native objects must remain outside canonical transcript history.

## Compaction and checkpoints

Compaction must not rewrite or delete the durable transcript.

It creates derived checkpoint state covering an older transcript range:

```text
Transcript

A B C D E F G H I J
─────────┘
   covered by K1

Materialized context

Base | K1 | F G H I J
```

The original entries remain available for inspection, recovery, re-compaction, and projection through another provider.

A checkpoint may eventually be a portable summary, provider-native compacted state, or another derived representation. It should identify the transcript range it covers and any compatibility information needed to decide whether it can be reused.

Checkpoint schema and storage are intentionally not part of the version-1 session log yet.

## Stable prefixes and provider caches

Prompt caching is an optimization over materialized provider input, not session state.

When semantics permit it, context should evolve from least-changing to most-changing regions:

```text
Base | Checkpoint | append-only Tail | Volatile
```

A session ID is not a cache key. Different sessions may share stable prefixes, while one session may cross a compaction generation that changes its reusable prefix.

Likewise, provider continuation handles may be retained as accelerators later, but cold resume from portable transcript/checkpoint state must remain a normal path.

## Workspace semantics

`session.State.Workspace` is durable session metadata for named CLI sessions.

On resume, kei uses that path as the starting point for current runtime reconstruction:

- extension search roots;
- root `AGENTS.md`;
- project Agent Skills;
- tool and slash-command child-process cwd;
- control-process cwd.

The workspace path being durable does not make discovery results durable. The session file does not serialize extension descriptors, Skill contents, control chains, provider selection, or credentials.

## Concurrency

A logical session is one linear mutation stream.

The file store appends individual records, but that does not make concurrent model turns on one `session.State` semantically valid. Only one turn should mutate a logical session at a time.

The named CLI path is naturally serialized by its REPL/prompt loop. ACP still processes requests concurrently and remains in-memory. Before persistent ACP resume or multiple runtimes can share one session, kei needs per-session turn serialization rather than relying on a process map mutex or file append behavior.

## Current runtime flow

For ordinary user text, an `agent.Runtime` now:

1. requires `session.State`;
2. persists the user entry when a Store is attached, then appends it to `State.Transcript`;
3. collects current tool and Skill-reader schemas;
4. applies `before_model` controls;
5. materializes a `context.Request` containing selected instructions, transcript tail, and visible tools;
6. calls `Provider.Generate`;
7. persists the assistant entry, including any logical tool calls, then appends it in memory;
8. returns if there are no tool calls;
9. otherwise executes tools;
10. persists each tool result before appending it in memory and continuing the loop.

Without a Store, the same runtime path remains purely in-memory.

The loop is capped at 32 model turns per `Prompt` call. Tool execution errors are converted into tool-result text so the model can react on the next turn. Provider failures, malformed arguments, unknown tools, control failures, persistence failures, and approval plumbing failures stop the prompt.

## Implementation direction

The transcript, session/runtime, context-request, and first durable-store seams now exist. The next steps should remain independent:

1. add per-session turn serialization before multiple frontends/runtimes can mutate one logical session;
2. decide and implement ACP load/resume against the same logical Store contract rather than inventing ACP-specific history;
3. expose provider usage and cache telemetry;
4. add checkpoint storage beside, not inside, canonical transcript history;
5. extend context materialization to select compatible checkpoints plus the uncovered tail;
6. let provider adapters exploit native cache/continuation state where supported;
7. implement the first simple compaction strategy.

Do not make persistence depend on a compaction algorithm, and do not make compaction depend on one provider cache API.

## Invariants

1. A session is logical conversation state, not provider/runtime state.
2. Runtime dependencies are rebuilt around persisted logical state.
3. The canonical transcript is append-only.
4. The durable schema is explicit and versioned; Go structs are not an implicit file format.
5. Provider transport structs are not canonical transcript state.
6. Runtime instructions are not transcript history.
7. Persistent tool calls become durable before tool execution.
8. Context is a projection of transcript plus the current runtime environment.
9. Provider adapters receive structured context regions before transport rendering.
10. Compaction creates replaceable checkpoints rather than rewritten history.
11. Provider continuations and caches are accelerators, never sources of truth.
12. A session has one linear mutation stream at a time.
