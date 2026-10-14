// Package run composes the child process invocation for a resolved skript
// target and runs it.
//
// Executor semantics:
//
//   - exec executor: [executable, ...args, entrypoint, ...vargs] with no
//     shell. The entrypoint resolves only against SKRIPT_ROOT and must
//     exist; the matched skript file occupies that slot for association
//     targets (which have no entrypoint field).
//   - command executor, string form: run through the platform shell; see
//     the per-platform files for the exact command lines.
//   - command executor, array form: [program, ...args, ...vargs] with no
//     shell; the safe choice against quoting pitfalls.
//
// The child inherits skript's stdio and the caller's working directory;
// `environment` entries are applied on top of the inherited environment.
package run

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/volkbyte/skript/internal/rc"
	"github.com/volkbyte/skript/internal/resolve"
)

// Compose builds the child command for a resolved target. Stdio is wired to
// skript's own streams and the caller must Wait for the command.
func Compose(target resolve.Target, vargs []string) (*exec.Cmd, error) {
	var (
		cmd *exec.Cmd
		err error
	)
	switch target.Executor.Type {
	case rc.TypeExec:
		cmd, err = composeExec(target, vargs)
	case rc.TypeCommand:
		cmd, err = composeCommand(target, vargs)
	default:
		err = fmt.Errorf("unknown executor type %d", target.Executor.Type)
	}
	if err != nil {
		return nil, err
	}
	applyEnv(cmd, target.Executor.Env)
	return cmd, nil
}

func composeExec(target resolve.Target, vargs []string) (*exec.Cmd, error) {
	if err := validateEntrypoint(target); err != nil {
		return nil, err
	}

	// The entrypoint handed to the child: a file target keeps the user's
	// path spelling; a rc entrypoint resolves against SKRIPT_ROOT because
	// the child inherits the caller's working directory, not the root.
	entrypoint := entrypointArgument(target)
	if target.File == "" {
		entrypoint = filepath.Join(target.Root, entrypoint)
	}

	argv := slices.Concat(
		target.Executor.Args,
		[]string{entrypoint},
		vargs,
	)
	return newExec(target.Executor.Executable, argv)
}

func composeCommand(target resolve.Target, vargs []string) (*exec.Cmd, error) {
	if target.Executor.CommandStr != "" {
		return shellCommand(target.Executor.CommandStr, vargs)
	}
	program, rest := target.Executor.CommandList[0], target.Executor.CommandList[1:]
	return newExec(program, slices.Concat(rest, vargs))
}

// validateEntrypoint checks the entrypoint slot of an exec executor. Only
// the rc's own entrypoint field is rooted at SKRIPT_ROOT and must exist; a
// file target occupies the slot itself and its existence was checked at
// resolve time.
func validateEntrypoint(target resolve.Target) error {
	if target.File != "" {
		return nil
	}

	entrypoint := target.Executor.Entrypoint
	if entrypoint == "" {
		return fmt.Errorf("executor.entrypoint is required")
	}
	if filepath.IsAbs(entrypoint) {
		return fmt.Errorf("executor.entrypoint %q must be relative and resolve against SKRIPT_ROOT", entrypoint)
	}

	full := filepath.Join(target.Root, entrypoint)
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("executor.entrypoint %q does not exist in SKRIPT_ROOT %s", entrypoint, target.Root)
		}
		return fmt.Errorf("stat executor.entrypoint %s: %w", full, err)
	}
	if info.IsDir() {
		return fmt.Errorf("executor.entrypoint %q is a directory", full)
	}
	return nil
}

// entrypointArgument is the raw entrypoint slot value: the file target when
// there is one, else the rc's entrypoint field, which composeExec resolves
// against SKRIPT_ROOT.
func entrypointArgument(target resolve.Target) string {
	if target.File != "" {
		return target.File
	}
	return target.Executor.Entrypoint
}

// newExec validates the executable through PATH resolution.
func newExec(executable string, args []string) (*exec.Cmd, error) {
	cmd := exec.Command(executable, args...)
	if cmd.Err != nil {
		return nil, fmt.Errorf("resolve executable %q: %w", executable, cmd.Err)
	}
	return cmd, nil
}

// applyEnv wires the child to skript's stdio and applies environment
// additions on top of the inherited environment. The child keeps the
// caller's working directory.
func applyEnv(cmd *exec.Cmd, additions []string) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(additions) > 0 {
		cmd.Env = mergedEnvironment(additions)
	}
}

// mergedEnvironment replaces inherited entries whose key matches an
// addition in place and appends the rest; a later addition wins over an
// earlier one, and environment additions never create duplicates. Keys fold
// case on Windows, where environment variable names are case-insensitive.
func mergedEnvironment(additions []string) []string {
	env := os.Environ()
	values := make(map[string]string, len(additions))
	for _, kv := range additions {
		values[foldEnvKey(kv)] = kv
	}

	for i, kv := range env {
		key := foldEnvKey(kv)
		value, found := values[key]
		if !found {
			continue
		}
		env[i] = value
		delete(values, key)
	}

	for _, kv := range additions {
		key := foldEnvKey(kv)
		if values[key] == kv {
			env = append(env, kv)
			delete(values, key)
		}
	}
	return env
}

func foldEnvKey(kv string) string {
	key, _, _ := strings.Cut(kv, "=")
	if runtime.GOOS == "windows" {
		return strings.ToLower(key)
	}
	return key
}
