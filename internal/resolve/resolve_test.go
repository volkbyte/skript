package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/volkbyte/skript/internal/config"
	"github.com/volkbyte/skript/internal/rc"
)

const luaRc = `
[meta]
version = "1"

[executor]
type = "exec"
executable = "lua"
entrypoint = "main.lua"
`

// rootWith creates a skript root containing a .skriptrc.
func rootWith(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".skriptrc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// emptyRoot creates a directory without a .skriptrc.
func emptyRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func associationsCfg() *config.Config {
	return &config.Config{
		Associations: map[string]string{
			"*.lua":       "lua",
			"*.apple.lua": "/opt/luaJIT/lua",
		},
	}
}

func TestResolve_Directory(t *testing.T) {
	root := rootWith(t, luaRc)

	target, err := Resolve(associationsCfg(), root)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if target.Root != root {
		t.Errorf("Root = %q, want %q", target.Root, root)
	}
	if target.Executor.Executable != "lua" {
		t.Errorf("Executable = %q, want %q", target.Executor.Executable, "lua")
	}
	if target.File != "" {
		t.Errorf("File = %q, want empty for a directory target", target.File)
	}
}

func TestResolve_FileInRoot(t *testing.T) {
	root := rootWith(t, luaRc)
	file := writeFile(t, root, "build.lua", "-- noop")

	target, err := Resolve(associationsCfg(), file)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if target.Root != root {
		t.Errorf("Root = %q, want %q", target.Root, root)
	}
	if target.File != file {
		t.Errorf("File = %q, want the target spelling %q", target.File, file)
	}
	if target.Executor.Entrypoint != "main.lua" {
		t.Errorf("Executor.Entrypoint = %q, want the rc entrypoint (composition must override it)",
			target.Executor.Entrypoint)
	}
}

func TestResolve_FileWalksUp(t *testing.T) {
	root := rootWith(t, luaRc)
	sub := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := writeFile(t, sub, "build.lua", "-- noop")

	target, err := Resolve(associationsCfg(), file)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if target.Root != root {
		t.Errorf("Root = %q, want nearest enclosing root %q", target.Root, root)
	}
	if target.File != file {
		t.Errorf("File = %q, want the target spelling %q", target.File, file)
	}
}

func TestResolve_FileWithoutRootFallsBackToAssociations(t *testing.T) {
	dir := emptyRoot(t)
	file := writeFile(t, dir, "build.apple.lua", "-- noop")

	target, err := Resolve(associationsCfg(), file)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if target.Root != "" {
		t.Errorf("Root = %q, want empty (no skript root)", target.Root)
	}
	if target.Executor.Type != rc.TypeExec {
		t.Errorf("Executor.Type = %v, want TypeExec", target.Executor.Type)
	}
	if target.Executor.Executable != "/opt/luaJIT/lua" {
		t.Errorf("Executable = %q, want most specific match /opt/luaJIT/lua", target.Executor.Executable)
	}
	if target.File != file {
		t.Errorf("File = %q, want the target spelling %q", target.File, file)
	}
}

func TestResolve_MoreSpecificAssociationWins(t *testing.T) {
	tests := []struct {
		filename   string
		executable string
	}{
		{"build.lua", "lua"},
		{"build.apple.lua", "/opt/luaJIT/lua"},
		{"plain.txt", "/usr/bin/txt-reader"},
	}

	cfg := &config.Config{
		Associations: map[string]string{
			"*.lua":       "lua",
			"*.apple.lua": "/opt/luaJIT/lua",
			"*.txt":       "/usr/bin/txt-reader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, found, err := MatchAssociations(cfg.Associations, tt.filename)
			if err != nil || !found {
				t.Fatalf("MatchAssociations(%q) = %q, %v, %v", tt.filename, got, found, err)
			}
			if got != tt.executable {
				t.Errorf("MatchAssociations(%q) = %q, want %q", tt.filename, got, tt.executable)
			}
		})
	}

	t.Run("no match", func(t *testing.T) {
		_, found, err := MatchAssociations(cfg.Associations, "image.png")
		if err != nil || found {
			t.Fatalf("MatchAssociations() = _, %v, %v, want _, false, nil", found, err)
		}
	})
}

func TestResolve_Missing(t *testing.T) {
	_, err := Resolve(associationsCfg(), filepath.Join(t.TempDir(), "ghost.lua"))
	if err == nil {
		t.Fatal("Resolve() = nil error, want not found")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want 'does not exist'", err)
	}
}

func TestResolve_DirectoryWithoutRC(t *testing.T) {
	dir := emptyRoot(t)
	_, err := Resolve(associationsCfg(), dir)
	if err == nil {
		t.Fatal("Resolve() = nil error, want 'not a skript root'")
	}
	if !strings.Contains(err.Error(), "no .skriptrc") {
		t.Errorf("error = %q, want mentioning the missing .skriptrc", err)
	}
}

func TestResolve_NoAssociationNoRC(t *testing.T) {
	dir := emptyRoot(t)
	file := writeFile(t, dir, "image.png", "…")

	_, err := Resolve(associationsCfg(), file)
	if err == nil {
		t.Fatal("Resolve() = nil error, want 'no executor'")
	}
	if !strings.Contains(err.Error(), "no executor for") {
		t.Errorf("error = %q, want 'no executor for'", err)
	}
}

func TestResolve_AliasWins(t *testing.T) {
	root := rootWith(t, luaRc)
	conflict := writeFile(t, root, "aliasfile.lua", "-- real file")

	cfg := &config.Config{
		Skripts: map[string]string{"aliasfile.lua": conflict},
	}

	target, err := Resolve(cfg, "aliasfile.lua")
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if target.Root != root {
		t.Errorf("Root = %q, want %q (alias target resolution)", target.Root, root)
	}
}

func TestResolve_AliasTargetNotFound(t *testing.T) {
	cfg := &config.Config{
		Skripts: map[string]string{"ghost": "/definitely/not/here.lua"},
	}
	_, err := Resolve(cfg, "ghost")
	if err == nil {
		t.Fatal("Resolve() = nil error, want not found for alias target")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want 'does not exist'", err)
	}
}
