package interp

import (
	"strings"
	"testing"
)

func testBuiltins(name string) (string, bool) {
	if name == "SKRIPT_ROOT" {
		return "/proj", true
	}
	return "", false
}

func testNamespaced(ns, name string) (string, bool) {
	if ns == "env" {
		switch name {
		case "HOME":
			return "/home/kael", true
		case "SELF":
			return "${SKRIPT_ROOT}", true
		}
	}
	return "", false
}

func TestExpand(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		expected string
		wantErr  string
	}{
		{name: "plain text", in: "hello", expected: "hello"},
		{name: "builtin", in: "${SKRIPT_ROOT}/bin", expected: "/proj/bin"},
		{name: "namespaced", in: "${env:HOME}", expected: "/home/kael"},
		{
			name:     "escape collapses to dollar",
			in:       "$${SKRIPT_ROOT}/bin",
			expected: "${SKRIPT_ROOT}/bin",
		},
		{name: "escape then real ref", in: "$${A} ${SKRIPT_ROOT}", expected: "${A} /proj"},
		{name: "lone dollar literal", in: "cost 5$ and ${SKRIPT_ROOT}", expected: "cost 5$ and /proj"},
		{name: "dollar at end", in: "end$", expected: "end$"},
		{
			name:     "single pass, nested ref not rescanned",
			in:       "${env:SELF} r",
			expected: "${SKRIPT_ROOT} r",
		},
		{
			name:    "unknown builtin",
			in:      "${NOPE}",
			wantErr: `unknown variable ${NOPE}`,
		},
		{
			name:    "unknown namespace",
			in:      "${vault:KEY}",
			wantErr: `unknown variable ${vault:KEY}`,
		},
		{
			name:    "unknown env var is an error",
			in:      "${env:NOPE}",
			wantErr: `unknown variable ${env:NOPE}`,
		},
		{
			name:    "empty reference",
			in:      "${}",
			wantErr: "empty reference",
		},
		{
			name:    "malformed namespaced",
			in:      "${env:}",
			wantErr: `malformed reference ${env:}`,
		},
		{
			name:    "malformed namespaced, empty namespace",
			in:      "${:NAME}",
			wantErr: `malformed reference ${:NAME}`,
		},
		{
			name:    "unclosed reference",
			in:      "a ${SKRIPT_ROOT} b ${x",
			wantErr: "unclosed reference",
		},
		{name: "brace without dollar stays", in: "a {x} b", expected: "a {x} b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(tt.in, testBuiltins, testNamespaced)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Expand(%q) = %q, want error containing %q", tt.in, got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Expand(%q) error = %q, want containing %q", tt.in, err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Expand(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.expected {
				t.Errorf("Expand(%q) = %q, want %q", tt.in, got, tt.expected)
			}
		})
	}
}
