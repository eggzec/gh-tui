package config

import (
	"fmt"
	"slices"
)

// The values of links.
const (
	// LinksAuto writes links unless the terminal is dumb.
	LinksAuto = "auto"
	// LinksOn always writes links.
	LinksOn = "on"
	// LinksOff writes plain text only.
	LinksOff = "off"
)

func validateLinks(v string) error {
	if !slices.Contains([]string{LinksAuto, LinksOn, LinksOff}, v) {
		return fmt.Errorf("links: must be %s, %s or %s, got %q", LinksAuto, LinksOn, LinksOff, v)
	}
	return nil
}
