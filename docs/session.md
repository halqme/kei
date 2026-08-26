# Sessions

A `kei` session is the provider-independent orchestration layer between a frontend, a model provider, discovered capabilities, and controls.

The central type is `internal/agent.Session`.

## Session state

A session currently carries:

- a session ID
- one selected provider implementation
- the discovered tool registry
- the discovered slash-command registry
- an optional control chain
- the system prompt
- the workspace directory
- conversation messages
- an optional approval callback
- an optional event callback used by frontends

The session owns conversation history. Frontends do not maintain a separate model transcript and provider transports do not own orchestration policy.

## Prompt path

For ordinary user text, the session:

1. appends the system message on the first model turn when a system prompt exists;
2. appends the user message;
3. applies `before_model` controls;
4. determines the visible tool set;
5. calls `Provider.Stream`;
6. appends the provider's completed assistant message;
7. returns text if there are no tool calls;
8. otherwise executes requested tools, appends tool-result messages, and repeats.

The loop is currently capped at 32 model turns for one `Prompt` call. Exceeding that limit returns `agent exceeded maximum turns`.

## Slash-command path

Before appending user text to model history, the session asks `internal/command.ParseInvocation` whether the text looks slash-prefixed.

A slash prefix alone is not enough to bypass the model. The parsed name must exist in the discovered command registry.

If it exists, the command process is executed directly and its output is returned. The slash command is not rewritten into a prompt and is not appended to model history.

If it does not exist, the original text continues through the normal prompt path. This keeps `/path`, `/some-unknown-command`, and other slash-prefixed user text available to the model.

## Tool-call path

For each provider tool call:

1. resolve the provider-facing tool name through the tool registry;
2. decode the function arguments as a JSON object;
3. emit a `tool_start` event;
4. apply `before_tool` controls;
5. deny, request approval, or continue according to the control decision;
6. execute the tool process;
7. emit `tool_end`;
8. append a tool-result message using the provider tool-call ID;
9. invoke `after_tool` controls.

Tool execution errors do not automatically terminate the whole session. The error is converted into tool-result text prefixed with `ERROR:` and returned to the model, allowing the model to react on the next turn.

By contrast, failures in provider calls, control execution, malformed tool arguments, unknown requested tools, and approval plumbing are session errors and stop the prompt.

## Controls around a session

### `before_model`

A control receives the session ID, current system prompt, and workdir. It may:

- replace the system prompt;
- hide tools from the next provider call;
- return an action, although model-level action handling is currently limited compared with tool-level handling.

Hidden tools are filtered from the provider-facing tool list for that turn. Qualified names such as `unix.search_text` are also matched against their model-facing underscore representation.

### `before_tool`

The event contains the qualified tool name, declared `effects`, decoded input, session ID, and workdir.

`deny` appends a denied tool result and continues the agent loop.

`ask` calls the session's approval callback. If no approval frontend is installed, the session returns an error. A user rejection is appended as `Denied by user` and the loop continues.

An empty action or `allow` permits execution.

### `after_tool`

The current session invokes `after_tool` after execution, but ignores both the returned decision and errors from that call. Treat this as the current implementation contract rather than assuming symmetrical semantics with `before_tool`.

## Provider streaming

The provider boundary is:

```go
Stream(ctx, messages, tools, callback) (Result, error)
```

It returns a completed `Result` and can emit `StreamEvent` values while producing it.

The only generic stream event currently defined is `text_delta`. When a session has an event callback, text deltas are forwarded as:

```text
assistant_message_chunk
```

Provider support differs. Codex consumes an SSE transport incrementally; other provider implementations may emit their completed text as a single delta. Code outside a provider should rely on the provider interface, not assumptions about one transport.

## Process output

Tools and slash commands currently buffer stdout and stderr until the child process exits. Successful stdout becomes the result. On failure, stderr is included in the returned Go error when present.

Controls also execute synchronously and buffer output because their protocol is currently one JSON event in and one JSON decision out.

This does not prevent a future process-streaming contract, but introducing one requires an explicit session/frontend protocol rather than simply changing `os/exec` plumbing.

## Events exposed to frontends

The session emits generic lifecycle events through `OnEvent`:

```text
command_start
command_end
tool_start
tool_end
assistant_message_chunk
```

Payloads are plain maps/values owned by the session layer. Frontends such as ACP translate these into their own wire representation.

## Workspace semantics

`Session.Workdir` serves two related purposes:

- it is the cwd of tool and slash-command child processes;
- it is included in control events and used by control child processes as cwd.

Extension-owned relative executable paths are resolved earlier relative to the extension root; that does not change the child cwd. See [Extensions](extension/index.md) for the distinction.

ACP sessions discover workspace-local extensions from each session cwd, so discovery is session/workspace scoped rather than one global registry shared by all ACP sessions.
