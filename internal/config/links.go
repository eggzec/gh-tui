package config

import (
	"fmt"
	"slices"
)

// The values of links.
const (
	// LinksAuto writes links unless TERM is dumb or empty. On Windows TERM
	// is ignored and only Windows Terminal (WT_SESSION) counts.
	LinksAuto = "auto"
	// LinksOn always writes links.
	LinksOn = "on"
	// LinksOff writes plain text only.
	LinksOff = "off"
)

// LinksEnabled reports whether links are written for the setting, on the
// given operating system and environment. On Windows, auto trusts only
// Windows Terminal, which sets WT_SESSION, and ignores TERM, which other
// Windows terminals may set without drawing links.
func LinksEnabled(setting, goos string, getenv func(string) string) bool {
	switch setting {
	case LinksOn:
		return true
	case LinksAuto:
		if goos == "windows" {
			return getenv("WT_SESSION") != ""
		}
		term := getenv("TERM")
		return term != "" && term != "dumb"
	}
	return false
}

func validateLinks(v string) error {
	if !slices.Contains([]string{LinksAuto, LinksOn, LinksOff}, v) {
		return fmt.Errorf("links: must be %s, %s or %s, got %q", LinksAuto, LinksOn, LinksOff, v)
	}
	return nil
}
