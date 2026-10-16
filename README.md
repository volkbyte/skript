<div align="center">

# skript | скрипт

*a small, cross-platform, configurable script runner*

[![Go version](https://img.shields.io/badge/Go-1.26.5-00ADD8?style=flat-square)](go.mod)
[![Platform](https://img.shields.io/badge/platform-Windows_·_Linux-4dabf7?style=flat-square)](#installation)

</div>

**skript** runs your scripts by *target*: an alias, a directory, or a file. It resolves the target to an *executor* — declared by the matching `.skriptrc` or by a filename association in your config — then composes the child process itself, wires its own stdio to it, and returns the child's exit code verbatim. One binary for Windows and Linux alike.

## How it works

Every invocation is `skript <target> [args...]`. skript resolves the target in this order:

| Target | Resolution |
|---|---|
| **Alias** ([`[skripts]` in config.toml](#configuration)) | Always wins, even over a real path with the same spelling |
| **Directory** | Must itself contain a `.skriptrc` (no walk-up); it defines `${SKRIPT_ROOT}` |
| **File** | Walks up to the nearest enclosing `.skriptrc`; runs as that root, with the file as entrypoint |
| **File** (no enclosing rc) | Falls back to an `[associations]` glob match against the filename |
| Anything else | Error |

Resolution always ends in an executor, and the child runs as

```
[executable, ...args, entrypoint, ...vargs]
```

on skript's own stdio, in the caller's working directory, under the executor's environment applied on top of the inherited one.

> [!TIP]
> Every mechanism below has a fully commented example in [`examples/.skriptrc`](examples/.skriptrc) and [`examples/config.toml`](examples/config.toml).

## Features

- **Three executor kinds** — direct exec, a command line through the platform shell, or a shell-free command array.
- **Single-pass `${}` interpolation** in `.skriptrc` values — `${SKRIPT_ROOT}`, namespaced `${env:NAME}` lookups, and a `$$` escape for a literal `$`.
- **Deterministic environment merging** — skript builds the child's environment in sorted order, replaces matching entries in place, and folds key case on Windows.
- **A strict failure contract** — skript-internal failures print exactly one `skript: <reason>` line to stderr and exit 1; a child's exit code passes through verbatim.
- **Careful Windows shell handling** — command strings run under `cmd /C call`, and vargs follow the CRT quoting rules, so quotes and cmd metacharacters reach the child inert.

## Installation

```bash
go install github.com/volkbyte/skript/cmd/skript@latest
```

Or build both platforms into `dist/` with [Task](https://taskfile.dev/installation):

```bash
task build            # → dist/skript.exe + dist/skript-linux-amd64
```

A plain `go build ./cmd/skript` also works; the [`Taskfile`](Taskfile.yml) shows the exact flags.

## Quick start

1. Write a user config at `~/.volkbyte/skript/config.toml` — usually just filename associations:

   ```toml
   [associations]
   "*.lua" = "lua"
   "*.py"  = "python3"
   ```

2. Run a target — a file path, a directory path, or an alias from `[skripts]`:

   ```text
   skript main.lua           # → [lua, main.lua]
   skript ./scripts/run.py   # → [python3, ./scripts/run.py]
   ```

That is all a standalone file needs. The two ways to make a script *skriptable* follow.

## Creating a Skript

### File Skript

A single file, matched by an `[associations]` glob against its filename — no root, no `.skriptrc`:

```toml
# ~/.volkbyte/skript/config.toml
[associations]
"*.lua" = "lua"
```

```text
skript main.lua -v   # → [lua, main.lua, -v] — vargs go straight to the child
```

Associations deliberately have no `entrypoint` field: the matched file occupies that slot itself. A bare executor name resolves through `PATH`, `~`-prefixed paths expand to your home directory, and no `${}` interpolation runs in `config.toml`.

> [!NOTE]
> A file under a directory with a `.skriptrc` runs under that root: the walk-up to the nearest rc precedes any association lookup.

### Directory Skript

A directory carrying a `.skriptrc`; that directory becomes `${SKRIPT_ROOT}`:

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

```text
skript ./tools/rules            # runs the root's own entrypoint
skript ./tools/rules/main.lua   # a file target replaces the entrypoint slot
```

For the full executor forms — exec, command string, command array — see [Executors](#executors).

## Executors

An `.skriptrc` lives in the root it governs. It requires `meta.version = "1"` and an `executor.type` of `exec` or `command`; a field belonging to the other kind is a load error, never silently ignored.

| Executor | Shape | Invocation |
|---|---|---|
| `exec` | `executable` + optional `args` + optional `entrypoint` | `[executable, ...args, entrypoint, ...vargs]`, no shell |
| `command` (string) | `command = "…"` | Through the platform shell: `sh -c '<command> "$@"' …` on Linux, `cmd /C call <command> …` on Windows |
| `command` (array) | `command = ["prog", "…"]` | `[program, ...args, ...vargs]`, no shell — the safe choice |

The two `command` forms, as written in an `.skriptrc`:

```toml
[executor]
type    = "command"
command = "make -f ${SKRIPT_ROOT}/rules.mk"    # string form: platform shell

# Array form — no shell at all, the safe choice against quoting pitfalls:
# [executor]
# type    = "command"
# command = ["make", "-f", "${SKRIPT_ROOT}/rules.mk"]
```

Both forms pass the caller's vargs through — the string form appends them to the shell command line, the array form after its last listed argument.

An `exec` executor's `entrypoint` must be relative and must exist; skript resolves it only against `${SKRIPT_ROOT}`. A file target bypasses the rc's entrypoint: the file occupies the slot itself, so one root serves every entrypoint you hand it.

### Environment

`executor.environment` sets or overrides child variables. Values are strings or arrays; array values join with the platform's path-list separator (`:` on Linux, `;` on Windows). skript interpolates every string value before use.

### Interpolation

```
${SKRIPT_ROOT}   the root directory whose .skriptrc governs the run
${env:HOME}      a namespaced lookup of an environment variable
$$               escapes a literal $, so $${HOME} stays ${HOME}
```

An unknown variable is an error. Interpolation runs exactly once — expanded values are never rescanned — so a value containing `${...}` lands as-is. Variable keys and the user's vargs pass through untouched.

## Configuration

skript reads its user configuration from `~/.volkbyte/skript/config.toml`. A missing file acts as an empty config; an unparseable file aborts with one error line and exit 1.

```toml
# ~/.volkbyte/skript/config.toml
#
# Values are used as-is: no ${} interpolation runs in this file.
# `~`-prefixed paths expand to your home directory — in [skripts]
# targets and [associations] executors alike.

[skripts]
# alias = target — a short name for a target you'd otherwise type out.
#
# alias   must match ^[A-Za-z][A-Za-z0-9/_-]*$; an alias always wins over
#         a real path with the same spelling
# target  a file, or a directory skript root (a directory target must carry
#         its own .skriptrc). Accepted spellings:
#           ~-prefixed  "~/projects/demo"    expands to your home directory
#           absolute    "/opt/tools/demo"    kept as-is
#           relative    "skripts/demo.lua"   resolved against the directory
#                                            containing this config file —
#                                            not the current working dir
demo       = "~/projects/skript-demo"        # a directory skript
hello      = "~/projects/helloer/main.lua"   # a file skript
docs-build = "skripts/docs-build.lua"        # relative to the config dir

[associations]
# "glob pattern" = executor — used for file targets with no enclosing
# .skriptrc. The pattern matches the file's base name only, never its path.
#
# pattern   a glob over the filename; the most specific pattern wins
#           (more literal characters beat fewer, so "*.apple.lua" beats
#           "*.lua"); ties break lexicographically
# executor  a bare name, an absolute path, a `~`-prefixed path, or a
#           relative one — see the value-shape table below
"*.lua"       = "lua"                # bare name, resolved via PATH
"*.mine.lua"  = "~/.bin/lua55.exe"   # `~`-prefixed: expands to your home
"*.apple.lua" = "/opt/luaJIT/lua"    # absolute path — more specific, wins
"*.py"        = "python3"

# Executor-table form: exec only (associations have no command form), with
# the same slots as a .skriptrc [executor], except there is deliberately NO
# entrypoint — the matched file fills that slot.
[associations."*.strict.lua"]
type       = "exec"
executable = "lua"
args       = ["-Xstrict"]
```

In both forms, an association's executor value behaves by shape:

| Value shape | Example | Behavior |
|---|---|---|
| `~`-prefixed | `"~/.bin/lua55.exe"` | Expands to an absolute path under your home directory |
| Bare name | `"lua55"` | Resolved via `PATH` at exec time |
| Absolute | `"/opt/luaJIT/lua"` | Used as-is |
| Relative | `"bin/lua"` | Resolved against the caller's working directory at exec time |

> [!NOTE]
> Associations accept **exec executors only**. Their job is to hand the matched file to a program: the file occupies the entrypoint slot, so an association decides *how* a script runs, never *which* script. Command executors live in `.skriptrc`; see [Executors](#executors).

> [!WARNING]
> On Windows, a command-string executor places vargs on a `cmd /C` line, where cmd expands `%VAR%` before the child sees them. Use the array form or an `exec` executor when vargs may contain percent signs.

## Development

```bash
task vet            # static checks
task test           # full suite (unit + e2e; spawns real child processes)
task smoke:linux    # linux-side smoke run (needs bash; WSL on Windows)
task smoke:windows  # windows-side smoke run
task clean          # remove dist/ and Task's cache dir
```

Each concern lives in its own package under [`internal/`](internal/) — resolution, executor composition, interpolation, and the config loader — and end-to-end scenarios cover alias targets, root resolution, associations, and executor wiring.
