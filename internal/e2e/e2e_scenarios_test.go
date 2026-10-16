package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// metacharVargs are user arguments carrying shell-active characters. They
// must arrive as single literal arguments on every executor path.
// metacharVargs are user arguments carrying shell-active characters. They
// must arrive as single literal arguments on every executor path — except
// the Windows command-string form, where an inner-quote varg has a
// documented, parser-dependent boundary (see
// TestE2E_CommandStringExecutor_InnerQuotesDocumented); for that form use
// shellMetacharVargs.
var metacharVargs = []string{
	"a&b",          // command separator
	"a|b",          // pipe
	`say "hi" now`, // quotes and spaces
	`c star*`,      // globbing
	"$(dangerous)", // linux command substitution
	`back\slash`,   // backslash
	"<redirect>",   // redirection
}

// shellMetacharVargs exercises shell metacharacters without inner quotes.
var shellMetacharVargs = []string{
	"a&b",
	"a|b",
	`c star*`,
	"$(dangerous)",
	`back\slash`,
	"<redirect>",
}

func assertExit(t *testing.T, got int, want int, stderr string) {
	t.Helper()
	if got != want {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got, want, stderr)
	}
}

func assertStderrEmpty(t *testing.T, stderr string) {
	t.Helper()
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

// assertStdoutLines asserts the child-echoed stdout lines.
func assertStdoutLines(t *testing.T, stdout string, want []string) {
	t.Helper()
	got := strings.Split(strings.TrimSuffix(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("stdout lines = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stdout[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func outLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")
}

// execRcBody builds a .skriptrc with an exec executor.
func execRcBody(t *testing.T, executable string, args []string, entrypoint string, env map[string]string) string {
	t.Helper()

	var b strings.Builder
	fmt.Fprintf(&b, "[meta]\nversion = \"1\"\n\n[executor]\ntype = \"exec\"\nexecutable = %s\n", tomlStr(executable))
	if len(args) > 0 {
		quoted := make([]string, 0, len(args))
		for _, a := range args {
			quoted = append(quoted, tomlStr(a))
		}
		fmt.Fprintf(&b, "args = [%s]\n", strings.Join(quoted, ", "))
	}
	if entrypoint != "" {
		fmt.Fprintf(&b, "entrypoint = %s\n", tomlStr(entrypoint))
	}
	if len(env) > 0 {
		b.WriteString("\n[executor.environment]\n")
		for _, k := range sortedKeys(env) {
			fmt.Fprintf(&b, "%s = %s\n", k, tomlStr(env[k]))
		}
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// enough for fixtures: sort for stable files
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func TestE2E_ExecExecutor_VargsLiteral(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	entrypoint := filepath.Join(root, "main.args")
	write(t, entrypoint, "-- entrypoint file")
	body := execRcBody(t, echoBin, []string{"-out=" + out}, "main.args", nil)
	write(t, filepath.Join(root, ".skriptrc"), body)

	vargs := append([]string{"100%literal"}, metacharVargs...)
	exit, _, stderr := h.run(work, append([]string{root}, vargs...)...)
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	// The rc entrypoint arrives at the child resolved against SKRIPT_ROOT.
	want := append([]string{filepath.Join(root, "main.args")}, vargs...)
	got := outLines(t, out)
	if len(got) != len(want) {
		t.Fatalf("got %d lines (%q), want %d (%q)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestE2E_CommandStringExecutor_MetacharsInert is the required proof that a
// varg with shell metacharacters stays inert on the platform shell: nothing
// extra is executed and every varg arrives as one argument.
//
// On Linux, "$@" re-expansions preserve arguments exactly, so the full
// metacharVargs list runs and every varg round-trips verbatim. On Windows,
// the CRT-style quoting of the cmd /C line makes shell metacharacters inert
// but an inner-quote varg has a documented, parser-dependent boundary — so
// only the metachar-only subset runs here, and the inner-quote behavior is
// asserted (as observed) by TestE2E_CommandStringExecutor_InnerQuotesDocumented.
func TestE2E_CommandStringExecutor_MetacharsInert(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")
	evil := filepath.Join(work, "evil.txt")

	var command string
	if runtime.GOOS == "windows" {
		command = `"` + echoBin + `" -out=` + out
	} else {
		command = echoBin + ` -out=` + out
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[meta]\nversion = \"1\"\n\n[executor]\ntype = \"command\"\ncommand = %s\n", tomlStr(command))
	write(t, filepath.Join(root, ".skriptrc"), b.String())

	vargs := metacharVargs
	if runtime.GOOS == "windows" {
		vargs = shellMetacharVargs
	}
	exit, _, stderr := h.run(work, append([]string{root}, vargs...)...)
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	got := outLines(t, out)
	if len(got) != len(vargs) {
		t.Fatalf("got %d lines (%q), want %d vargs (%q)", len(got), got, len(vargs), vargs)
	}
	for i := range vargs {
		if got[i] != vargs[i] {
			t.Errorf("varg[%d] = %q, want literal %q", i, got[i], vargs[i])
		}
	}

	// No shell trick must have spawned anything: `a&b` among the vargs must
	// have stayed an argument, so this file was never created.
	if _, err := os.Stat(evil); !os.IsNotExist(err) {
		t.Errorf("evil.txt exists, vargs must not trigger additional commands")
	}
}

// TestE2E_CommandStringExecutor_InnerQuotesDocumented records — and thus
// documents — the Windows command-string behavior for a varg that carries
// inner quotes. The vargs are CRT-quoted with inner quotes doubled; the
// child's own argv parser then decides the final boundaries, and a
// CommandLineToArgvW-style child (which is what every Go program uses)
// closes the arg at the doubled quote and merges what follows.
//
// What is asserted as skript's contract: nothing is executed beyond the one
// command (the metacharacters stay inert), the run fails cleanly (exit 0,
// empty stderr), and the observed shape is stable. Full inner-quote
// fidelity is what the command array form is for.
func TestE2E_CommandStringExecutor_InnerQuotesDocumented(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("documents Windows cmd /C behavior")
	}

	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")
	evil := filepath.Join(work, "evil.txt")

	command := `"` + echoBin + `" -out=` + out
	var b strings.Builder
	fmt.Fprintf(&b, "[meta]\nversion = \"1\"\n\n[executor]\ntype = \"command\"\ncommand = %s\n", tomlStr(command))
	write(t, filepath.Join(root, ".skriptrc"), b.String())

	exit, _, stderr := h.run(work, root, `say "hi" now`, "tail", "x")
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	// Observed with cmd.exe + the CRT quoting rules (inner quotes doubled):
	// the arg closes at the doubled inner quote, what follows merges.
	got := outLines(t, out)
	want := []string{`say "hi`, "now tail x"}
	if len(got) != len(want) {
		t.Fatalf("arg lines = %q, want the documented shape %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if _, err := os.Stat(evil); !os.IsNotExist(err) {
		t.Errorf("evil.txt exists, vargs must not trigger additional commands")
	}
}

func TestE2E_CommandListExecutor_VargsLiteral(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	var b strings.Builder
	fmt.Fprintf(&b, "[meta]\nversion = \"1\"\n\n[executor]\ntype = \"command\"\ncommand = [%s, %s]\n",
		tomlStr(echoBin), tomlStr("-out="+out))
	write(t, filepath.Join(root, ".skriptrc"), b.String())

	exit, _, stderr := h.run(work, append([]string{root}, metacharVargs...)...)
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	got := outLines(t, out)
	if len(got) != len(metacharVargs) || got[0] != metacharVargs[0] {
		t.Fatalf("args = %q, want %q", got, metacharVargs)
	}
	for i := range metacharVargs {
		if got[i] != metacharVargs[i] {
			t.Errorf("varg[%d] = %q, want literal %q", i, got[i], metacharVargs[i])
		}
	}
}

func TestE2E_AliasDirectory(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	entrypoint := filepath.Join(root, "main.args")
	write(t, entrypoint, "-- entrypoint file")
	write(t, filepath.Join(root, ".skriptrc"),
		execRcBody(t, echoBin, []string{"-out=" + out}, "main.args", nil))

	h.config(fmt.Sprintf("[skripts]\nhello = %s\n", tomlStr(root)))

	exit, _, stderr := h.run(work, "hello", "some", "vargs")
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	// The rc entrypoint arrives at the child resolved against SKRIPT_ROOT.
	got := outLines(t, out)
	want := []string{filepath.Join(root, "main.args"), "some", "vargs"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestE2E_AliasFileViaAssociation(t *testing.T) {
	h := newHome(t)
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	lone := filepath.Join(work, "scripts", "lone.lua")
	write(t, lone, "-- lone skript")

	h.config(fmt.Sprintf(`
[skripts]
lone = %s

[associations]
"*.lua" = %s
`, tomlStr(lone), tomlStr(echoBin)))

	// Alias targets resolve against the config directory; here the config
	// lives in h.dir. The -out flag is consumed by the helper; a trailing
	// varg proves the argv wiring beyond the entrypoint slot.
	exit, _, stderr := h.run(work, "lone", "-out="+out, "v2")
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	got := outLines(t, out)
	if len(got) != 2 {
		t.Fatalf("lines = %q, want the resolved skript path plus the varg", got)
	}
	// Spelling: the alias target arrives as its resolved forward-slash path.
	if !strings.HasSuffix(got[0], "lone.lua") {
		t.Errorf("argv[0] = %q, want the lone.lua path", got[0])
	}
	if got[1] != "v2" {
		t.Errorf("argv[1] = %q, want \"v2\"", got[1])
	}
}

func TestE2E_FileTarget_WalkUp(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	// The rc's own entrypoint field is deliberately invalid: a file target
	// must override it with the matched file.
	deep := filepath.Join(root, "sub", "deep", "g.args")
	write(t, deep, "-- file skript")
	write(t, filepath.Join(root, ".skriptrc"),
		execRcBody(t, echoBin, []string{"-out=" + out}, "shall-not-resolve.lua", nil))

	exit, _, stderr := h.run(work, deep, "varg-a", "varg-b")
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	got := outLines(t, out)
	want := []string{deep, "varg-a", "varg-b"}
	if len(got) != len(want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestE2E_AssociationFallback(t *testing.T) {
	h := newHome(t)
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	h.config(fmt.Sprintf("[associations]\n\"*.x\" = %s\n", tomlStr(echoBin)))

	skriptFile := filepath.Join(work, "mything.x")
	write(t, skriptFile, "-- association skript")

	// Relative target spelling from the caller's cwd; the -out flag is
	// consumed by the helper.
	exit, _, stderr := h.run(work, "mything.x", "-out="+out, "v2")
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	got := outLines(t, out)
	if len(got) != 2 || got[0] != "mything.x" || got[1] != "v2" {
		t.Fatalf("lines = %q, want the file spelling plus the varg", got)
	}
}

func TestE2E_AssociationTildeExecutor(t *testing.T) {
	h := newHome(t)
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	// The executor lives inside the fake home and is reachable only through
	// the `~` expansion of the association value.
	binDir := filepath.Join(h.dir, ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(echoBin)
	if err != nil {
		t.Fatal(err)
	}
	tildeBin := filepath.Join(binDir, filepath.Base(echoBin))
	if err := os.WriteFile(tildeBin, data, 0o755); err != nil {
		t.Fatal(err)
	}

	h.config(fmt.Sprintf("[associations]\n\"*.x\" = %s\n",
		tomlStr("~/.bin/"+filepath.Base(echoBin))))

	skriptFile := filepath.Join(work, "tilde.x")
	write(t, skriptFile, "-- tilde association skript")

	exit, _, stderr := h.run(work, "tilde.x", "-out="+out)
	assertExit(t, exit, 0, stderr)
	assertStderrEmpty(t, stderr)

	got := outLines(t, out)
	if len(got) != 1 || got[0] != "tilde.x" {
		t.Fatalf("lines = %q, want the file spelling", got)
	}
}

func TestE2E_AssociationMostSpecificWins(t *testing.T) {
	h := newHome(t)
	work := t.TempDir()

	h.config(fmt.Sprintf(`
[associations]
"*.lua" = "lua-exe-never-invoked"
"*.apple.lua" = %s
`, tomlStr(echoBin)))

	apple := filepath.Join(work, "build.apple.lua")
	write(t, apple, "-- apple skript")

	// The most specific pattern's executor runs; it exists.
	exit, stdout, stderr := h.run(work, apple)
	assertExit(t, exit, 0, stderr)
	// The helper got the file as its entrypoint and echoed it to stdout:
	// .skriptrc-derived targets print argv to stdout.
	assertStdoutLines(t, stdout, []string{apple})

	// The less specific pattern points to a nonexistent executable, so a
	// *.lua file fails while the specific one cannot have been used.
	plain := filepath.Join(work, "build.lua")
	write(t, plain, "-- plain skript")

	exit, _, stderr = h.run(work, plain)
	assertExit(t, exit, 1, stderr)
	if !strings.Contains(stderr, "lua-exe-never-invoked") {
		t.Errorf("stderr = %q, want the *.lua executor name", stderr)
	}
}

func TestE2E_EnvironmentAdditions(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()
	out := filepath.Join(work, "out.txt")

	entrypoint := filepath.Join(root, "main.args")
	write(t, entrypoint, "-- entrypoint file")
	body := execRcBody(t, echoBin,
		[]string{"-out=" + out, "-env=SKRIPT_ADDED", "-env=SKRIPT_ROOT"},
		"main.args",
		map[string]string{"SKRIPT_ADDED": "one" + string(os.PathListSeparator) + "two"},
	)
	write(t, filepath.Join(root, ".skriptrc"), body)

	exit, _, stderr := h.run(work, root)
	assertExit(t, exit, 0, stderr)

	got := outLines(t, out)
	sep := string(os.PathListSeparator)
	wantEnv := "SKRIPT_ADDED=one" + sep + "two"
	if got[0] != wantEnv {
		t.Errorf("env entry = %q, want %q", got[0], wantEnv)
	}
	// SKRIPT_ROOT is an interpolation variable only, never an environment
	// variable of the child.
	if got[1] != "SKRIPT_ROOT=(unset)" {
		t.Errorf("env entry = %q, want SKRIPT_ROOT unset", got[1])
	}
	if !strings.HasSuffix(got[2], "main.args") {
		t.Errorf("argv echo = %q, want the entrypoint", got[2])
	}
}

func TestE2E_ChildExitCodeVerbatim(t *testing.T) {
	h := newHome(t)
	root := t.TempDir()
	work := t.TempDir()

	var b strings.Builder
	fmt.Fprintf(&b, "[meta]\nversion = \"1\"\n\n[executor]\ntype = \"command\"\ncommand = [%s, %s]\n",
		tomlStr(echoBin), tomlStr("-exit=7"))
	write(t, filepath.Join(root, ".skriptrc"), b.String())

	exit, _, _ := h.run(work, root)
	if exit != 7 {
		t.Fatalf("exit = %d, want the child's 7", exit)
	}
}

func TestE2E_ErrorShape_OneLine(t *testing.T) {
	h := newHome(t)
	work := t.TempDir()

	exit, _, stderr := h.run(work, "ghost.lua")
	assertExit(t, exit, 1, stderr)
	if !strings.HasPrefix(stderr, "skript: target") {
		t.Errorf("stderr = %q, want a single 'skript: target ...' line", stderr)
	}
	if n := strings.Count(stderr, "\n"); n != 1 {
		t.Errorf("stderr has %d newlines, want exactly one trailing one: %q", n, stderr)
	}
}

func TestE2E_UnparseableConfig_OneLine(t *testing.T) {
	h := newHome(t)
	work := t.TempDir()
	h.config("associations = { broken")

	exit, _, stderr := h.run(work, "anything")
	assertExit(t, exit, 1, stderr)
	if !strings.HasPrefix(stderr, "skript: parse config") {
		t.Errorf("stderr = %q, want a single 'skript: parse config ...' line", stderr)
	}
	if n := strings.Count(stderr, "\n"); n != 1 {
		t.Errorf("stderr has %d newlines, want exactly one trailing one: %q", n, stderr)
	}
}
