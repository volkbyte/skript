//go:build linux

package run

import "testing"

func TestShellCommand_ShMinusC(t *testing.T) {
	cmd, err := shellCommand(`printf '%s\n'`, []string{"met&achar", "plain arg"})
	if err != nil {
		t.Fatalf("shellCommand() unexpected error: %v", err)
	}

	want := []string{"sh", "-c", `printf '%s\n' "$@"`, `printf '%s\n'`, "met&achar", "plain arg"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("Args = %q, want %q", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}
