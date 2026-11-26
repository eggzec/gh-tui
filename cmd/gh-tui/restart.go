package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/eggzec/gh-tui/internal/target"
)

// restartArgs returns the arguments that start the program again as t: those
// it started with, without a --hostname or --user it was given, then t's
// own. Both spellings of a flag are dropped, with its separate value.
func restartArgs(args []string, t target.Target) []string {
	out := make([]string, 0, len(args)+4)
	for i := 0; i < len(args); i++ {
		a := strings.TrimPrefix(strings.TrimPrefix(args[i], "-"), "-")
		switch name, _, attached := strings.Cut(a, "="); {
		case name != "hostname" && name != "user":
			out = append(out, args[i])
		case !attached:
			i++ // its value
		}
	}
	return append(out, "--hostname", t.Host, "--user", t.User)
}

// restart replaces the process with the same program started as t. It
// returns only when that fails.
func restart(t target.Target) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("restart as %s: %w", t, err)
	}
	argv := append([]string{exe}, restartArgs(os.Args[1:], t)...)
	if err := execProgram(exe, argv, os.Environ()); err != nil {
		if errors.Is(err, fs.ErrNotExist) || strings.HasSuffix(exe, " (deleted)") {
			return fmt.Errorf("restart as %s: the gh-tui binary is gone, so start gh-tui again", t)
		}
		return fmt.Errorf("restart as %s: %w", t, err)
	}
	return nil
}
