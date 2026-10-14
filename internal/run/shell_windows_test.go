//go:build windows

package run

import (
	"testing"
)

func TestCrtQuote(t *testing.T) {
	tests := []struct {
		name     string
		arg      string
		expected string
	}{
		{name: "plain", arg: "abc", expected: "abc"},
		{name: "empty", arg: "", expected: `""`},
		{name: "space", arg: "a b", expected: `"a b"`},
		{name: "raw string with tab", arg: "a\tb", expected: "\"a\tb\""},
		{name: "inner quote doubled", arg: `a"b`, expected: `"a""b"`},
		{name: "quoted phrase", arg: `say "hi" now`, expected: `"say ""hi"" now"`},
		{name: "no wrap trigger, trailing backslash kept", arg: `C:\path\`, expected: `C:\path\`},
		{name: "trailing backslash doubled when wrapped", arg: `C:\dir path\`, expected: `"C:\dir path\\"`},
		{name: "lone backslash kept single", arg: `x\ y`, expected: `"x\ y"`},
		{name: "metachar wrapped", arg: `met&achar`, expected: `"met&achar"`},
		{name: "backslash before quote doubled", arg: `a\"b`, expected: `"a\\""b"`},
		{name: "quote then trailing backslash", arg: `tr "\"`, expected: `"tr ""\\"""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crtQuote(tt.arg)
			if got != tt.expected {
				t.Errorf("crtQuote(%q) = %q, want %q", tt.arg, got, tt.expected)
			}
		})
	}
}

func TestShellCommand_CmdLine(t *testing.T) {
	cmd, err := shellCommand(`echo "a & b"`, []string{`met&achar`, "plain arg"})
	if err != nil {
		t.Fatalf("shellCommand() unexpected error: %v", err)
	}

	want := `cmd /C call echo "a & b" "met&achar" "plain arg"`
	if got := cmd.SysProcAttr.CmdLine; got != want {
		t.Errorf("CmdLine = %q, want %q", got, want)
	}
	if cmd.Path == "" {
		t.Error("cmd.Path is empty, want cmd.exe resolved through PATH")
	}
}
