package config

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// UI configures what every screen draws with.
type UI struct {
	// Icons is the set of glyphs that mark repositories, languages and the
	// states of issues and pull requests: IconsNerd, which needs a Nerd
	// Font and marks the types of files too, IconsUnicode or IconsASCII.
	Icons string `yaml:"icons" scope:"global"`
	Toast Toast  `yaml:"toast"`
}

// Toast is how long a toast stays before it goes on its own.
type Toast struct {
	// Info is how long an info, success or warning toast stays.
	Info time.Duration `yaml:"info"`
	// Error is how long an error toast stays.
	Error time.Duration `yaml:"error"`
}

// minToast is the shortest a toast stays, so that it can be read.
const minToast = time.Second

// Icon sets.
const (
	IconsNerd    = "nerd"
	IconsUnicode = "unicode"
	IconsASCII   = "ascii"
)

func (u UI) validate() error {
	var errs []error
	if !slices.Contains([]string{IconsNerd, IconsUnicode, IconsASCII}, u.Icons) {
		errs = append(errs, fmt.Errorf("ui.icons: must be %s, %s or %s, got %q", IconsNerd, IconsUnicode, IconsASCII, u.Icons))
	}
	if u.Toast.Info < minToast {
		errs = append(errs, fmt.Errorf("ui.toast.info: must be at least %v, got %v", minToast, u.Toast.Info))
	}
	if u.Toast.Error < minToast {
		errs = append(errs, fmt.Errorf("ui.toast.error: must be at least %v, got %v", minToast, u.Toast.Error))
	}
	return errors.Join(errs...)
}
