package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// UI configures what every screen draws with.
type UI struct {
	// Icons is the set of glyphs that mark repositories, languages and the
	// states of issues and pull requests: IconsNerd, which needs a Nerd
	// Font and marks the types of files too, IconsUnicode or IconsASCII.
	Icons string `yaml:"icons" scope:"global"`
	Toast Toast  `yaml:"toast"`
	// DateFormat is how every date reads, in the lists, the modals and
	// the history: DateRelative, as an age, such as "3d" in a row and
	// "3d ago" in a sentence, DateAbsolute ("2006-01-02 15:04 MST"), or
	// a Go time layout such as "2006-01-02 15:04".
	DateFormat string `yaml:"date_format"`
	// Maximized lists the modals that open filling the screen, each by the
	// name of its key context, such as "history". The maximize key
	// toggles the one that is open, whichever way it opened.
	Maximized []string `yaml:"maximized"`
}

// Toast is how long a toast stays before it goes on its own. An error
// toast has no time: it stays until it is dismissed.
type Toast struct {
	// Info is how long an info, success or warning toast stays.
	Info time.Duration `yaml:"info"`
}

// minToast is the shortest a toast stays, so that it can be read.
const minToast = time.Second

// Date formats of [UI.DateFormat] besides a Go layout.
const (
	DateRelative = "relative"
	DateAbsolute = "absolute"
)

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
	errs = append(errs, validateDateFormat(u.DateFormat), u.validateMaximized())
	return errors.Join(errs...)
}

// validateMaximized checks that Maximized names modals that can open
// first: the contexts of keys that are modals, but for the steps that show
// inside another.
func (u UI) validateMaximized() error {
	var modals []string
	for _, c := range contexts {
		if c.Modal && c.Within == "" {
			modals = append(modals, c.Name)
		}
	}
	var errs []error
	for _, name := range u.Maximized {
		if !slices.Contains(modals, name) {
			errs = append(errs, fmt.Errorf("ui.maximized: unknown modal %q: must be one of %s", name, strings.Join(modals, ", ")))
		}
	}
	return errors.Join(errs...)
}

// maxDateWidth bounds the cells a date takes, since the rows that always
// show one, such as the notifications', have no room for a longer one.
const maxDateWidth = 40

func validateDateFormat(f string) error {
	switch {
	case !validDateFormat(f):
		return fmt.Errorf(`ui.date_format: must be relative, absolute or a Go time layout such as "2006-01-02 15:04", got %q`, f)
	case strings.ContainsFunc(f, unicode.IsControl):
		return fmt.Errorf("ui.date_format: must be one line with no control characters, got %q", f)
	}
	if f == DateRelative || f == DateAbsolute {
		return nil
	}
	// A Wednesday in September, the longest names of a day and a month.
	wide := time.Date(2026, time.September, 30, 23, 59, 59, 999999999, time.UTC).Format(f)
	if w := ansi.StringWidth(wide); w > maxDateWidth {
		return fmt.Errorf("ui.date_format: must make dates of at most %d cells, but makes %q, %d cells", maxDateWidth, wide, w)
	}
	return nil
}

// validDateFormat reports whether f is a format name, or a layout with at
// least one element of the reference time: any other text formats to
// itself, whatever the time. The time must not be the reference time, which
// formats every layout to itself.
func validDateFormat(f string) bool {
	if f == DateRelative || f == DateAbsolute {
		return true
	}
	t := time.Date(2001, time.March, 4, 7, 8, 9, 0, time.UTC)
	return strings.TrimSpace(f) != "" && t.Format(f) != f
}
