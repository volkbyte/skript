// Package e2e runs the built skript binary against real fixture trees.
//
// It builds cmd/skript and the echoargs test helper with the local Go
// toolchain. When the binaries are prebuilt (set SKRIPT_TEST_SKRIPT and
// SKRIPT_TEST_ECHOARGS to their paths), they are used instead — so this
// suite also runs where no Go toolchain is installed, e.g. inside WSL
// against a cross-compiled test binary.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	skriptBin string
	echoBin   string
	allTmp    string
)

func TestMain(m *testing.M) {
	var err error
	allTmp, err = os.MkdirTemp("", "skript-e2e")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(3)
	}

	skriptBin, err = locate("SKRIPT_TEST_SKRIPT", "skript", "github.com/volkbyte/skript/cmd/skript")
	if err != nil {
		os.RemoveAll(allTmp)
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(3)
	}
	echoBin, err = locate("SKRIPT_TEST_ECHOARGS", "echoargs", "./internal/testdata/echoargs")
	if err != nil {
		os.RemoveAll(allTmp)
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(3)
	}

	code := m.Run()
	os.RemoveAll(allTmp)
	os.Exit(code)
}

// locate returns a prebuilt binary from the environment or builds it.
func locate(envName, name, goPackage string) (string, error) {
	if p := os.Getenv(envName); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	out := filepath.Join(allTmp, name+ext)
	cmd := exec.Command("go", "build", "-o", out, goPackage)
	cmd.Dir = moduleRoot()
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// moduleRoot is the directory containing go.mod.
func moduleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// home is a fake user home: its config.toml governs the runs.
type home struct {
	t   *testing.T
	dir string
}

func newHome(t *testing.T) *home {
	t.Helper()
	dir := t.TempDir()
	return &home{t: t, dir: dir}
}

func (h *home) config(content string) {
	h.t.Helper()
	path := filepath.Join(h.dir, ".volkbyte", "skript", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// write creates a file with dirs and returns its native path.
func write(t *testing.T, path, content string) string {
	t.Helper()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// tomlStr renders a TOML basic string literal.
func tomlStr(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

// run executes the skript binary with a fake home and a working directory,
// and returns the exit code plus the child's stdout and stderr (the binary
// itself writes errors to stderr only).
func (h *home) run(cwd string, args ...string) (int, string, string) {
	h.t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(skriptBin, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), homeEnv()+"="+h.dir)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	}
	h.t.Fatalf("running %q: %v\nstderr: %s", args, err, stderr.String())
	return -1, "", ""
}

func homeEnv() string {
	if runtime.GOOS == "windows" {
		return "USERPROFILE"
	}
	return "HOME"
}
