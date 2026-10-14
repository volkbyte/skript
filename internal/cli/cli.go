// Package cli wires target resolution and execution into the skript command
// line: `skript <target> [args...]`.
//
// Failure contract: every skript-internal failure prints exactly one
// `skript: <reason>` line to stderr and exits 1. A resolved child runs with
// skript's stdio and the caller's working directory, and its exit code is
// returned verbatim.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/volkbyte/skript/internal/config"
	"github.com/volkbyte/skript/internal/resolve"
	"github.com/volkbyte/skript/internal/run"
)

// App carries the resolved runtime location of skript.
type App struct {
	// Stderr receives the single-line `skript: <reason>` errors; it is
	// injectable for tests. Nil means os.Stderr.
	Stderr io.Writer

	// ConfigPath and HomeDir locate `~/.volkbyte/skript/config.toml` and
	// drive `~` expansion in alias targets. Both are injectable for tests.
	ConfigPath string
	HomeDir    string
}

// Run executes the command line (without the program name) and returns the
// process exit code.
func (a *App) Run(args []string) int {
	if len(args) < 1 {
		return a.fail(errors.New("usage: skript <target> [args...]"))
	}
	target, vargs := args[0], args[1:]

	cfg, err := config.Load(a.ConfigPath, a.HomeDir)
	if err != nil {
		return a.fail(err)
	}

	resolved, err := resolve.Resolve(cfg, target)
	if err != nil {
		return a.fail(err)
	}

	cmd, err := run.Compose(resolved, vargs)
	if err != nil {
		return a.fail(err)
	}

	err = cmd.Run()
	var childExit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &childExit):
		return childExit.ExitCode()
	default:
		return a.fail(fmt.Errorf("run child: %w", err))
	}
}

func (a *App) fail(err error) int {
	stderr := a.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	fmt.Fprintf(stderr, "skript: %s\n", oneLine(err))
	return 1
}

// oneLine flattens an error reason onto a single line, keeping the contract
// of exactly one `skript: <reason>` line even when underlying details (for
// example TOML parser diagnostics) span multiple lines.
func oneLine(err error) string {
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\r\n", " ")
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	return strings.TrimSpace(msg)
}
