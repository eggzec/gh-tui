package config

import (
	"errors"
	"fmt"
	"slices"
)

// Images configures the images drawn inline with kitty's graphics
// protocol, which only some terminals show as the app needs.
type Images struct {
	// Enabled is ImagesAuto, which draws images when the terminal is
	// known to show them, ImagesOn, which draws them without asking the
	// terminal for its kitty graphics but only on a terminal the app
	// knows shows them, or ImagesOff, which never draws or asks.
	Enabled string `yaml:"enabled"`
	// MaxRows is the tallest an inline image may be, in rows.
	MaxRows int `yaml:"max_rows"`
	// Avatars draws the avatars of people and the icons of repositories
	// and organizations.
	Avatars bool `yaml:"avatars"`
	// Animate plays animated GIFs where the terminal can.
	Animate bool `yaml:"animate"`
}

// The values of images.enabled.
const (
	ImagesAuto = "auto"
	ImagesOn   = "on"
	ImagesOff  = "off"
)

func (i Images) validate() error {
	var errs []error
	if !slices.Contains([]string{ImagesAuto, ImagesOn, ImagesOff}, i.Enabled) {
		errs = append(errs, fmt.Errorf("images.enabled: must be %s, %s or %s, got %q", ImagesAuto, ImagesOn, ImagesOff, i.Enabled))
	}
	if i.MaxRows < 1 {
		errs = append(errs, fmt.Errorf("images.max_rows: must be at least 1, got %d", i.MaxRows))
	}
	return errors.Join(errs...)
}
