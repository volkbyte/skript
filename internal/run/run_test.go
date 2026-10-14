package run

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/volkbyte/skript/internal/rc"
	"github.com/volkbyte/skript/internal/resolve"
)

func writeFile(t *testing.T, part ...string) string {
	t.Helper()
	path := filepath.Join(part...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("-- anything"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// platformProgram is easy to LookPath on all supported platforms.
func platformProgram() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

func execTarget(root, entrypoint string) resolve.Target {
	return resolve.Target{
		Root: root,
		Executor: rc.Executor{
			Type:       rc.TypeExec,
			Executable: platformProgram(),
			Args:       []string{"-v"},
			Entrypoint: entrypoint,
		},
	}
}

func TestComposeExec_RCEntrypointResolvesAgainstRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.lua")

	cmd, err := Compose(execTarget(root, "main.lua"), []string{"--name", "x"})
	if err != nil {
		t.Fatalf("Compose() unexpected error: %v", err)
	}

	want := []string{platformProgram(), "-v", filepath.Join(root, "main.lua"), "--name", "x"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("Args = %q, want %q", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
	if cmd.Dir != "" {
		t.Errorf("Dir = %q, want inherited (empty)", cmd.Dir)
	}
}

func TestComposeComposeErrors(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name   string
		target resolve.Target
		vargs  []string
		want   string
	}{
		{
			name:   "entrypoint missing in root",
			target: execTarget(root, "ghost.lua"),
			vargs:  nil,
			want:   "does not exist in SKRIPT_ROOT",
		},
		{
			name: "absolute entrypoint rejected",
			target: resolve.Target{
				Root: root,
				Executor: rc.Executor{
					Type:       rc.TypeExec,
					Executable: platformProgram(),
					Entrypoint: filepath.Join(root, "abs.lua"),
				},
			},
			want: "must be relative and resolve against SKRIPT_ROOT",
		},
		{
			name: "missing entrypoint field",
			target: resolve.Target{
				Root:     root,
				Executor: rc.Executor{Type: rc.TypeExec, Executable: platformProgram()},
			},
			want: "executor.entrypoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compose(tt.target, tt.vargs)
			if err == nil {
				t.Fatalf("Compose() = nil error, want containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Compose() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCompose_FileTargetKeepsSpelling(t *testing.T) {
	root := t.TempDir()
	// Compose does not parse the rc; it only needs the target record.
	_ = writeFile(t, root, ".skriptrc")

	// A file target occupies the entrypoint slot itself; the rc's own
	// entrypoint field (here: invalid) is ignored.
	target := resolve.Target{
		Root: root,
		Executor: rc.Executor{
			Type:       rc.TypeExec,
			Executable: platformProgram(),
			Args:       []string{"-v"},
			Entrypoint: "must-not-resolve.lua",
		},
		File: "target.lua",
	}

	cmd, err := Compose(target, nil)
	if err != nil {
		t.Fatalf("Compose() unexpected error: %v", err)
	}
	want := []string{platformProgram(), "-v", "target.lua"}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestCompose_CommandArrayForm(t *testing.T) {
	target := resolve.Target{
		Executor: rc.Executor{
			Type:        rc.TypeCommand,
			CommandList: []string{platformProgram(), "/c", "make"},
		},
	}

	cmd, err := Compose(target, []string{"clean", "all"})
	if err != nil {
		t.Fatalf("Compose() unexpected error: %v", err)
	}
	want := []string{platformProgram(), "/c", "make", "clean", "all"}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestMergedEnvironment(t *testing.T) {
	t.Run("overrides inherited and appends others", func(t *testing.T) {
		t.Setenv("SKRIPT_OVERRIDDEN", "old")
		t.Setenv("SKRIPT_KEPT", "kept")

		env := mergedEnvironment([]string{"SKRIPT_OVERRIDDEN=new", "SKRIPT_ADDED=added"})
		got := map[string]string(env2map(env))
		if got["SKRIPT_OVERRIDDEN"] != "new" {
			t.Errorf("override = %q, want new", got["SKRIPT_OVERRIDDEN"])
		}
		if got["SKRIPT_KEPT"] != "kept" {
			t.Errorf("kept = %q, want kept", got["SKRIPT_KEPT"])
		}
		if got["SKRIPT_ADDED"] != "added" {
			t.Errorf("added = %q, want added", got["SKRIPT_ADDED"])
		}
	})

	t.Run("duplicate additions collapse, last wins", func(t *testing.T) {
		t.Setenv("SKRIPT_BASE", "base")

		env := mergedEnvironment([]string{"SKRIPT_BASE=first", "SKRIPT_BASE=second"})
		if n := countEnv(env, "SKRIPT_BASE="); n != 1 {
			t.Errorf("found %d SKRIPT_BASE entries, want 1: %q", n, env)
		}
		if !containsExact(env, "SKRIPT_BASE=second") {
			t.Errorf("env = %q, want last addition to win", env)
		}
	})
}

func env2map(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		key, value, _ := strings.Cut(kv, "=")
		m[key] = value
	}
	return m
}

func containsExact(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}

func countEnv(env []string, prefix string) int {
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			n++
		}
	}
	return n
}
