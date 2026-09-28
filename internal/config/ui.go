package config

import (
	"fmt"
	"slices"
)

// UI configures what every screen draws with.
type UI struct {
	// Icons is the set of glyphs that mark repositories, languages and the
	// states of issues and pull requests: IconsNerd, which needs a Nerd
	// Font and marks the types of files too, IconsUnicode or IconsASCII.
	Icons string `yaml:"icons"`
}

// Icon sets.
const (
	IconsNerd    = "nerd"
	IconsUnicode = "unicode"
	IconsASCII   = "ascii"
)

func (u UI) validate() error {
	if !slices.Contains([]string{IconsNerd, IconsUnicode, IconsASCII}, u.Icons) {
		return fmt.Errorf("ui.icons: must be %s, %s or %s, got %q", IconsNerd, IconsUnicode, IconsASCII, u.Icons)
	}
	return nil
}
