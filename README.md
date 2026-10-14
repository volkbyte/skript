<div align="center">

# skript

*a small, cross-platform, configurable script runner*

[![Go version](https://img.shields.io/badge/Go-1.26.5-00ADD8?style=flat-square)](go.mod)
[![Platform](https://img.shields.io/badge/platform-Windows_·_Linux-4dabf7?style=flat-square)](#installation)

</div>

**skript** runs your scripts by *target* — an alias, a skript root directory, or a script file — and delegates the actual execution to the executor declared by the matching `.skriptrc` or to a filename association from your config. One binary across Windows and Linux, with no shell harness of its own: it composes the child process, wires its own stdio to it, and hands over the exit code verbatim.

## How it works

Every invocation is `skript <target> [args...]`. The target is resolved in this order:

| Target | Resolution |
|---|---|
| **Alias** ([`[skripts]` in config.toml](#configuration)) | Always wins, even over a real path with the same spelling |
| **Directory** | Must itself contain a `.skriptrc` (no walk-up); it defines `${SKRIPT_ROOT}` |
| **File** | Walks up to the nearest enclosing `.skriptrc`; runs as that root with the file as entrypoint |
| **File** (no enclosing rc) | Falls back to an `[associations]` glob match against the filename |
| Anything else | Error |

The resolution outcome is an *executor*, and the child runs as

```
[executable, ...args, entrypoint, ...vargs]
```

with skript's own stdio, the caller's working directory, and the resolved executor's environment applied on top of the inherited one.

> [!TIP]
> Browse [`examples/.skriptrc`](examples/.skriptrc) and [`examples/config.toml`](examples/config.toml) for fully commented definitions of each mechanism.

## Features

- **Three executor kinds** — direct exec, one-line commands through the platform shell, or shell-free command arrays (the safe choice against quoting pitfalls).
- **`${}`-interpolation** in `.skriptrc` string values, single-pass, with `${SKRIPT_ROOT}`, namespaced `${env:…}` lookups and a `$$` escape for a literal `$`.
- **Deterministic environment merging** — the child's environment is built in sorted order; entries are replaced in place, never duplicated, and fold case on Windows.
- **A strict failure contract** — every skript-internal failure prints exactly one `skript: <reason>` line to stderr and exits with status 1. A resolved child's exit code is returned verbatim.
- **Per-platform shell handling done right** — on Windows the CRT-quoting rules of `cmd /C call` are honored, so vargs containing quotes and cmd metacharacters arrive at the child as inert, single arguments.

## Installation

```bash
go install github.com/volkbyte/skript/cmd/skript@latest
```

…or build locally with [Task](https://taskfile.dev/installation), which builds both platforms into `dist/`:

```bash
task build            # → dist/skript.exe + dist/skript-linux-amd64
```

A plain `go build ./cmd/skript` works too (see the [`Taskfile`](Taskfile.yml) for the exact flags).

## Quick start

Create a skript root with a `.skriptrc` that names the interpreter:

```toml
[meta]
version = "1"

[executor]
type       = "exec"
executable = "lua"
entrypoint = "main.lua"

[executor.environment]
LUA_PATH = [
    "${SKRIPT_ROOT}/lib/?.lua",
    "${SKRIPT_ROOT}/vendor/?.lua",
]
```

…then run it as a directory, or as a file (which forgoes `entrypoint` and fills the slot itself):

```
skript ./tools/rules            # → calls lua, -e rules…, <root>/main.lua, …
skript ./tools/rules/main.lua   # → the file itself becomes the entrypoint
```

## Executors

An `.skriptrc` lives in the root directory it governs. `meta.version` must be exactly `"1"`, and `executor.type` is either `exec` or `command` — fields belonging to the other kind are a load error, never silently ignored.

| Executor | Shape | Invocation |
|---|---|---|
| `exec` | `executable` + optional `args` + optional `entrypoint` | `[executable, ...args, entrypoint, ...vargs]`, no shell |
| `command` (string) | `command = "…"` | Through the platform shell: `sh -c '<command> "$@"' …` on Linux, `cmd /C call <command> …` on Windows |
| `command` (array) | `command = ["prog", "…"]` | `[program, ...args, ...vargs]`, no shell — the safe choice |

The `exec` executor's `entrypoint` resolves **only** against `${SKRIPT_ROOT}`, must there exist as a file, and must be relative. A file target bypasses the rc's entrypoint entirely: the target file occupies that slot, so the same root serves every entrypoint you hand to it.

### Environment

`executor.environment` sets or overrides child variables. Values are strings or arrays; array values join with the platform's path-list separator (`:` on Linux, `;` on Windows). Every string value is `${}`-interpolated before use.

### Interpolation

```
${SKRIPT_ROOT}   the root directory whose .skriptrc governs the run
${env:HOME}      a namespaced lookup of an environment variable
$$               escapes a literal $, so $${HOME} stays ${HOME}
```

Unknown variables are an error, and interpolation runs exactly once: expanded values are never rescanned, so a value containing `${...}` is inserted as-is. Variable *keys* and the user's vargs are never touched, and no interpolation happens inside `config.toml`.

## Configuration

`~/.volkbyte/skript/config.toml` is skript's user configuration; a missing file is a valid empty config, an unparseable one aborts with one error line.

- **`[skripts]`** — alias → target map. Names must match `^[A-Za-z][A-Za-z0-9/_-]*$`. An absolute target is kept, `~`-prefixed paths expand against your home, and an ordinary relative path resolves against the directory containing the config file.
- **`[associations]`** — glob patterns over script *filenames* → executor. The most specific pattern wins (more literal characters beat fewer, so `*.apple.lua` beats `*.lua`). Ties are broken lexicographically. A value is either a bare executable path or an exec executor table.

### Example

```toml
[skripts]
demo       = "~/projects/skript-demo"
hello      = "~/projects/helloer/main.lua"

[associations]
"*.lua"       = "lua"
"*.apple.lua" = "/opt/luaJIT/lua"
"*.py"        = "python3"

[associations."*.strict.lua"]
type       = "exec"
executable = "lua"
args       = ["-Xstrict"]
```

> [!NOTE]
> There is deliberately no `entrypoint` field in `[associations]`: the matched file occupies that slot, so the association decides *how* a script runs, never *which* script.

> [!WARNING]
> On Windows, a command-string executor places your vargs on a `cmd /C` line, and cmd expands `%VAR%` in them before the child sees it. Prefer the command array form or an `exec` executor when vargs may contain percent signs.

## Development

```bash
task vet            # static checks
task test           # full suite (unit + e2e; spawns real child processes)
task smoke:linux    # linux-side smoke run (needs bash; WSL on Windows)
task smoke:windows  # windows-side smoke run
task clean          # remove dist/ and Task's cache dir
```

The resolver, executor composition, interpolation and config loader each carry their own packages under [`internal/`](internal/), with end-to-end scenarios covering alias targets, root resolution, associations and executor wiring.
