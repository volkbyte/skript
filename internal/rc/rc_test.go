package rc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRoot creates a skript root directory containing a .skriptrc with the
// given body.
func writeRoot(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".skriptrc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

const execTOML = `
[meta]
version = "1"

[executor]
type = "exec"
executable = "${SKRIPT_ROOT}/bin/lua"
args = ["-v", "${env:SKRIPT_FLAG}"]
entrypoint = "main/$${FILE}.lua"
`

func TestLoadExec(t *testing.T) {
	t.Setenv("SKRIPT_FLAG", "verbose")
	root := writeRoot(t, execTOML)

	rc, err := Load(root)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if rc.Type != TypeExec {
		t.Errorf("Type = %v, want TypeExec", rc.Type)
	}
	if !strings.Contains(rc.Executable, root) || !strings.Contains(rc.Executable, "bin/lua") {
		t.Errorf("Executable = %q, want ${SKRIPT_ROOT} expanded to %q", rc.Executable, root)
	}
	if len(rc.Args) != 2 || rc.Args[0] != "-v" || rc.Args[1] != "verbose" {
		t.Errorf("Args = %q, want [-v verbose]", rc.Args)
	}
	if rc.Entrypoint != "main/${FILE}.lua" {
		t.Errorf("Entrypoint = %q, want main/${FILE}.lua", rc.Entrypoint)
	}
}

const commandStrTOML = `
[meta]
version = "1"

[executor]
type = "command"
command = "lua ${SKRIPT_ROOT}/main.lua"
`

func TestLoadCommandString(t *testing.T) {
	root := writeRoot(t, commandStrTOML)

	rc, err := Load(root)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if rc.Type != TypeCommand {
		t.Errorf("Type = %v, want TypeCommand", rc.Type)
	}
	if !strings.Contains(rc.CommandStr, root+"/main.lua") {
		t.Errorf("CommandStr = %q, want rooted at %q", rc.CommandStr, root)
	}
	if rc.CommandList != nil {
		t.Errorf("CommandList = %q, want nil", rc.CommandList)
	}
}

const commandListTOML = `
[meta]
version = "1"

[executor]
type = "command"
command = ["lua", "-e", "print('hi')"]
`

func TestLoadCommandList(t *testing.T) {
	root := writeRoot(t, commandListTOML)

	rc, err := Load(root)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if rc.CommandStr != "" {
		t.Errorf("CommandStr = %q, want empty", rc.CommandStr)
	}
	want := []string{"lua", "-e", "print('hi')"}
	if len(rc.CommandList) != len(want) {
		t.Fatalf("CommandList = %q, want %q", rc.CommandList, want)
	}
	for i, w := range want {
		if rc.CommandList[i] != w {
			t.Errorf("CommandList[%d] = %q, want %q", i, rc.CommandList[i], w)
		}
	}
}

func TestLoadEnvironment(t *testing.T) {
	body := `
[meta]
version = "1"

[executor]
type = "command"
command = ["true"]

[executor.environment]
NAME = "plain ${SKRIPT_ROOT}"
PATH = ["${SKRIPT_ROOT}/bin", "${env:SKRIPT_EXTRA_PATH}"]
`
	t.Setenv("SKRIPT_EXTRA_PATH", "/extra")
	root := writeRoot(t, body)

	rc, err := Load(root)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	name := "NAME=plain " + root
	if !containsExact(rc.Env, name) {
		t.Errorf("Env = %q, want entry %q", rc.Env, name)
	}
	sep := string(os.PathListSeparator)
	path := "PATH=" + root + "/bin" + sep + "/extra"
	if !containsExact(rc.Env, path) {
		t.Errorf("Env = %q, want entry %q", rc.Env, path)
	}
	if len(rc.Env) != 2 {
		t.Errorf("Env has %d entries, want 2: %q", len(rc.Env), rc.Env)
	}
}

func containsExact(env []string, e string) bool {
	for _, item := range env {
		if item == e {
			return true
		}
	}
	return false
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing meta",
			body: `[executor]\ntype = "command"\ncommand = "true"`,
			want: `meta.version must be "1"`,
		},
		{
			name: "wrong version",
			body: `[meta]\nversion = "2"\n[executor]\ntype = "command"\ncommand = "true"`,
			want: `meta.version must be "1"`,
		},
		{
			name: "unparseable toml",
			body: `meta = { version = `,
			want: "parse",
		},
		{
			name: "unknown executor type",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "shell"`,
			want: `executor.type must be "exec" or "command", got "shell"`,
		},
		{
			name: "missing command",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "command"`,
			want: "executor.command is required",
		},
		{
			name: "non-string command contents",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "command"\ncommand = [42]`,
			want: "must be a string or an array of strings",
		},
		{
			name: "empty command string",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "command"\ncommand = ""`,
			want: "executor.command is empty",
		},
		{
			name: "exec without executable",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "exec"`,
			want: "executor.executable is required",
		},
		{
			name: "entrypoint on command executor",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "command"\ncommand = "true"\nentrypoint = "main.lua"`,
			want: `executor.entrypoint is only valid for "exec" executors`,
		},
		{
			name: "executable on command executor",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "command"\ncommand = "true"\nexecutable = "lua"`,
			want: `executor.executable is only valid for "exec" executors`,
		},
		{
			name: "args on command executor",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "command"\ncommand = "true"\nargs = ["-v"]`,
			want: `executor.args is only valid for "exec" executors`,
		},
		{
			name: "command on exec executor",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "exec"\nexecutable = "lua"\ncommand = "make"`,
			want: `executor.command is only valid for "command" executors`,
		},
		{
			name: "unknown variable in executable",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "exec"\nexecutable = "${NOPE}"`,
			want: `unknown variable ${NOPE}`,
		},
		{
			name: "unsuitable env value type",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "exec"\nexecutable = "lua"\n\n[executor.environment]\nK = 4`,
			want: `environment: "K" must be a string or an array of strings`,
		},
		{
			name: "invalid env key",
			body: `[meta]\nversion = "1"\n[executor]\ntype = "exec"\nexecutable = "lua"\n\n[executor.environment]\n"K=L" = "v"`,
			want: `environment: invalid key`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeRoot(t, strings.ReplaceAll(tt.body, `\n`, "\n"))
			_, err := Load(root)
			if err == nil {
				t.Fatalf("Load() = nil error, want containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLoad_MissingRCFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("Load() = nil error, want read failure")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error = %q, want wrapping the read failure", err)
	}
}
