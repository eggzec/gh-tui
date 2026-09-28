package config

import (
	"fmt"
	"strings"
	"unicode"
)

// validateEditor reports an editor command that names no program, or
// that holds a control character, such as a newline, which no command
// line can.
func validateEditor(cmd string) error {
	if cmd != "" && (strings.TrimSpace(cmd) == "" || strings.ContainsFunc(cmd, unicode.IsControl)) {
		return fmt.Errorf("editor: must be a program and its arguments on one line, such as \"vim\" or \"code --wait\", got %q", cmd)
	}
	return nil
}
