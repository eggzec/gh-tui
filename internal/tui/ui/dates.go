package ui

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

// AbsoluteLayout is how a date reads with the absolute date format.
const AbsoluteLayout = "2006-01-02 15:04 MST"

// ageWidth is the most cells an age takes, as in "11mo".
const ageWidth = 4

// Dates tells dates the way ui.date_format says: relative, as an age
// that Ago and AgoProse tell, absolute, in AbsoluteLayout, or in a Go time
// layout. The zero Dates tells them as ages, and dates in a layout in the
// local zone.
type Dates struct {
	// layout is the Go layout dates are told in, or "" when they are
	// told as ages.
	layout string
	// loc is the zone of dates in a layout; nil means time.Local.
	loc *time.Location
	// width is the most cells a date in the layout takes.
	width int
}

// NewDates returns the dates of format, a config.UI.DateFormat, which
// Validate has checked.
func NewDates(format string) Dates {
	switch format {
	case "", config.DateRelative:
		return Dates{}
	case config.DateAbsolute:
		format = AbsoluteLayout
	}
	d := Dates{layout: format}
	d.width = d.measure()
	return d
}

// In returns d with dates in a layout told in the zone loc.
func (d Dates) In(loc *time.Location) Dates {
	d.loc = loc
	if !d.Relative() {
		d.width = d.measure()
	}
	return d
}

// Relative reports whether dates are told as ages.
func (d Dates) Relative() bool {
	return d.layout == ""
}

// Short tells t in a row: as Ago does, such as "3d", when dates are
// relative, or in the layout.
func (d Dates) Short(t, now time.Time) string {
	if d.Relative() {
		return Ago(t, now)
	}
	return d.Date(t, "")
}

// Prose tells t in a sentence: as AgoProse does, such as "3d ago", as in
// "updated 3d ago", when dates are relative, or in the layout.
func (d Dates) Prose(t, now time.Time) string {
	if d.Relative() {
		return AgoProse(t, now)
	}
	return d.Date(t, "")
}

// Date tells t in the layout, or, when dates are relative, in relative,
// a layout for where an age is shown already or isn't enough, such as
// AbsoluteLayout.
func (d Dates) Date(t time.Time, relative string) string {
	layout := d.layout
	if d.Relative() {
		layout = relative
	}
	return t.In(d.zone()).Format(layout)
}

// Width returns the most cells that Short takes, so that a column of
// dates keeps room for the widest.
func (d Dates) Width() int {
	if d.Relative() {
		return ageWidth
	}
	return d.width
}

// measure returns the most cells a date in the layout takes. It measures
// a week of two-digit days in each month, so that every name of a month
// and a day, a month of two digits, and the zone in winter and in summer
// are measured, at a time with two digits in each field.
func (d Dates) measure() int {
	w := 0
	for month := time.January; month <= time.December; month++ {
		for day := 22; day <= 28; day++ {
			t := time.Date(2026, month, day, 23, 59, 59, 999999999, d.zone())
			w = max(w, ansi.StringWidth(t.Format(d.layout)))
		}
	}
	return w
}

func (d Dates) zone() *time.Location {
	if d.loc == nil {
		return time.Local
	}
	return d.loc
}
