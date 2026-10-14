// Command echoargs is a test helper: it records how a child process
// received its arguments, so tests can assert exact argv behavior of the
// executors (literal pass-through, quoting, exit codes, environment).
//
// Its arguments split into flags first and an echo section after:
//
//	-out=<file>  write one line per remaining argument to <file> instead
//	             of stdout
//	-exit=<n>    exit with code <n> (default 0)
//	-env=<name>  print "<name>=$<value>", or "<name>=(unset)"
//
// Everything after the flags is echoed verbatim, one argument per line.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	var (
		outPath string
		exit    = 0
		envs    []string
		rest    []string
	)

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "-out="):
			outPath = args[i][len("-out="):]
		case strings.HasPrefix(args[i], "-exit="):
			code, err := strconv.Atoi(args[i][len("-exit="):])
			if err != nil {
				fmt.Fprintf(os.Stderr, "echoargs: invalid -exit value %q\n", args[i][len("-exit="):])
				os.Exit(2)
			}
			exit = code
		case strings.HasPrefix(args[i], "-env="):
			envs = append(envs, args[i][len("-env="):])
		default:
			rest = append(rest, args[i])
		}
	}

	var b strings.Builder
	for _, name := range envs {
		value, ok := os.LookupEnv(name)
		if !ok {
			b.WriteString(name + "=(unset)\n")
			continue
		}
		b.WriteString(name + "=" + value + "\n")
	}
	for _, arg := range rest {
		b.WriteString(arg + "\n")
	}

	if outPath == "" {
		fmt.Print(b.String())
	} else {
		if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "echoargs: write %s: %v\n", outPath, err)
			os.Exit(2)
		}
	}
	os.Exit(exit)
}
