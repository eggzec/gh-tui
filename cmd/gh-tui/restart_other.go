//go:build !unix

package main

import "errors"

// execProgram can't replace the process where there is no exec.
func execProgram(string, []string, []string) error {
	return errors.New("restarting is not supported on this system; start gh-tui again with --hostname and --user")
}
