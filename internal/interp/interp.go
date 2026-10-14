// Package interp implements the single-pass `${}` string interpolation
// used by string values in a `.skriptrc` file.
//
// Syntax and rules (see notes/config/skriptrc.ts):
//
//   - `${NAME}` references a built-in variable, e.g. `${SKRIPT_ROOT}`.
//   - `${ns:NAME}` references a namespaced variable, e.g. `${env:HOME}`.
//   - `$$` collapses to a literal `$`, so `$${HOME}` yields `${HOME}`.
//   - A lone `$` not followed by `$` or `{` is literal text.
//   - Unknown variables are an error, not an empty string.
//   - Interpolation runs once: expanded values are never rescanned, so a
//     variable whose value contains `${...}` is inserted as-is.
package interp

import (
	"fmt"
	"strings"
)

// Builtins resolves built-in names such as SKRIPT_ROOT.
// ok is false for unknown built-ins.
type Builtins func(name string) (value string, ok bool)

// Namespaced resolves namespaced references of the form "ns:name".
// ok is false for an unknown namespace or variable.
type Namespaced func(namespace, name string) (value string, ok bool)

// Expand performs single-pass interpolation of s.
func Expand(s string, builtins Builtins, namespaced Namespaced) (string, error) {
	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		c := s[i]
		if c != '$' {
			b.WriteByte(c)
			i++
			continue
		}

		switch {
		case i+1 < len(s) && s[i+1] == '$':
			// "$$" escapes to a literal "$"; the following text is copied
			// verbatim, so "$${HOME}" stays "${HOME}" and is never looked up.
			b.WriteByte('$')
			i += 2

		case i+1 < len(s) && s[i+1] == '{':
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				return "", fmt.Errorf("interpolate: unclosed reference in %q", s)
			}
			value, err := resolveRef(s[i+2:i+2+end], builtins, namespaced)
			if err != nil {
				return "", fmt.Errorf("interpolate: %w", err)
			}
			b.WriteString(value)
			i += 2 + end + 1

		default:
			// A lone "$" is literal text.
			b.WriteByte('$')
			i++
		}
	}

	return b.String(), nil
}

func resolveRef(ref string, builtins Builtins, namespaced Namespaced) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty reference")
	}

	if ns, name, ok := strings.Cut(ref, ":"); ok {
		if ns == "" || name == "" {
			return "", fmt.Errorf("malformed reference ${%s}", ref)
		}
		value, ok := namespaced(ns, name)
		if !ok {
			return "", fmt.Errorf("unknown variable ${%s}", ref)
		}
		return value, nil
	}

	value, ok := builtins(ref)
	if !ok {
		return "", fmt.Errorf("unknown variable ${%s}", ref)
	}
	return value, nil
}
