// Package calendar provides a contribution heatmap like the one on GitHub
// profiles.
//
// A calendar shows a year of daily counts as weeks in columns and weekdays
// in rows, each day one cell colored by its level, with the months above,
// the weekdays on the left and a legend below. When the width cannot fit
// every week, it shows the most recent weeks that fit. The calendar does no
// I/O: the parent fetches the days and sets them with [Model.SetWeeks].
//
// While focused, the arrow keys move a cursor over the days, a status line
// tells the count of the day under it, and a [SelectMsg] names it.
package calendar

import (
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Day is one cell of the calendar.
type Day struct {
	// Date is the day. Its weekday picks the row.
	Date time.Time
	// Count is the number of contributions on the day.
	Count int
	// Level is the intensity of the cell, from 0 for none to Levels-1 for
	// the most. Levels out of range are clamped.
	Level int
}

var lastID atomic.Int64

func nextID() int {
	return int(lastID.Add(1))
}

// Model is a contribution calendar. Create one with [New].
type Model struct {
	settings

	id   int
	grid []week
	sum  int
	// cw and cd are the week and weekday of the cursor.
	cw, cd int
	// start is the first week shown and cols the number of weeks shown.
	start, cols int
	weekdays    bool

	// lines is the view, rendered when the data, size, styles, focus or
	// cursor change, so View only joins it.
	lines []string

	// Rendered in SetStyles and SetGlyph.
	frags       [Levels]string
	cursorFrags [Levels]string
	dayLabels   [7]string
	legend      string
	legendW     int
}

// New returns a calendar. Days are set with [WithWeeks] or
// [Model.SetWeeks].
func New(opts ...Option) Model {
	m := Model{settings: defaultSettings(), id: nextID()}
	for _, opt := range opts {
		opt(&m.settings)
	}
	weeks := m.weeks
	// The model keeps its own grid, not the caller's slices.
	m.weeks = nil
	m.prerender()
	m.SetWeeks(weeks)
	return m
}

// Init does nothing: the calendar needs no commands to start.
func (m Model) Init() tea.Cmd {
	return nil
}

// ID returns the unique ID of the calendar.
func (m Model) ID() int {
	return m.id
}

// SetWeeks sets the days to show, one slice per week from the oldest, each
// from Sunday to Saturday. The first and last weeks may be partial, and
// empty weeks are skipped. The cursor stays on the same date if it is still
// there and goes to the last day otherwise, and the calendar shows the most
// recent weeks again.
func (m *Model) SetWeeks(weeks [][]Day) {
	prev, had := m.Selected()
	grid := make([]week, 0, len(weeks))
	for _, w := range weeks {
		var wk week
		for _, d := range w {
			d.Level = min(max(d.Level, 0), Levels-1)
			wk[d.Date.Weekday()] = slot{day: d, ok: true}
		}
		if len(w) > 0 {
			grid = append(grid, wk)
		}
	}
	m.grid = grid
	m.sum = 0
	for i := range grid {
		for _, s := range grid[i] {
			m.sum += s.day.Count
		}
	}
	m.cw, m.cd = 0, 0
	if i, ok := m.step(len(grid)*7, -1); ok {
		m.cw, m.cd = i/7, i%7
	}
	if had {
		m.Select(prev.Date)
	}
	m.relayout()
}

// Weeks returns the days, one slice per week.
func (m Model) Weeks() [][]Day {
	weeks := make([][]Day, len(m.grid))
	for i := range m.grid {
		for _, s := range m.grid[i] {
			if s.ok {
				weeks[i] = append(weeks[i], s.day)
			}
		}
	}
	return weeks
}

// SetTotal sets the total shown above the grid, such as the one the API
// reports. A negative total shows the sum of the counts, the default.
func (m *Model) SetTotal(total int) {
	m.total = max(total, -1)
	m.render()
}

// Total returns the total shown above the grid.
func (m Model) Total() int {
	if m.total >= 0 {
		return m.total
	}
	return m.sum
}

// Selected returns the day under the cursor, or false if there are no days.
func (m Model) Selected() (Day, bool) {
	if len(m.grid) == 0 {
		return Day{}, false
	}
	return m.grid[m.cw][m.cd].day, true
}

// Select moves the cursor to the day of date and reports whether there is
// one. It sends no [SelectMsg].
func (m *Model) Select(date time.Time) bool {
	y, mo, d := date.Date()
	for w := range m.grid {
		for wd, s := range m.grid[w] {
			if !s.ok {
				continue
			}
			if sy, smo, sd := s.day.Date.Date(); sy == y && smo == mo && sd == d {
				m.cw, m.cd = w, wd
				m.follow()
				m.render()
				return true
			}
		}
	}
	return false
}

// SetSize sets the width and height of the calendar, and shows the most
// recent weeks that fit.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.relayout()
}

