package logview

import (
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Kind is what a line of a log is, as the runner marked it.
type Kind uint8

const (
	// Plain is output of a step.
	Plain Kind = iota
	// Group starts a group that folds, "##[group]" in GitHub Actions. Its
	// Text is the title of the group.
	Group
	// EndGroup ends the innermost group, "##[endgroup]". It isn't shown.
	EndGroup
	// Error is an error annotation, "##[error]".
	Error
	// Warning is a warning annotation, "##[warning]".
	Warning
	// Notice is a notice annotation, "##[notice]".
	Notice
	// Command is a command the runner ran, "[command]".
	Command
	// Debug is a debug message, "##[debug]".
	Debug
)

// kindSection is the kind of the title row of a section, which has no line
// of its own.
const kindSection Kind = math.MaxUint8

// Line is one line of a log, without the runner's markers.
type Line struct {
	// Time is when the line was written, or zero when unknown.
	Time time.Time
	// Text is the text of the line. It may hold SGR escape sequences for
	// colors and bold, which are kept; every other escape sequence and
	// control character is removed.
	Text string
	// Kind is what the line is.
	Kind Kind
}

// Section is a run of lines that folds under a title, such as a step of a
// job.
type Section struct {
	// Title is the title, such as the name of the step.
	Title string
	// Start and End are the lines of the section: Start is the index of the
	// first, and End the index after the last. A section ends where the
	// next one starts. The last section that reaches the end of the lines
	// also takes the lines appended later.
	Start, End int
	// Failed marks a step that failed.
	Failed bool
	// Duration is how long the step took, or zero to show none.
	Duration time.Duration
}

// row is a row of the view before folding and wrapping: a line of the log
// or the title of a section.
type row struct {
	// text is the sanitized text, without escape sequences, and marks the
	// SGR sequences that color it.
	text  string
	marks []mark
	time  time.Time
	// src is the index of the line in the log. The title of a section has
	// the index of the section's first line.
	src int
	// fold is the fold the row starts, or -1. parent is the innermost fold
	// the row is in, or -1, and sec the section, or -1.
	fold, parent, sec int
	depth             int
	kind              Kind
}

// fold is a section or a group: a header row and the rows under it, up to
// end.
type fold struct {
	head, end int
	parent    int
	open      bool
	// sec is the index of the section, or -1 for a group.
	sec int
}

// section is a section as the view keeps it.
type section struct {
	start, end int
	failed     bool
	duration   time.Duration
	title      string
	// began is the time of its first line with a time.
	began time.Time
	fold  int
}

// content is the log and how it folds. Rows, folds and the lists of rows
// only grow, by Append; a toggle copies the folds and vis, so copies of a
// model don't see each other's folding.
type content struct {
	rows  []row
	folds []fold
	secs  []section
	// vis holds the rows shown, in order: those outside collapsed folds.
	vis []int
	// errs and warns hold the rows of errors and warnings.
	errs, warns []int
	// n is the number of lines of the log, and began the time of its first
	// line with a time.
	n     int
	began time.Time

	// open holds the folds still open at the end of the log, outermost
	// first, which appended lines go into. nextSec is the section to start
	// next, and inSec the one lines go into, or -1.
	open           []int
	nextSec, inSec int
}

// SetLines shows a log from its first line, with the lines of sections
// folded under their titles. Sections start expanded and groups collapsed;
// with [WithFocusFailed] only the failed section is expanded. Lines of the
// kind Group and EndGroup fold the lines between them.
func (m *Model) SetLines(lines []Line, sections []Section) {
	m.reset(stateReady, nil)
	m.secs = normalize(sections, len(lines))
	m.rows = make([]row, 0, len(lines)+len(m.secs))
	for _, l := range lines {
		m.add(l)
	}
	m.reach(m.n)
	m.settle()
	m.rebuild()
	m.enableKeys()
	m.clamp()
	if m.focusFailed {
		m.FocusFailed()
	}
}

// Append adds lines at the end of the log, for a job that is still
// running. They go into the section and groups still open at the end. While
// the view follows, a cursor on the last line stays there.
//
// Append adds to the model's slices in place, so a copy of the model made
// before must not append too.
func (m *Model) Append(lines ...Line) {
	if len(lines) == 0 {
		return
	}
	if m.state != stateReady {
		m.SetLines(nil, nil)
	}
	pinned := m.follow && (len(m.vis) == 0 || m.cur == len(m.vis)-1)
	from := len(m.rows)
	for _, l := range lines {
		m.add(l)
	}
	m.reach(m.n)
	m.settle()
	for r := from; r < len(m.rows); r++ {
		if m.shown(r) {
			m.vis = append(m.vis, r)
		}
	}
	m.searchFrom(from)
	m.enableKeys()
	m.live = true
	if pinned {
		m.end()
		return
	}
	m.clamp()
}

// SetLoading shows a spinner while the log is on its way. The returned
// command starts the spinner, which stops once the lines or an error are
// set.
func (m *Model) SetLoading() tea.Cmd {
	m.reset(stateLoading, nil)
	return m.spin.Tick
}

// SetError shows that the log failed to load with err.
func (m *Model) SetError(err error) {
	m.reset(stateFailed, err)
}

// Lines returns the number of lines of the log.
func (m Model) Lines() int { return m.n }

// Errors returns the number of error lines.
func (m Model) Errors() int { return len(m.errs) }

// Warnings returns the number of warning lines.
func (m Model) Warnings() int { return len(m.warns) }

// reset forgets the log.
func (m *Model) reset(s state, err error) {
	m.state, m.err = s, err
	m.errText, m.errHint = "", ""
	if s == stateFailed {
		m.errText, m.errHint = m.errorWords()
	}
	m.content = content{inSec: -1}
	m.cur, m.top, m.row, m.left = 0, 0, 0, 0
	m.live = false
	m.jumped, m.at = Plain, 0
	m.clearSearch()
}

// normalize puts sections in order and keeps them within n lines and apart.
// A section that reaches the end stays open for appended lines.
func normalize(sections []Section, n int) []section {
	sorted := slices.Clone(sections)
	slices.SortStableFunc(sorted, func(a, b Section) int { return a.Start - b.Start })
	out := make([]section, 0, len(sorted))
	prev := 0
	for _, s := range sorted {
		start := min(max(s.Start, prev), n)
		end := max(s.End, start)
		if end >= n {
			end = math.MaxInt
		}
		out = append(out, section{
			start: start, end: end, failed: s.Failed, duration: s.Duration, title: cleanTitle(s.Title),
		})
		prev = start
	}
	return out
}

// cleanTitle returns a title on one line, without escape sequences.
func cleanTitle(s string) string {
	t, _ := sanitize(strings.Join(strings.Fields(ansi.Strip(s)), " "), 1)
	return t
}

// add adds line l of the log.
func (m *Model) add(l Line) {
	i := m.n
	m.n++
	m.reach(i)
	if m.began.IsZero() {
		m.began = l.Time
	}
	if m.inSec >= 0 && m.secs[m.inSec].began.IsZero() {
		m.secs[m.inSec].began = l.Time
	}
	switch l.Kind {
	case EndGroup:
		// A group never ends its section.
		if k := len(m.open) - 1; k >= 0 && m.folds[m.open[k]].sec < 0 {
			m.closeFold()
		}
		return
	case Group:
		m.openFold(-1)
	case Error:
		m.errs = append(m.errs, len(m.rows))
	case Warning:
		m.warns = append(m.warns, len(m.rows))
	default:
		// Other lines are only shown.
	}
	text, marks := sanitize(l.Text, m.tabWidth)
	m.addRow(row{text: text, marks: marks, time: l.Time, src: i, kind: l.Kind})
}

// addRow adds r under the folds open at the end. A group's header row is
// added after its fold is opened, so it heads it.
func (m *Model) addRow(r row) {
	r.fold, r.parent, r.sec = -1, -1, m.inSec
	k := len(m.open) - 1
	if k >= 0 && m.folds[m.open[k]].head == len(m.rows) {
		r.fold = m.open[k]
		k--
	}
	if k >= 0 {
		r.parent = m.open[k]
		r.depth = len(m.open[:k+1])
	}
	m.rows = append(m.rows, r)
}

// openFold opens a fold whose header is the next row: section sec, or a
// group for -1.
func (m *Model) openFold(sec int) {
	parent := -1
	if k := len(m.open) - 1; k >= 0 {
		parent = m.open[k]
	}
	m.folds = append(m.folds, fold{head: len(m.rows), parent: parent, open: sec >= 0, sec: sec})
	m.open = append(m.open, len(m.folds)-1)
}

// closeFold closes the innermost open fold.
func (m *Model) closeFold() {
	k := len(m.open) - 1
	m.folds[m.open[k]].end = len(m.rows)
	m.open = m.open[:k]
}

// reach starts and ends the sections up to line i.
func (m *Model) reach(i int) {
	for {
		switch {
		case m.inSec >= 0 && i >= m.secs[m.inSec].end:
			m.endSection()
		case m.nextSec < len(m.secs) && m.secs[m.nextSec].start <= i:
			m.endSection()
			m.startSection()
		default:
			return
		}
	}
}

func (m *Model) startSection() {
	s := m.nextSec
	m.nextSec++
	m.inSec = s
	m.openFold(s)
	m.secs[s].fold = len(m.folds) - 1
	m.addRow(row{text: m.secs[s].title, src: m.secs[s].start, kind: kindSection})
}

// endSection closes the section lines go into, and the groups left open in
// it.
func (m *Model) endSection() {
	if m.inSec < 0 {
		return
	}
	for len(m.open) > 0 {
		f := m.open[len(m.open)-1]
		m.closeFold()
		if m.folds[f].sec >= 0 {
			break
		}
	}
	m.inSec = -1
}

// settle ends the folds still open at the end of the rows.
func (m *Model) settle() {
	for _, f := range m.open {
		m.folds[f].end = len(m.rows)
	}
}

// shown reports whether row r is outside collapsed folds.
func (m *Model) shown(r int) bool {
	for f := m.rows[r].parent; f >= 0; f = m.folds[f].parent {
		if !m.folds[f].open {
			return false
		}
	}
	return true
}
