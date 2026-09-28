package config

import "fmt"

// Commands configures the command line.
type Commands struct {
	// History is how many of the lines typed on the command line are kept
	// for each account, across sessions.
	History int `yaml:"history"`
}

// maxCommandHistory bounds the lines kept, which are read and written
// whole after each command.
const maxCommandHistory = 10000

func (c Commands) validate() error {
	if c.History < 1 || c.History > maxCommandHistory {
		return fmt.Errorf("commands.history: must be between 1 and %d, got %d", maxCommandHistory, c.History)
	}
	return nil
}
