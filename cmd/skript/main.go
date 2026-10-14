// Command skript runs skripts by target — an alias, a skript root
// directory, or a skript file — resolved per notes/config and executed by
// the matching `.skriptrc` executor or config association. See the README
// for the exact resolution and executor semantics.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/volkbyte/skript/internal/cli"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "skript: %s\n", err)
		os.Exit(1)
	}

	app := &cli.App{
		ConfigPath: filepath.Join(home, ".volkbyte", "skript", "config.toml"),
		HomeDir:    home,
	}
	os.Exit(app.Run(os.Args[1:]))
}
