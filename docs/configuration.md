# Configuration

`kei` separates ordinary connection/runtime configuration from authentication credentials.

Normal configuration is JSON and is suitable for dotfiles or workspace-local settings as long as secrets are kept out of it.

## Lookup order

When no explicit `-config` path is supplied, `kei` checks configuration paths in this order, skipping duplicates:

1. `$KEI_CONFIG`, when set
2. `<workspace>/.kei/config.json`
3. `$XDG_CONFIG_HOME/kei/config.json`, when `XDG_CONFIG_HOME` is absolute
4. `~/.config/kei/config.json`

An explicit `-config <path>` is authoritative. If that path does not exist, session startup returns an error rather than creating it.

When a normal session starts without any existing config, `kei` may create the default user config. New configuration directories/files use owner-only permissions (`0700` directory, `0600` file).

## Schema

The current top-level shape is:

```json
{
  "providers": [],
  "models": {},
  "extension_dirs": [],
  "controls": [],
  "system_prompt": "..."
}
```

All fields are optional.

### `providers`

`providers` is an ordered array of named connection targets.

```json
{
  "providers": [
    {
      "name": "openai",
      "type": "openai",
      "model": "gpt-5.6",
      "models": ["gpt-5.6", "gpt-5.5-mini"]
    },
    {
      "name": "local",
      "type": "ollama",
      "base_url": "http://localhost:11434/v1",
      "model": "llama3.3"
    }
  ]
}
```

Each target supports:

| field | meaning |
| --- | --- |
| `name` | connection-target name used by `-provider`; if omitted, the provider type acts as the name |
| `type` | provider implementation such as `openai`, `codex`, `anthropic`, `gemini`, `ollama`, or `azure` |
| `base_url` | optional provider endpoint override |
| `api_key_env` | optional environment variable to check before the provider's standard key variable |
| `model` | default model for this target |
| `models` | optional advertised/allowed-looking model list used for inspection and defaulting; the first item is used when `model` is empty |

The first configured target is selected when no `-provider` override is supplied.

Target names are compared case-insensitively after trimming whitespace. An unknown target is an explicit error and the error lists configured target names.

There is no silent reinterpretation of an unknown provider type. `internal/provider` returns an unsupported-provider error including available provider types.

### Provider aliases

The provider factory currently recognizes canonical providers and compatibility aliases. The canonical list exposed by `provider.List` is normalized to names such as:

```text
anthropic
azure
codex
gemini
ollama
openai
```

Compatibility aliases such as `claude`, `google`, `openai-compatible`, `openai-codex`, and `azure-openai` map to their canonical implementation.

### `models`

`models` is a map of model aliases:

```json
{
  "models": {
    "fast": "gpt-5.5-mini",
    "smart": "claude-3-7-sonnet-20250219"
  }
}
```

`kei run -m fast` resolves the alias before applying the model override to the selected target.

The map is intentionally global rather than tied to one provider. It is the caller's responsibility to choose an alias meaningful for the selected target.

### `extension_dirs`

`extension_dirs` appends extra extension search roots after workspace and XDG roots:

```json
{
  "extension_dirs": [
    "./examples/extensions",
    "~/src/my-kei-extensions"
  ]
}
```

`~/` is expanded. Relative entries are resolved against the session workspace. Search order matters because the first copy of an extension ID shadows later copies.

See [Extensions](extension/index.md) for the complete discovery contract.

### `controls`

Controls are currently configured as process commands:

```json
{
  "controls": [
    {
      "command": "./bin/policy",
      "args": ["--profile", "safe"]
    }
  ]
}
```

Each control receives JSON on stdin and returns a decision as JSON on stdout. Controls are executed in array order. See [Controls](extension/control.md) for event and decision shapes.

Unlike tools/commands, controls are **not yet extension declarations**. The docs live under `extension/` because controls participate in extensibility, but the current schema belongs to `config.json`.

### `system_prompt`

`system_prompt` sets the initial session system prompt.

The default is:

```text
You are a coding agent. Use tools when they help you complete the task.
```

A `before_model` control can replace the active system prompt for subsequent model calls.

## Authentication is separate

Credentials are not part of the `Config` struct and should not be written into `config.json`.

The auth store writes to `$KEI_AUTH_FILE`, `$XDG_STATE_HOME/kei/auth.json`, or the default `~/.local/state/kei/auth.json`. Provider factories also check provider-specific environment variables.

The general precedence for API-key resolution is:

1. an API key explicitly supplied to provider construction internally
2. the target's `api_key_env`
3. the provider's standard environment variable
4. `kei` auth-store credentials

Some providers have additional compatibility sources; Codex, for example, can reuse native Codex credentials.

## Empty provider configuration

`Config.ResolveProvider` itself requires at least one target. Session startup has an earlier fallback path that can synthesize targets from authenticated provider types when `providers` is empty.

That distinction is intentional: configuration resolution remains explicit, while the session command can provide first-run ergonomics.

Read-only inspection should not become a hidden configuration mutation merely to reuse session startup logic.

## Example

```json
{
  "providers": [
    {
      "name": "codex",
      "type": "codex",
      "model": "gpt-5.5",
      "models": ["gpt-5.5", "gpt-5.5-mini"]
    },
    {
      "name": "local",
      "type": "ollama",
      "model": "llama3.3"
    }
  ],
  "models": {
    "fast": "gpt-5.5-mini"
  },
  "extension_dirs": ["./examples/extensions"],
  "system_prompt": "You are a coding agent. Prefer small, verifiable changes."
}
```

Keep secrets out of this file; use `kei login` or environment variables instead.
