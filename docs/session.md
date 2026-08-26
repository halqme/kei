# Sessions

A `kei` session is the provider-independent orchestration layer between a frontend, a model provider, discovered capabilities, workspace instructions, Agent Skills, and controls.

The central type is `internal/agent.Session`.

## Session state

A session currently carries:

- a session ID
- one selected provider implementation
- the discovered tool registry
- the discovered slash-command registry
- the discovered Agent Skill registry
- an optional control chain
- the assembled system prompt
- the workspace directory
- conversation messages
- an optional approval callback
- an optional event callback used by frontends

The session owns conversation history. Frontends do not maintain a separate model transcript and provider transports do not own orchestration policy.

## Instructions

Session startup assembles one system prompt from:

1. kei's small built-in coding-agent prompt;
2. the name and description of each discovered Agent Skill;
3. `<workspace>/AGENTS.md`, when present.

`AGENTS.md` is read from the workspace root. Nested `AGENTS.md` scoping is not part of the current contract.

Natural-language instructions are intentionally not a `config.json` field. Project instructions therefore have one filesystem surface rather than competing with a `system_prompt` setting.

## Agent Skills

`kei` follows the Agent Skills `SKILL.md` format rather than defining a kei-specific Skill descriptor. The current search roots, in precedence order, are:

1. `<workspace>/.agents/skills`
2. `~/.agents/skills`

Each immediate non-hidden child directory is a Skill candidate. A candidate without `SKILL.md` is skipped. Required `name` and `description` metadata are validated during discovery, and the Skill name must match its parent directory. When both roots contain the same Skill name, the workspace copy wins.

Only Skill names and descriptions are placed in the initial system prompt. This preserves the progressive-disclosure model of Agent Skills. When a task matches a Skill, the model can call the built-in `load_skill` tool to read its complete `SKILL.md`, then `read_skill_resource` for referenced files under that Skill directory as needed.

Those built-in readers are read-only and reject resource paths that escape the Skill root. They participate in the same tool lifecycle events and control checks as extension tools.

The format and directory conventions are documented by the Agent Skills project:

- <https://agentskills.io/specification>
- <https://agentskills.io/client-implementation/adding-skills-support>

## Prompt path

For ordinary user text, the session:

1. appends the assembled system message on the first model turn;
2. appends the user message;
3. applies `before_model` controls;
4. determines the visible extension and Skill-loader tool set;
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

For each provider tool call, whether it resolves to an extension tool or a built-in Skill reader, the session:

1. resolves the provider-facing tool name;
2. decodes the function arguments as a JSON object;
3. emits a `tool_start` event;
4. applies `before_tool` controls;
5. denies, requests approval, or continues according to the control decision;
6. executes the operation;
7. emits `tool_end`;
8. appends a tool-result message using the provider tool-call ID;
9. invokes `after_tool` controls.

Tool execution errors do not automatically terminate the whole session. The error is converted into tool-result text prefixed with `ERROR:` and returned to the model, allowing the model to react on the next turn.

By contrast, failures in provider calls, control execution, malformed tool arguments, unknown requested tools, and approval plumbing are session errors and stop the prompt.

## Controls around a session

### `before_model`

A control receives the session ID, current assembled system prompt, and workdir. It may:

- replace the system prompt;
- hide tools from the next provider call;
- return an action, although model-level action handling is currently limited compared with tool-level handling.

Hidden tools are filtered from the provider-facing tool list for that turn. Qualified extension names such as `unix.search_text` are also matched against their model-facing underscore representation. Built-in Skill reader names can be hidden directly.

### `before_tool`

The event contains the canonical tool name, declared `effects` when applicable, decoded input, session ID, and workdir.

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

`Session.Workdir` serves several related purposes:

- it selects the root `AGENTS.md` and project `.agents/skills` directory;
- it is the cwd of tool and slash-command child processes;
- it is included in control events and used by control child processes as cwd.

Extension-owned relative executable paths are resolved earlier relative to the extension root; that does not change the child cwd. See [Extensions](extension/index.md) for the distinction.

ACP sessions discover workspace-local extensions, instructions, and Skills from each session cwd, so these inputs are session/workspace scoped rather than one global registry shared by all ACP sessions.
