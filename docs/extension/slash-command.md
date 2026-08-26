# Slash commands

Slash commands are human-facing routes from a frontend to ordinary processes. They are declared in an extension's `commands.json` and intercepted before normal model prompting when the command is discovered.

They are not prompts with special syntax.

## File shape

```json
{
  "commands": [
    {
      "name": "status",
      "description": "Show the concise Git working tree status.",
      "command": "git",
      "args": ["status", "--short"]
    },
    {
      "name": "inspect",
      "description": "Inspect a symbol.",
      "input_hint": "symbol name",
      "command": "./commands/inspect",
      "args": ["{arguments?}"]
    }
  ]
}
```

## Descriptor fields

| field | required | meaning |
| --- | --- | --- |
| `name` | yes | local command name within the extension |
| `description` | no in parser | human-facing description used by help/frontends |
| `input_hint` | no | short hint for argument UI such as ACP command completion |
| `command` | yes | executable or command name |
| `args` | no | argv template using the complete argument string |
| `stdin` | no | currently `text` or empty |
| `timeout_ms` | no | process timeout; default is 60 seconds |

The extension loader derives the extension ID, qualified name, and extension base directory.

## Identity and invocation

For extension `unix` and local command `status`, the canonical registry identity is:

```text
unix:status
```

The user invokes it as:

```text
/unix:status
```

Arguments are everything after the first space:

```text
/astrolabe:inspect Foo.Bar
```

is parsed as:

```text
name      = astrolabe:inspect
arguments = Foo.Bar
```

Leading/trailing whitespace around the complete input and around the extracted argument tail is trimmed.

## Only discovered commands are special

`ParseInvocation` recognizes slash-prefixed syntax, but the session only takes the direct-command path when the parsed name exists in the discovered command registry.

Therefore:

```text
/unknown something
```

remains ordinary user input if no `unknown` command is discovered.

This is important for preserving general prompt text and avoiding a global reservation of every leading `/` string.

The built-in REPL also has its own local commands such as `/help`, `/model`, `/models`, `/exit`, and `/quit`; extension command names are separately namespaced.

## Argument templates

Slash commands intentionally expose one raw human argument string instead of a structured JSON object.

Supported placeholders are:

```text
{arguments}
{arguments?}
```

`{arguments}` requires a non-empty argument tail and returns an error when none was supplied.

`{arguments?}` is omitted from argv when the argument tail is empty.

Both placeholders pass the entire argument tail as **one** argv element. `kei` does not shell-split the user's text.

For example:

```json
{
  "args": ["--query", "{arguments}"]
}
```

with:

```text
/foo:search hello world
```

executes with argv equivalent to:

```text
--query
hello world
```

where `hello world` is one argument.

All other `args` entries are literal.

## Text stdin mode

A command can receive the complete argument tail on stdin instead:

```json
{
  "name": "review",
  "command": "./commands/review",
  "stdin": "text"
}
```

When `stdin` is `text`, the argument string is written verbatim to stdin after invocation parsing/trimming.

argv may still be used at the same time.

## Executable resolution and cwd

The same rules as tools apply:

- `git` resolves through `PATH`;
- `./commands/inspect` resolves relative to the extension root;
- absolute paths stay absolute;
- the child process runs with the session workspace as cwd.

This allows a packaged command helper to live inside the extension while naturally operating on the active repository.

## Timeouts, stdout, and failures

The default timeout is 60 seconds unless `timeout_ms` is positive.

stdout is returned as the command result. stderr is attached to the Go error when a command fails and stderr is non-empty.

Unlike model tool failures, a slash-command execution error is returned directly from `Session.Prompt`; it is not converted into a tool-result message because no model tool-call loop is involved.

## Frontend behavior

### REPL

Discovered commands appear in `/help` and can be invoked directly.

### Non-interactive CLI

A slash command can be supplied through:

```sh
kei run -p '/unix:status'
```

### ACP

After `session/new`, the ACP adapter advertises the session's discovered commands through `available_commands_update`.

Each advertised entry uses the qualified name without the leading slash, for example:

```text
unix:status
```

and includes `description` plus an input hint when `input_hint` is set.

## When to use a slash command instead of a tool

Use a slash command when the capability is primarily a direct human action or shortcut and should bypass model interpretation.

Use a tool when the model should decide when to invoke the operation from a structured input schema.

The same executable may back both. Keeping two descriptors is often preferable to inventing one abstraction that conflates human argument text with model JSON input.
