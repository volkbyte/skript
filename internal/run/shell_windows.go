//go:build windows

package run

import (
	"os/exec"
	"strings"
	"syscall"
)

// shellCommand builds the invocation for a command executor's string form:
//
//	cmd /C call <command> <varg1> <varg2> ...
//
// The command line is composed directly (SysProcAttr.CmdLine) so the command
// text stays unquoted and each varg is quoted with the CRT quoting rules of
// crtQuote.
//
// A `call` prefix guards the body: cmd's /C processing strips the first and
// last quote of the remainder whenever its first character is a quote (and
// leading spaces are trimmed before that check, so spacing is no guard).
// That stripping would otherwise eat the opening quote of a quoted
// executable path — which user command strings legitimately start with —
// and, by shifting the quote parity, corrupt every following varg (observed:
// `say "hi" now` split in two). With `call`, the body's first character is
// not a quote, no stripping happens, and the CRT-quoted vargs keep the
// metacharacters inert. `call` both runs executables and relays batch
// files, so it is behavior-neutral for the command string.
//
// Documented hazard: %VAR% in the vargs is expanded by cmd before the child
// program sees it; because the vargs end up on a cmd /C line, that
// expansion cannot be suppressed from a cmd /C line. Prefer the command
// array form (or the exec executor) when vargs may contain percent signs.
func shellCommand(command string, vargs []string) (*exec.Cmd, error) {
	var b strings.Builder
	b.WriteString(`cmd /C call `)
	b.WriteString(command)
	for _, arg := range vargs {
		b.WriteByte(' ')
		b.WriteString(crtQuote(arg))
	}

	cmd := exec.Command("cmd", "/C")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: b.String()}
	return cmd, nil
}

// crtQuote quotes an argument with the CRT command-line rules:
//
//   - wrapped in double quotes when it contains a space, a tab, a quote, or
//     any cmd metacharacter (& | < > ^ % ( ) = ; ,): outside quotes those
//     would be interpreted by cmd, so wrapping is what keeps such a varg
//     inert; over-wrapping is harmless under the CRT rules;
//   - an empty argument is wrapped as "" so it survives the command line;
//   - inner quotes are doubled;
//   - backslashes bordering a quote — including the closing one — are
//     doubled, so a trailing backslash stays a literal backslash.
//
// For the interaction of doubled inner quotes with argv parsers (some
// modern tools split the line with CommandLineToArgvW rules and may not
// reproduce an inner literal quote), see the executor docs in
// notes/config/skriptrc.ts and the README. The command array form has no
// such caveat.
func crtQuote(arg string) string {
	if arg != "" && !needsWrapping(arg) {
		return arg
	}

	var b strings.Builder
	b.Grow(len(arg) + 2)
	b.WriteByte('"')

	slashes := 0
	for i := 0; i < len(arg); i++ {
		switch c := arg[i]; c {
		case '\\':
			slashes++
		case '"':
			// Backslashes bordering the doubled quote are doubled too.
			for j := 0; j < 2*slashes; j++ {
				b.WriteByte('\\')
			}
			slashes = 0
			b.WriteString(`""`)
		default:
			// Backslashes not followed by a quote stay single.
			for j := 0; j < slashes; j++ {
				b.WriteByte('\\')
			}
			slashes = 0
			b.WriteByte(c)
		}
	}
	// Backslashes bordering the closing quote are doubled.
	for j := 0; j < 2*slashes; j++ {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}

// cmdMetachars are characters cmd interprets on an unquoted command line.
const cmdMetachars = "&|<>^%()=;,"

func needsWrapping(arg string) bool {
	return strings.ContainsAny(arg, " \t\""+cmdMetachars)
}
