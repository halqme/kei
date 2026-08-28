# Controls

Controls are external policy processes invoked around model and tool execution. They let `kei` expose general decision points without hard-coding higher-level modes into the agent loop.

The current implementation configures controls in `config.json`; they are not yet discovered from extension directories.

## Configuration

```json
{
  "controls": [
    {
      "command": "./bin/policy",
      "args": ["--profile", "default"]
    }
  ]
}
```

Each configured control is executed in order for each control event.

The command is currently used as configured. Unlike extension tool/command descriptors, a configured control does not carry an extension base directory or the extension-relative executable resolution rule.

When the event contains a workdir, the control process runs with that directory as cwd.

## Protocol

One event is JSON-encoded and written to stdin. A control may write no output, meaning no opinion, or write one JSON decision to stdout.

### Event

The current event shape is:

```json
{
  "kind": "before_tool",
  "session_id": "kei-1",
  "tool": "unix.search_text",
  "effects": ["filesystem.read"],
  "input": {"pattern": "foo"},
  "system_prompt": "...",
  "workdir": "/path/to/repo"
}
```

Fields are omitted when not relevant to the event.

| field | meaning |
| --- | --- |
| `kind` | control point such as `before_model`, `before_tool`, or `after_tool` |
| `session_id` | current session identity when available |
| `tool` | canonical qualified tool name for tool events |
| `effects` | descriptor-declared effect metadata |
| `input` | decoded tool input object |
| `system_prompt` | current system prompt for model events |
| `workdir` | session workspace |

### Decision

A control can return:

```json
{
  "action": "ask",
  "reason": "This tool may modify files.",
  "system_prompt": "optional replacement prompt",
  "hidden_tools": ["unix.remove_file"]
}
```

Current fields are:

| field | meaning |
| --- | --- |
| `action` | `allow`, `deny`, `ask`, or empty for no opinion |
| `reason` | explanation used particularly for deny/approval flows |
| `system_prompt` | replacement system prompt |
| `hidden_tools` | qualified/model-facing tool names to hide from a model call |

## Chain semantics

Controls are applied in configuration order.

For each decision:

- a non-empty `system_prompt` replaces the accumulated prompt and is passed to later controls in the same chain;
- `hidden_tools` are appended to the accumulated hidden set;
- a non-empty `action` and `reason` replace the accumulated action/reason;
- `deny` and `ask` short-circuit the chain immediately;
- `allow` does not short-circuit, so later controls can still contribute or override.

Invalid JSON, a failed control process, or a non-zero exit causes the control chain to return an error.

## `before_model`

The session applies `before_model` before each provider call.

A returned `system_prompt` replaces the instructions used when the context builder materializes that provider request. It does not rewrite the session transcript or mutate the builder's stable base. Returned `hidden_tools` are removed from the tool list presented to the provider for that model turn.

The current session primarily consumes prompt/tool-visibility changes from `before_model`; do not assume `deny`/`ask` has a fully symmetrical model-level workflow.

## `before_tool`

Before executing a model-requested tool, the session sends the canonical tool name, effects metadata, input object, session ID, and workdir.

### `deny`

The tool process is skipped. The session appends a tool result:

```text
Denied: <reason>
```

and continues the model loop.

### `ask`

The tool process is skipped until the session's approval callback responds.

If no approval callback is available, the prompt fails with an error stating that approval is required but no approval frontend is available.

If the user rejects the request, the session appends:

```text
Denied by user
```

and continues the model loop.

### `allow` or empty

Execution continues normally.

## `after_tool`

The session currently invokes the control chain after a tool executes, but ignores the returned decision and ignores errors from the `after_tool` application.

That behavior is intentionally documented because it is easy to assume a stronger post-execution contract than the code currently provides. If `after_tool` becomes a reliable auditing/enforcement hook, the session semantics and tests must be tightened explicitly.

## Effects are metadata

A tool may declare:

```json
"effects": ["filesystem.read", "network"]
```

These strings are passed to controls but are not independently verified by `kei`.

A dishonest or buggy descriptor can misstate its effects. Therefore effects can drive UX and policy heuristics, but they are not a sandbox or security boundary.

Strong isolation should be implemented at the OS boundary: sandbox wrappers, containers, namespaces, macOS sandbox profiles, capability-limited helper processes, or other mechanisms appropriate to the host.

## Building modes outside the core

Controls are the intended primitive for behaviors often exposed as named agent modes.

For example, a conservative mode might combine:

- a `before_model` control that hides write-capable tools;
- a `before_tool` control that returns `ask` for selected effects;
- a system-prompt adjustment;
- frontend state showing the active policy.

A permissive mode could use the same control points with a different process/configuration.

The goal is for Plan/YOLO/Review-like behavior to be compositions over generic hooks rather than enums and branches embedded into `internal/agent`.

## Current status

Controls are process-oriented but still early:

- they are configured globally through `config.json` rather than extension declarations;
- the protocol is synchronous request/response JSON;
- `after_tool` is best-effort in the current session implementation;
- ACP-native approval round-trips are not yet robust;
- there is no built-in sandbox implied by a control decision.

Future control declaration/distribution can evolve without changing the core idea that policy lives behind an external process boundary.