// Width returns the width of the calendar.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the calendar.
func (m Model) Height() int {
	return m.height
}

// Focus makes the calendar react to keys and show its cursor.
func (m *Model) Focus() {
	m.focused = true
	m.render()
}

// Blur makes the calendar ignore keys and hide its cursor.
func (m *Model) Blur() {
	m.focused = false
	m.render()
}

// Focused reports whether the calendar reacts to keys.
func (m Model) Focused() bool {
	return m.focused
}

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keyMap = k
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap {
	return m.keyMap
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.prerender()
	m.render()
}

// Styles returns the styles.
func (m Model) Styles() Styles {
	return m.styles
}

// SetGlyph sets the glyph of a day. It must be one cell wide, such as "▪"
// or "#"; other glyphs are ignored.
func (m *Model) SetGlyph(glyph string) {
	if ansi.StringWidth(glyph) != 1 {
		return
	}
	m.glyph = glyph
	m.prerender()
	m.render()
}

// Glyph returns the glyph of a day.
func (m Model) Glyph() string {
	return m.glyph
}

// SetEmptyText sets the text shown when there are no days.
func (m *Model) SetEmptyText(text string) {
	m.emptyText = text
	m.render()
}

// relayout fits the weeks to the width, shows the most recent ones and
// renders the view.
func (m *Model) relayout() {
	m.weekdays = showWeekdays(m.width)
	room := m.width
	if m.weekdays {
		room -= gutterW
	}
	m.cols = min(fitWeeks(room), len(m.grid))
	m.start = len(m.grid) - m.cols
	m.follow()
	m.render()
}

// follow scrolls the weeks shown to the cursor and reports whether they
// moved.
func (m *Model) follow() bool {
	old := m.start
	if m.cw < m.start {
		m.start = m.cw
	}
	if m.cw >= m.start+m.cols {
		m.start = m.cw - m.cols + 1
	}
	m.start = max(min(m.start, len(m.grid)-m.cols), 0)
	return m.start != old
}

// prerender renders the fragments that depend on the styles and glyph.
func (m *Model) prerender() {
	s := m.styles
	if ansi.StringWidth(m.glyph) != 1 {
		m.glyph = DefaultGlyph
	}
	var legend strings.Builder
	legend.WriteString(s.Legend.Render("Less"))
	for l := range Levels {
		m.frags[l] = s.Levels[l].Render(m.glyph)
		m.cursorFrags[l] = s.Levels[l].Inherit(s.Cursor).Render(m.glyph)
		legend.WriteByte(' ')
		legend.WriteString(m.frags[l])
	}
	legend.WriteByte(' ')
	legend.WriteString(s.Legend.Render("More"))
	m.legend = legend.String()
	m.legendW = ansi.StringWidth(m.legend)
	for d := range m.dayLabels {
		m.dayLabels[d] = strings.Repeat(" ", gutterW)
		if d%2 == 1 {
			name := time.Weekday(d).String()[:3]
			m.dayLabels[d] = s.Weekday.Render(name) + strings.Repeat(" ", gutterW-len(name))
		}
	}
}
