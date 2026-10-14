// Package resolve maps a skript target to the executor that should run it.
//
// Resolution order:
//
//  1. An alias from `config.toml [skripts]` always wins over a real path.
//  2. A directory target must itself contain `.skriptrc` (no walk-up) and
//     defines SKRIPT_ROOT.
//  3. A file target first walks up from its own directory to the nearest
//     enclosing `.skriptrc`; it then runs as that root, with the file as the
//     entrypoint (the rc's own entrypoint is ignored).
//  4. Without an enclosing `.skriptrc`, the file falls back to the config's
//     `[associations]`, matched against its filename. There is deliberately
//     no entrypoint field there; the matched file occupies that slot.
//  5. Otherwise: error.
package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/volkbyte/skript/internal/config"
	"github.com/volkbyte/skript/internal/rc"
)

// Target is the outcome of target resolution.
type Target struct {
	// Root is the SKRIPT_ROOT (the directory whose `.skriptrc` governs the
	// run). It is empty for association targets, which have no skript root.
	Root string

	// Executor runs the target. Association targets always use an exec
	// executor whose Executable comes from the matched pattern.
	Executor rc.Executor

	// File is the target file itself when the target is a file. It becomes
	// the entrypoint; empty for directory targets.
	File string
}

// Resolve maps a skript target to the executor that should run it.
func Resolve(cfg *config.Config, target string) (Target, error) {
	// The target spelling that reaches the child process: alias targets
	// arrive as their resolved path, file targets as typed by the user.
	spelling := target

	resolved := target
	if alias, found, err := cfg.AliasTarget(target); err != nil {
		return Target{}, err
	} else if found {
		spelling = alias
		resolved = alias
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return Target{}, fmt.Errorf("target %q does not exist", spelling)
		}
		return Target{}, fmt.Errorf("stat target %s: %w", spelling, err)
	}

	if info.IsDir() {
		return directory(cfg, resolved)
	}
	return file(cfg, spelling, resolved)
}

func directory(cfg *config.Config, dir string) (Target, error) {
	rcPath := filepath.Join(dir, ".skriptrc")
	if _, err := os.Stat(rcPath); err != nil {
		if os.IsNotExist(err) {
			return Target{}, fmt.Errorf("%s: not a skript root (no .skriptrc)", dir)
		}
		return Target{}, fmt.Errorf("stat %s: %w", rcPath, err)
	}

	exec, err := rc.Load(dir)
	if err != nil {
		return Target{}, err
	}
	return Target{Root: dir, Executor: exec}, nil
}

func file(cfg *config.Config, spelling, path string) (Target, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Target{}, fmt.Errorf("locate target %s: %w", spelling, err)
	}

	root, ok := enclosingRoot(abs)
	if !ok {
		return association(cfg, spelling, path)
	}

	exec, err := rc.Load(root)
	if err != nil {
		return Target{}, err
	}
	return Target{Root: root, Executor: exec, File: spelling}, nil
}

func association(cfg *config.Config, spelling, path string) (Target, error) {
	base := filepath.Base(path)
	executable, found, err := MatchAssociations(cfg.Associations, base)
	if err != nil {
		return Target{}, err
	}
	if !found {
		return Target{}, fmt.Errorf("no executor for skript %q: no enclosing .skriptrc and no matching association", spelling)
	}
	return Target{
		Executor: rc.Executor{Type: rc.TypeExec, Executable: executable},
		File:     spelling,
	}, nil
}

// enclosingRoot walks up from child to the nearest ancestor (or child
// itself) containing a `.skriptrc`.
func enclosingRoot(child string) (string, bool) {
	for dir := child; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".skriptrc")); err == nil {
			return dir, true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", false
		}
	}
}

// MatchAssociations matches a filename against configured glob patterns,
// most specific pattern first.
//
// Specificity counts literal characters ('*' does not count), so
// `*.apple.lua` beats `*.lua`. Ties are broken by the lexicographically
// smaller pattern.
func MatchAssociations(associations map[string]string, filename string) (string, bool, error) {
	type candidate struct {
		pattern    string
		executable string
		score      int
	}

	var matches []candidate
	for pattern, executable := range associations {
		ok, err := filepath.Match(pattern, filename)
		if err != nil {
			return "", false, fmt.Errorf("associations: matched pattern %q against %q: %w", pattern, filename, err)
		}
		if ok {
			matches = append(matches, candidate{pattern, executable, specificity(pattern)})
		}
	}
	if len(matches) == 0 {
		return "", false, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].pattern < matches[j].pattern
	})
	return matches[0].executable, true, nil
}

func specificity(pattern string) int {
	score := 0
	for _, r := range pattern {
		if r != '*' {
			score++
		}
	}
	return score
}
