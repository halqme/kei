# Tools

Tools are model-facing operations declared by an extension in `tools.json`.

A tool descriptor tells `kei` two things: the function contract shown to the model and the process invocation used when that function is called.

## File shape

```json
{
  "tools": [
    {
      "name": "search_text",
      "description": "Search text in the workspace.",
      "input_schema": {
        "type": "object",
        "properties": {
          "pattern": {"type": "string"},
          "path": {"type": "string", "default": "."}
        },
        "required": ["pattern"]
      },
      "command": "rg",
      "args": ["--line-number", "{pattern}", "{path}"],
      "effects": ["filesystem.read"]
    }
  ]
}
```

## Descriptor fields

| field | required | meaning |
| --- | --- | --- |
| `name` | yes | local name within the extension |
| `description` | no in parser, expected for model UX | human/model-facing description |
| `input_schema` | no in parser | JSON-Schema-like object passed to providers as function parameters |
| `command` | yes | executable or command name |
| `args` | no | argument template |
| `stdin` | no | currently `json` or empty |
| `timeout_ms` | no | per-invocation timeout; default is 60 seconds |
| `effects` | no | policy/UX metadata made available to controls |

Discovery also attaches internal metadata such as extension ID, qualified name, provider-facing model name, and extension base directory. Those fields are derived and should not be authored as part of a normal descriptor.

## Identity

For extension `unix` and local tool `search_text`, the canonical name is:

```text
unix.search_text
```

The provider-facing name is derived as:

```text
unix_search_text
```

Characters outside letters, digits, `_`, and `-` are replaced with `_` when generating provider-facing names. The registry rejects collisions between derived model names, so two canonical names cannot silently collapse to the same provider function.

Tool listings are sorted by qualified name for deterministic output.

## Provider schema

`kei` presents tools to providers in an OpenAI-style function shape:

```json
{
  "type": "function",
  "function": {
    "name": "unix_search_text",
    "description": "Search text in the workspace.",
    "parameters": {}
  }
}
```

Provider transports translate that representation as needed. The tool package owns the stable internal descriptor; provider packages own wire conversion details.

## Argument templates

Each `args` element is either a literal argument or one whole-field placeholder.

Required placeholder:

```text
{name}
```

Optional placeholder:

```text
{name?}
```

A required placeholder fails before process execution when its input value is missing, null, or stringifies to an empty string.

An optional placeholder is omitted entirely when the value is absent/empty.

Placeholders must occupy the whole argv element. This is supported:

```json
["--line-number", "{pattern}", "{path}"]
```

This is treated as a literal string, not interpolation:

```json
["--path={path}"]
```

The intentionally small template language avoids shell parsing and quoting semantics.

## Defaults

Before argv expansion, `kei` applies top-level defaults from `input_schema.properties`.

For:

```json
{
  "input_schema": {
    "type": "object",
    "properties": {
      "path": {"type": "string", "default": "."}
    }
  }
}
```

an omitted `path` becomes `.` before `{path}` is expanded.

Existing input values always win over schema defaults.

The current executor applies defaults but does not implement a complete JSON Schema validator. Providers may enforce more of the input schema before returning a tool call, but tool code should not assume that every JSON-Schema keyword has been validated locally by `kei`.

## Arrays

When a placeholder value is decoded as a JSON array, each array item becomes a separate argv element.

For example, with:

```json
"args": ["{files}"]
```

and input:

```json
{"files": ["a.go", "b.go"]}
```

execution receives two arguments: `a.go` and `b.go`.

## JSON stdin mode

A process designed specifically for agent input can skip argv templates and request the complete object on stdin:

```json
{
  "name": "analyze",
  "command": "./tools/analyze",
  "stdin": "json",
  "input_schema": {
    "type": "object"
  }
}
```

After defaults are applied, the input object is JSON-encoded and written to stdin.

`args` can still be present; stdin mode and argv are independent parts of the process invocation.

## Executable path and cwd

`command: "rg"` is resolved through `PATH`.

`command: "./tools/analyze"` is resolved relative to the extension root.

In both cases the child process itself runs with the session workspace as cwd.

See [Extensions](index.md) for the full resolution rule.

## Timeouts and cancellation

Every tool execution runs under a context timeout.

- `timeout_ms > 0` uses the configured duration.
- otherwise the default is 60 seconds.

Cancellation of the parent session context also cancels the command through `exec.CommandContext`.

On timeout/cancellation, any stdout captured before termination is returned alongside the context error.

## stdout, stderr, and failures

stdout is the tool result.

stderr is not included in successful results. When the process exits unsuccessfully and stderr is non-empty, stderr text is attached to the returned error.

Inside a normal agent session, a tool execution error is converted into a tool-result message beginning with `ERROR:` so the model can see the failure and continue reasoning.

`kei exec`, by contrast, exposes direct execution behavior to the caller.

## Effects

`effects` is an array of free-form strings such as:

```json
"effects": ["filesystem.read"]
```

The session includes these values in `before_tool` and `after_tool` control events.

Effects are descriptive metadata. They are not enforced by the OS and do not provide sandboxing. A security boundary must be implemented by a control/sandbox process or operating-system isolation.

## Direct testing

Use `kei exec` while developing descriptors:

```sh
kei exec -input '{"pattern":"harness","path":"."}' unix.search_text
```

This exercises discovery, descriptor lookup, defaults, argv/stdin construction, executable resolution, cwd, timeout, and process-result handling without involving a provider or model.
