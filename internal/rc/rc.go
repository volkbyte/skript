// Package rc parses `.skriptrc` files, the per-root executor definition.
//
// A `.skriptrc` lives in the skript root directory it governs; that
// directory is the ${SKRIPT_ROOT} interpolation value. The rules:
//
//   - meta.version must be "1".
//   - The executor is discriminated on its type ("exec" or "command").
//     Fields belonging to the other kind are a load error, not ignored.
//   - String values of the executor get single-pass ${} interpolation (see
//     the interp package). Nothing else is interpolated: keys are not, and
//     the user's vargs are never touched.
//   - environment sets/overrides variables for the executed process; array
//     values are joined with the platform's path-list separator.
package rc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/volkbyte/skript/internal/interp"
)

// ExecutorType enumerates the executor kinds.
type ExecutorType int

const (
	TypeUnknown ExecutorType = iota
	TypeExec
	TypeCommand
)

// Executor is a parsed, interpolated and validated skript executor.
type Executor struct {
	// Type discriminates the remaining fields.
	Type ExecutorType

	// Exec-executor fields. The child runs as
	// [executable, ...args, entrypoint, ...vargs].
	//
	// Entrypoint is empty when a file target fills the slot itself,
	// instead of this field.
	Executable string
	Args       []string
	Entrypoint string

	// Command-executor fields. Exactly one is set:
	// CommandStr runs the command text through the platform shell;
	// CommandList runs [program, ...args, ...vargs] with no shell. The
	// other stays empty.
	CommandStr  string
	CommandList []string

	// Env additions in KEY=VALUE form, applied on top of the environment
	// skript itself inherited.
	Env []string
}

// Load parses and interpolates the `.skriptrc` file of root, the skript root
// directory (the value of ${SKRIPT_ROOT} for interpolations).
func Load(root string) (Executor, error) {
	rc := Executor{}
	path := filepath.Join(root, ".skriptrc")

	// Built-ins for the interpolation of executor string values.
	builtins := func(name string) (string, bool) {
		if name == "SKRIPT_ROOT" {
			return root, true
		}
		return "", false
	}
	namespaced := func(ns, name string) (string, bool) {
		if ns != "env" {
			return "", false
		}
		return os.LookupEnv(name)
	}

	var raw struct {
		Meta struct {
			Version string `toml:"version"`
		} `toml:"meta"`
		Executor struct {
			Type        string         `toml:"type"`
			Executable  string         `toml:"executable"`
			Args        []string       `toml:"args"`
			Entrypoint  string         `toml:"entrypoint"`
			Command     any            `toml:"command"`
			Environment map[string]any `toml:"environment"`
		} `toml:"executor"`
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return rc, fmt.Errorf("read %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return rc, fmt.Errorf("parse %s: %w", path, err)
	}

	if raw.Meta.Version != "1" {
		return rc, fmt.Errorf("%s: meta.version must be \"1\"", path)
	}

	switch raw.Executor.Type {
	case "exec":
		rc.Type = TypeExec
	case "command":
		rc.Type = TypeCommand
	default:
		return rc, fmt.Errorf("%s: executor.type must be \"exec\" or \"command\", got %q",
			path, raw.Executor.Type)
	}

	// Fields that do not belong to this executor kind are config mistakes:
	// reject each one instead of silently ignoring it.
	if rc.Type == TypeCommand {
		switch {
		case raw.Executor.Entrypoint != "":
			return rc, fmt.Errorf("%s: executor.entrypoint is only valid for \"exec\" executors", path)
		case raw.Executor.Executable != "":
			return rc, fmt.Errorf("%s: executor.executable is only valid for \"exec\" executors", path)
		case raw.Executor.Args != nil:
			return rc, fmt.Errorf("%s: executor.args is only valid for \"exec\" executors", path)
		}
	}
	if rc.Type == TypeExec && raw.Executor.Command != nil {
		return rc, fmt.Errorf("%s: executor.command is only valid for \"command\" executors", path)
	}

	if raw.Executor.Command == nil && rc.Type == TypeCommand {
		return rc, fmt.Errorf("%s: executor.command is required", path)
	}

	if rc.Type == TypeExec {
		if raw.Executor.Executable == "" {
			return rc, fmt.Errorf("%s: executor.executable is required", path)
		}
		rc.Executable, err = interp.Expand(raw.Executor.Executable, builtins, namespaced)
		if err != nil {
			return rc, err
		}
		for _, a := range raw.Executor.Args {
			expanded, argErr := interp.Expand(a, builtins, namespaced)
			if argErr != nil {
				return rc, argErr
			}
			rc.Args = append(rc.Args, expanded)
		}
		rc.Entrypoint, err = interp.Expand(raw.Executor.Entrypoint, builtins, namespaced)
		if err != nil {
			return rc, err
		}
	}

	if rc.Type == TypeCommand {
		switch c := raw.Executor.Command.(type) {
		case string:
			rc.CommandStr, err = interp.Expand(c, builtins, namespaced)
			if err != nil {
				return rc, err
			}
			if rc.CommandStr == "" {
				return rc, fmt.Errorf("%s: executor.command is empty", path)
			}
		case []any:
			for _, item := range c {
				s, ok := item.(string)
				if !ok {
					return rc, fmt.Errorf("%s: executor.command must be a string or an array of strings", path)
				}
				expanded, itemErr := interp.Expand(s, builtins, namespaced)
				if itemErr != nil {
					return rc, itemErr
				}
				rc.CommandList = append(rc.CommandList, expanded)
			}
			if len(rc.CommandList) == 0 {
				return rc, fmt.Errorf("%s: executor.command is empty", path)
			}
		default:
			return rc, fmt.Errorf("%s: executor.command must be a string or an array of strings", path)
		}
	}

	if raw.Executor.Environment != nil {
		rc.Env, err = environment(raw.Executor.Environment, builtins, namespaced)
		if err != nil {
			return rc, err
		}
	}

	return rc, nil
}

// environment validates keys, interpolates string values and joins array
// values with the platform's path-list separator. Keys are sorted so the
// child's environment is deterministic.
func environment(table map[string]any, builtins interp.Builtins, namespaced interp.Namespaced) ([]string, error) {
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(table))
	for _, k := range keys {
		if k == "" || strings.ContainsAny(k, "=\n") {
			return nil, fmt.Errorf("executor.environment: invalid key %q", k)
		}

		var (
			value string
			err   error
		)
		switch v := table[k].(type) {
		case string:
			value, err = interp.Expand(v, builtins, namespaced)
		case []any:
			parts := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("executor.environment: %q must be an array of strings", k)
				}
				expanded, itemErr := interp.Expand(s, builtins, namespaced)
				if itemErr != nil {
					return nil, itemErr
				}
				parts = append(parts, expanded)
			}
			value = strings.Join(parts, string(os.PathListSeparator))
		default:
			return nil, fmt.Errorf("executor.environment: %q must be a string or an array of strings", k)
		}
		if err != nil {
			return nil, err
		}

		env = append(env, k+"="+value)
	}
	return env, nil
}
