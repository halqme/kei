# Connect

This page covers the shortest path from a fresh build to a working `kei` session.

## Build

```sh
go build -o ./build/kei ./cmd/kei
```

The resulting binary has no extension runtime dependency beyond whatever provider credentials and extension executables you choose to use.

## Authenticate

`kei login` stores credentials separately from normal configuration.

```sh
./build/kei login
```

shows the supported login targets. Current login commands are:

```sh
./build/kei login codex
./build/kei login openai
./build/kei login anthropic
./build/kei login gemini
```

For Codex, the normal flow uses browser OAuth PKCE. Headless or SSH environments can request device-code authentication:

```sh
./build/kei login codex -device
```

`-browser` forces the browser flow. `-out <path>` can override the credential output path for a login command.

API-key providers can also use environment variables instead of `kei login`:

```text
openai      OPENAI_API_KEY
anthropic   ANTHROPIC_API_KEY
gemini      GEMINI_API_KEY or GOOGLE_API_KEY
azure       AZURE_OPENAI_API_KEY
codex       CODEX_ACCESS_TOKEN (and compatibility fallbacks)
```

Ollama is treated as local and does not require authentication.

## Credential storage

The primary auth file is:

```text
$KEI_AUTH_FILE                         when set
$XDG_STATE_HOME/kei/auth.json          otherwise when XDG_STATE_HOME is set
~/.local/state/kei/auth.json           default fallback
```

The auth store also searches the XDG data location as a compatibility read path. Codex can additionally reuse native `~/.codex/auth.json` credentials.

Auth files are separate from `config.json`. Do not put access tokens into normal configuration merely because a provider target contains API settings.

## Start a session

```sh
./build/kei run
```

With no `-p`, `kei run` opens the built-in REPL. With `-p`, it runs one prompt and exits:

```sh
./build/kei run -p 'Summarize this repository.'
```

A discovered slash command can be passed the same way:

```sh
./build/kei run -p '/examples:status'
```

The REPL also has built-ins such as `/help`, `/model`, `/models`, `/exit`, and `/quit`. Discovered slash commands are listed by `/help`.

## Provider target selection

A provider type such as `openai` or `anthropic` is not necessarily the thing selected by `kei run`. The selectable object is a **connection target** from the ordered `providers` configuration list.

For example:

```json
{
  "providers": [
    {
      "name": "claude",
      "type": "anthropic",
      "model": "claude-3-7-sonnet-20250219"
    },
    {
      "name": "local",
      "type": "ollama",
      "model": "llama3.3"
    }
  ]
}
```

Without an override, the first configured target is selected. Choose another target with:

```sh
./build/kei run -provider local
```

Override the model with `-m`:

```sh
./build/kei run -m fast
./build/kei run -provider claude -m claude-3-7-sonnet-20250219
```

`-m` accepts either a model name or an alias from the `models` map in configuration.

## First-run configuration

A normal session path may create a user configuration when none exists, using currently available authenticated provider types as candidates. An explicitly supplied missing `-config` path is never created implicitly.

Inspection commands such as `kei models`, `kei extensions`, `kei tools`, and `kei commands` are intended to inspect state, not create configuration as a side effect.

See [Configuration](configuration.md) for lookup order and the full schema.

## Inspect before running

Useful inspection commands are:

```sh
./build/kei models
./build/kei extensions
./build/kei tools
./build/kei commands
```

Each supports `-config <path>`; the inspection commands also support `-json`.

To directly exercise a discovered tool without involving a model:

```sh
./build/kei exec -input '{"pattern":"Unix-native","path":"."}' examples.search_text
```

Direct execution is useful when developing an extension because it separates descriptor/process problems from provider and agent-loop behavior.

## Use an ACP client

Run:

```sh
./build/kei acp
```

or choose a target/model:

```sh
./build/kei acp -provider claude -m fast
```

ACP uses stdin/stdout as its transport. See [ACP](acp.md) for the implemented protocol surface and how session events are projected.
