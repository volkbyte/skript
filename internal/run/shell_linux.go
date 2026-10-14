//go:build linux

package run

import "os/exec"

// shellCommand builds the invocation for a command executor's string form:
//
//	sh -c '<command> "$@"' <command> ...vargs
//
// The vargs become the shell's "$@" expansion and are passed on by the
// shell verbatim — no command sees them re-tokenized, so a varg containing
// shell metacharacters arrives as a single literal argument.
func shellCommand(command string, vargs []string) (*exec.Cmd, error) {
	cmd := exec.Command("sh",
		append([]string{"-c", command + ` "$@"`, command}, vargs...)...)
	return cmd, nil
}
