package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newApp(stderr *bytes.Buffer) *App {
	return &App{Stderr: stderr, ConfigPath: "unused", HomeDir: "unused"}
}

func TestRun_Usage(t *testing.T) {
	var stderr bytes.Buffer
	app := newApp(&stderr)

	code := app.Run(nil)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "skript: usage:") {
		t.Errorf("stderr = %q, want 'skript: usage:'", got)
	}
}

func TestRun_MissingConfigIsNotAnError(t *testing.T) {
	var stderr bytes.Buffer
	app := &App{
		Stderr:     &stderr,
		ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		HomeDir:    t.TempDir(),
	}

	// A missing target file still fails, but not because of the config.
	code := app.Run([]string{"ghost.lua"})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if msg := stderr.String(); strings.Contains(msg, "config") {
		t.Errorf("stderr = %q, want the target error, not a config error", msg)
	}
	if msg := stderr.String(); !strings.Contains(msg, "skript: target") {
		t.Errorf("stderr = %q, want 'skript: target'", msg)
	}
}

func TestRun_UnparseableConfigIsOneLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("associations = { broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	app := &App{Stderr: &stderr, ConfigPath: path, HomeDir: dir}

	code := app.Run([]string{"anything"})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}

	msg := stderr.String()
	if !strings.HasPrefix(msg, "skript: ") {
		t.Errorf("stderr = %q, want 'skript: ' prefix", msg)
	}
	if got := strings.Split(strings.TrimSuffix(msg, "\n"), "\n"); len(got) != 1 {
		t.Errorf("stderr has %d lines, want exactly one: %q", len(got), msg)
	}
}

func TestRun_LoadingErrorIsOneLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[skripts]\n\"bad name\" = \"x\""), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	app := &App{Stderr: &stderr, ConfigPath: path, HomeDir: dir}

	code := app.Run([]string{"anything"})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "invalid name") {
		t.Errorf("stderr = %q, want the alias validation error", msg)
	}
	if got := strings.Split(strings.TrimSuffix(msg, "\n"), "\n"); len(got) != 1 {
		t.Errorf("stderr has %d lines, want exactly one: %q", len(got), msg)
	}
}
