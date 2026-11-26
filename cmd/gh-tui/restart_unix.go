//go:build unix

package main

import "syscall"

// execProgram replaces the process with the program at path.
func execProgram(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
