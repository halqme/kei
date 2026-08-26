# AGENTS.md

This file describes how to change `kei`. Product concepts and public contracts belong in `docs/`; do not turn this file back into a second README.

## Before editing

1. Read the files that own the behavior you are changing, including nearby tests.
2. Read the matching document under `docs/` when the change touches a public or architectural contract.
3. Decide whether the behavior belongs in the Go control plane at all. Prefer an extension process, descriptor, control, or existing Unix command when that keeps the core smaller.
4. For Go changes, follow `.agents/skills/use-modern-go/SKILL.md` before editing the relevant Go files.

Start from the narrowest package that can own the change. Avoid routing unrelated behavior through `internal/agent` just because every session eventually passes through it.

## Change map

Use the existing package boundary as the first routing decision:

```text
cmd/kei              CLI parsing, help, REPL wiring, command entry points
internal/acp          ACP JSON-RPC/frontend projection
internal/agent        session orchestration and model/tool loop
internal/auth         credential storage and provider authentication
internal/command      slash-command descriptors, parsing, execution
internal/config       config schema, lookup, creation, persistence
internal/control      generic external control chain
internal/extension    extension roots, discovery, shadowing, namespacing
internal/provider     provider interface and provider transports
internal/tool         tool descriptors, registry, argv/stdin execution
```

A new package needs a clear owner-independent responsibility. If the proposed package is primarily one concrete capability such as Git integration, code search, LSP, browser automation, formatting, or sandbox implementation, first try to make it an external process.

## Implementation workflow

Make changes in this order unless the task gives a better local sequence:

1. Locate the existing contract and its tests.
2. Add or adjust the smallest test that captures the intended behavior.
3. Implement the change in the package that owns that contract.
4. Run `gofmt` on modified Go files.
5. Run focused tests while iterating.
6. Run the repository verification commands before finishing.
7. Update `docs/` in the same change when user-visible behavior, descriptor fields, discovery, configuration, execution semantics, provider behavior, controls, or ACP projection changed.

Do not add speculative abstractions for future backends. Introduce an interface or protocol field when current independent implementations need the distinction.

## Verification

The repository tasks are defined in `.justfile`:

```sh
just test
just vet
just build
```

Equivalent commands are:

```sh
go test -count=1 ./...
go vet ./...
go build -o ./build/kei ./cmd/kei
```

Run focused package tests first when useful, but the full test suite is the completion gate for Go changes.

Documentation-only changes do not require inventing code changes merely to exercise the suite. Still check that command names, paths, JSON fields, and examples match the implementation.

## Tests that must travel with behavior

When changing extension discovery, cover the affected precedence and determinism boundaries: workspace/user/system/additional roots, whole-extension shadowing, hidden directories, and qualified names.

When changing tool or slash-command execution, cover the relevant combination of `PATH` lookup, extension-relative executable resolution, workspace cwd, stdin mode, placeholders/defaults, timeout/cancellation, stderr, and non-zero exit behavior.

When changing configuration or provider selection, cover ordering, explicit overrides, generated config location/permissions, existing-file preservation, explicit missing paths, authentication checks, and unsupported provider errors as applicable.

When changing provider transports, keep transport-specific serialization/parsing tests in `internal/provider`; do not make agent-loop tests prove HTTP details.

When changing ACP, keep protocol parsing/projection tests in `internal/acp` and session semantics in `internal/agent` where possible.

## Architectural invariants

Preserve these unless the task explicitly changes the contract and the corresponding docs/tests are updated:

- The OS process boundary is the extension ABI: executable, argv, stdin/stdout/stderr, exit status, signals, environment, and cwd.
- Extensions are namespace/distribution units, not in-process plugins.
- `tools.json` and `commands.json` are explicit declarations. Normal discovery does not infer agent contracts from `--help`, man pages, or shell completion metadata.
- Higher-precedence extension roots shadow a lower-precedence extension with the same ID as a whole; copies are not partially merged.
- Canonical tool identity is `<extension>.<tool>`. Provider-facing function names are transport representations.
- Canonical slash-command identity is `<extension>:<command>`, invoked by users as `/<extension>:<command>`.
- Unknown slash-prefixed text remains ordinary prompt input; only discovered commands are intercepted.
- A command without a path separator is resolved through `PATH`. A relative command containing a path separator is resolved from the extension root. The child process runs with the session workspace as cwd.
- Tools, slash commands, skills, and controls are separate concepts.
- Tool `effects` are policy/UX metadata, not a security boundary.
- ACP is a frontend adapter, not the internal data model.
- Credentials stay in the auth store or environment; generated configuration does not contain secrets.
- `providers` is ordered. Without an explicit target, the first configured target wins. Session startup may synthesize candidates when none are configured; read-only inspection must not mutate configuration.

The rationale and public description of these invariants live in `docs/architecture.md` and the relevant reference documents.

## Cross-cutting changes

A change that alters a seam should be treated as cross-cutting even if the diff begins in one package. In particular:

- Descriptor schema changes usually touch descriptor parsing, execution, examples, docs, and tests.
- Provider stream changes usually touch `internal/provider`, `internal/agent`, and frontend projection.
- New control decisions usually touch the control protocol, session behavior, approval behavior, and docs.
- Workspace semantics usually touch discovery, process cwd, ACP session creation, CLI wiring, and tests.
- Naming changes usually touch extension discovery, registries, provider function-name conversion, CLI inspection, ACP command advertisement, docs, and examples.

Trace these paths before coding instead of fixing downstream breakage one package at a time.

## Scope discipline

Keep `kei` a harness. A feature request described as a mode, integration, or UX feature is not automatically core runtime work.

Before adding core behavior, check whether the requirement can be satisfied by:

1. an existing command on `PATH`;
2. an extension-owned executable;
3. `tools.json` or `commands.json`;
4. a generic control process;
5. a skill or prompt;
6. the ACP client/frontend.

Only expand the core when the missing behavior is genuinely coordination or a reusable boundary that the harness must own.
