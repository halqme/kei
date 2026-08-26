# Extensions

An extension is a namespace and distribution unit for capabilities exposed through explicit declarations. It is not an in-process plugin loaded into `kei`.

An extension may contain tools, slash commands, executable helpers, or only declarations pointing at commands already installed on the host.

## Layout

```text
extensions/<id>/
├── tools.json       # optional
├── commands.json    # optional
├── tools/           # optional convention for extension-owned tool executables
└── commands/        # optional convention for extension-owned command executables
```

The `tools/` and `commands/` directories are conventions only. `kei` does not inspect them to synthesize descriptors and does not require any implementation language.

For example:

```text
.kei/extensions/unix/
├── tools.json
└── commands.json
```

can expose `rg` and `git` directly through `PATH` without shipping any executable files.

## Search roots

For a workspace, `kei` builds extension roots in precedence order:

1. `<workspace>/.kei/extensions`
2. `$XDG_DATA_HOME/kei/extensions`, or `~/.local/share/kei/extensions` when `XDG_DATA_HOME` is unset
3. each `$XDG_DATA_DIRS` entry with `/kei/extensions` appended; when unset this defaults to `/usr/local/share/kei/extensions:/usr/share/kei/extensions`
4. each configured `extension_dirs` entry

Configured `extension_dirs` entries support `~/` expansion. Relative configured roots are resolved against the workspace.

Duplicate resulting directories are removed while preserving first occurrence.

Missing roots are silently skipped. Other directory-read errors are returned.

## Discovery

Each immediate non-hidden child directory of a search root is an extension candidate. Entries are sorted by name before loading so discovery is deterministic within a root.

Hidden child directories, whose names begin with `.`, are ignored.

The directory name is the extension ID.

### Whole-extension shadowing

The first root that contains an extension ID wins. Once an ID has been seen, lower-precedence copies are ignored entirely.

Example:

```text
repo/.kei/extensions/git/tools.json
~/.local/share/kei/extensions/git/commands.json
```

The workspace `git` extension shadows the user `git` extension as a whole. `kei` does **not** combine the workspace tools with the user commands.

This rule makes workspace overrides predictable and keeps one extension copy internally coherent.

## Declaration files

`tools.json` has the shape:

```json
{
  "tools": []
}
```

`commands.json` has the shape:

```json
{
  "commands": []
}
```

Both files are optional. Invalid JSON, duplicate local names inside one declaration file, or entries missing required names/commands cause discovery to fail.

See [Tools](tool.md) and [Slash commands](slash-command.md) for descriptor details.

## Names

A local tool name is qualified with the extension ID:

```text
<extension>.<tool>
```

For example:

```text
unix.search_text
```

A local slash command is qualified as:

```text
<extension>:<command>
```

and invoked by a user with a leading slash:

```text
/unix:status
```

Tool providers may impose function-name restrictions. `kei` therefore also derives a model-facing tool name by sanitizing the extension and tool components and joining them with `_`:

```text
unix_search_text
```

The qualified dotted name remains the canonical identity inside `kei`; the underscore form is a provider-facing representation.

The tool registry accepts either representation when resolving a model call.

## Executable resolution

Declarations preserve a distinction between **where the executable lives** and **where it runs**.

A command without a path separator, for example:

```json
"command": "rg"
```

is left unchanged and resolved through the normal `PATH` rules of `os/exec`.

A relative command containing a path separator, for example:

```json
"command": "./tools/symbol"
```

is resolved relative to the extension root.

However, the spawned process runs with the session workspace as its cwd.

This means an extension can ship its executable next to its descriptors while that executable naturally sees the repository being worked on as `.`.

Absolute command paths remain absolute.

## Distribution

Because extensions are files plus processes, installation can use ordinary distribution mechanisms.

A package manager can install a command on `PATH` and descriptors under a data prefix:

```text
<prefix>/bin/foo
<prefix>/share/kei/extensions/foo/tools.json
<prefix>/share/kei/extensions/foo/commands.json
```

Or it can keep helpers private to the extension directory:

```text
<prefix>/share/kei/extensions/foo/
├── tools.json
├── commands.json
├── tools/analyze
└── commands/review
```

Homebrew, Nix, Cargo, npm, pipx, `go install` plus copied descriptors, a repository checkout, or plain filesystem deployment can all fit this model. `kei` does not need to become the package manager.

## Why descriptors are explicit

Normal discovery does not inspect `--help`, man pages, shell completions, or binaries to infer an agent interface.

Human-facing CLI metadata usually cannot reliably express:

- a useful model-facing operation boundary;
- JSON input shape;
- defaults and optionality;
- side-effect metadata;
- which parts of a large CLI should be exposed;
- the desired slash-command UX.

An explicit descriptor is small enough to maintain and makes the agent-facing contract reviewable.

## Inspection

Use:

```sh
kei extensions
kei tools
kei commands
```

or their `-json` variants to inspect what a workspace discovers.

Use `kei exec <extension.tool>` to test a tool descriptor and process directly without involving a model.

## Controls

Controls are related to the extension philosophy because they are also external processes, but the current implementation configures them in `config.json` rather than discovering `controls.json` from extension directories.

See [Controls](control.md) for the current protocol and status.
