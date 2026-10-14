#!/bin/sh
# Smoke run for skript on Linux (run in WSL2 Ubuntu for Linux coverage).
#
# Demonstrates one exec-skript and one command-skript end to end against a
# throwaway `~/.volkbyte/skript/config.toml` layout, using a fake HOME.
#
# Usage: bash scripts/smoke-linux.sh [path-to-skript-binary]
# Builds the binary with `go build` when no path is given.

set -u

cd "$(dirname "$0")/.." || exit 1
repo_pwd=$(pwd)

if [ "${1:-}" != "" ]; then
    skript_bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
else
    echo "== building skript with go"
    mkdir -p .smoke
    go build -o .smoke/skript ./cmd/skript || exit 1
    skript_bin="$repo_pwd/.smoke/skript"
fi

home=$(mktemp -d)
trap 'rm -rf "$home" "$repo_pwd/.smoke"' EXIT
export HOME="$home"

mkdir -p "$home/.volkbyte/skript"

root1="$home/roots/exec-root"
root2="$home/roots/command-root"
mkdir -p "$root1" "$root2"

cat > "$home/.volkbyte/skript/config.toml" <<EOF
[skripts]
demo = "$root1"
EOF

# Quoted heredoc: the .skriptrc contents go out verbatim.
cat > "$root1/.skriptrc" <<'EOF'
[meta]
version = "1"

[executor]
type = "exec"
executable = "/bin/sh"
args = ["-c", "printf 'exec-skript: entrypoint=%s vargs=%s|%s|%s\n' \"$0\" \"$@\""]
entrypoint = "entry.dummy"
EOF

printf 'entrypoint file for the exec skript\n' > "$root1/entry.dummy"

cat > "$root2/.skriptrc" <<'EOF'
[meta]
version = "1"

[executor]
type = "command"
command = "printf 'command-skript: vargs=%s %s %s\\n'"
EOF

fail=0
expect_line() {
    echo "   output: $2"
    if [ "$2" != "$3" ]; then
        echo "   !! ($1) FAIL: wanted: $3" >&2
        fail=1
    fi
}

echo "=== 1. exec executor: directory target (entrypoint resolved to SKRIPT_ROOT)"
out=$("$skript_bin" "$root1" A B)
expect_line exec "$out" "exec-skript: entrypoint=$root1/entry.dummy vargs=A|B|"

echo "=== 2. exec executor: file target inside the root (walk-up)"
out=$("$skript_bin" "$root1/entry.dummy" A B)
expect_line exec "$out" "exec-skript: entrypoint=$root1/entry.dummy vargs=A|B|"

echo "=== 3. alias from ~/.volkbyte/skript/config.toml"
out=$("$skript_bin" demo A B)
expect_line alias "$out" "exec-skript: entrypoint=$root1/entry.dummy vargs=A|B|"

echo "=== 4. command executor: string form through sh -c"
out=$("$skript_bin" "$root2" A B C)
expect_line command "$out" "command-skript: vargs=A B C"

echo "=== 5. metacharacters stay literal through the shell"
out=$("$skript_bin" "$root2" 'A&B' 'in $(dangerous)' 'x|y')
expect_line command "$out" 'command-skript: vargs=A&B in $(dangerous) x|y'

echo "=== 6. errors are one skript: line with exit 1"
out=$("$skript_bin" "$home/ghost.lua" 2>&1)
code=$?
echo "   output: $out (exit code $code)"
if [ "$out" != "skript: target \"$home/ghost.lua\" does not exist" ] || [ "$code" != "1" ]; then
    echo "   !! FAIL" >&2
    fail=1
fi

if [ "$fail" = "0" ]; then
    echo
    echo "SMOKE OK"
else
    echo >&2
    echo "SMOKE FAILED" >&2
    exit 1
fi
