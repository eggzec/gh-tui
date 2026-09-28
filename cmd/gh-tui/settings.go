package main

import (
	"github.com/eggzec/gh-tui/internal/config"
)

// session holds the config as the set command left it, for what reads it
// each time it opens, such as the Actions modal, rather than once. The
// app changes it and opens modals on its own goroutine, which alone uses
// it.
type session struct {
	cfg config.Config
}

// set takes cfg, which the set command changed, for tui.WithSettings.
func (s *session) set(cfg config.Config) {
	s.cfg = cfg
}
