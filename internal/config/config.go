// Package config loads `~/.volkbyte/skript/config.toml`.
//
// Semantics:
//
//   - A missing config file is an empty (but valid) configuration.
//   - An unparseable config file is an error (one `skript: <reason>` line).
//   - Alias names must match ^[A-Za-z][A-Za-z0-9/_-]*$.
//   - Association patterns are glob patterns over skript filenames; they are
//     validated at load time.
//   - No ${} interpolation happens inside config.toml; values are used as-is
//     (after alias-specific path handling, see AliasTarget).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// aliasNamePattern restricts skript alias names.
var aliasNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9/_-]*$`)

// Config is the parsed user configuration.
type Config struct {
	// Dir is the directory containing config.toml; relative alias targets
	// are resolved against it.
	Dir string

	// Associations maps glob patterns over skript filenames to an executable
	// that runs matching files.
	Associations map[string]string

	// Skripts maps alias names to target paths.
	Skripts map[string]string

	home string
}

// Load parses the config file at path. home is the user's home directory,
// used to expand `~` in alias targets. A missing file yields an empty config.
func Load(path, home string) (*Config, error) {
	cfg := &Config{Dir: filepath.Dir(path), home: home}

	var raw struct {
		Associations map[string]any    `toml:"associations"`
		Skripts      map[string]string `toml:"skripts"`
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	associations, err := normaliseAssociations(raw.Associations)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	cfg.Associations = associations
	cfg.Skripts = raw.Skripts

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// normaliseAssociations accepts an association value as a bare executable
// string or as an exec executor table ({type = "exec", executable = ..., args
// = [...]}) without an entrypoint field — there is deliberately no
// entrypoint in associations, because the matched file occupies that slot.
func normaliseAssociations(raw map[string]any) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}

	result := make(map[string]string, len(raw))
	for pattern, value := range raw {
		switch v := value.(type) {
		case string:
			result[pattern] = v
		case map[string]any:
			executable, err := assocExecutor(pattern, v)
			if err != nil {
				return nil, err
			}
			result[pattern] = executable
		default:
			return nil, fmt.Errorf("associations: %q must be an executable path or an exec table", pattern)
		}
	}
	return result, nil
}

func assocExecutor(pattern string, table map[string]any) (string, error) {
	if tp, ok := table["type"]; ok && tp != "exec" {
		return "", fmt.Errorf("associations: %q: executor.type must be \"exec\", got %v", pattern, tp)
	}
	if _, hasEntrypoint := table["entrypoint"]; hasEntrypoint {
		return "", fmt.Errorf("associations: %q: there is deliberately no entrypoint field; the matched file occupies that slot", pattern)
	}

	executable, ok := table["executable"].(string)
	if !ok || executable == "" {
		return "", fmt.Errorf("associations: %q: executor.executable must be a non-empty path", pattern)
	}
	return executable, nil
}

func (c *Config) validate() error {
	for name, target := range c.Skripts {
		if !aliasNamePattern.MatchString(name) {
			return fmt.Errorf("skripts: invalid name %q", name)
		}
		if target == "" {
			return fmt.Errorf("skripts: %q has an empty target", name)
		}
	}
	for pattern, executor := range c.Associations {
		if executor == "" {
			return fmt.Errorf("associations: %q has an empty executor", pattern)
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return fmt.Errorf("associations: invalid pattern %q", pattern)
		}
	}
	return nil
}

// AliasTarget resolves an alias name to the path it points to.
//
// Absolute targets are kept, relative targets are resolved against the
// config file's directory and `~` expands to the user's home directory.
// Paths prefer forward slashes.
func (c *Config) AliasTarget(name string) (string, bool, error) {
	raw, found := c.Skripts[name]
	if !found {
		return "", false, nil
	}

	expanded, isHomed, err := expandHome(raw, c.home)
	if err != nil {
		return "", false, fmt.Errorf("skripts: %q: %w", name, err)
	}

	// A `~`-expanded path is already anchored in the user's home; only an
	// ordinary relative path resolves against the config directory.
	target := expanded
	if !isHomed && !filepath.IsAbs(target) {
		target = filepath.Join(c.Dir, target)
	}
	return filepath.ToSlash(target), true, nil
}

func expandHome(p, home string) (string, bool, error) {
	switch {
	case p == "~":
		if home == "" {
			return "", false, fmt.Errorf("cannot expand ~: no home directory")
		}
		return home, true, nil
	case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
		if home == "" {
			return "", false, fmt.Errorf("cannot expand %q: no home directory", p)
		}
		return filepath.Join(home, p[2:]), true, nil
	default:
		return p, false, nil
	}
}
