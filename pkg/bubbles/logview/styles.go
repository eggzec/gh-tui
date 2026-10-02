package logview

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Glyphs drawn in the gutter and before folds.
const (
	cursorGlyph   = "▌"
	openGlyph     = "▾ "
	closedGlyph   = "▸ "
	warningGlyph  = "!"
	noticeGlyph   = "i"
	ellipsisGlyph = "…"
)

// Styles holds the styles of a log view. The styles of lines go under the
// log's own colors, which win where they are set.
type Styles struct {
	// Text styles plain lines.
	Text lipgloss.Style
	// Command, Debug, ErrorLine, WarningLine and NoticeLine style the lines
	// of those kinds.
	Command     lipgloss.Style
	Debug       lipgloss.Style
	ErrorLine   lipgloss.Style
	WarningLine lipgloss.Style
	NoticeLine  lipgloss.Style
	// Group styles the titles of groups, Section the titles of sections,
	// and FailedSection those of failed sections. Duration styles how long
	// a section took, at the right.
	Group         lipgloss.Style
	Section       lipgloss.Style
	FailedSection lipgloss.Style
	Duration      lipgloss.Style
	// Marker styles the ▸ and ▾ before sections and groups.
	Marker lipgloss.Style
	// ErrorMark, WarningMark and NoticeMark style the marks in the gutter
	// of those lines, and of failed sections.
	ErrorMark   lipgloss.Style
	WarningMark lipgloss.Style
	NoticeMark  lipgloss.Style
	// Cursor marks the line under the cursor while the view is focused,
	// and BlurredCursor while it is blurred.
	Cursor        lipgloss.Style
	BlurredCursor lipgloss.Style
	// LineNumber and Time style the gutter.
	LineNumber lipgloss.Style
	Time       lipgloss.Style
	// Match styles the matches of a search, and CurrentMatch the one the
	// last jump went to. They replace the colors of the match.
	Match        lipgloss.Style
	CurrentMatch lipgloss.Style
	// Title styles the title in the status line, Status the rest of it, and
	// NoMatches a search that found nothing.
	Title     lipgloss.Style
	Status    lipgloss.Style
	NoMatches lipgloss.Style
	// Message styles the placeholder shown instead of a log, Spinner the
	// spinner while loading, and LoadError a log that failed to load.
	Message   lipgloss.Style
	Spinner   lipgloss.Style
	LoadError lipgloss.Style
	// ErrorGlyph starts the text of a log that failed to load, and marks
	// error lines in the gutter, beside the "!" of warnings and the "i" of
	// notices, cut or padded to the gutter's one cell. The default is "✗".
	ErrorGlyph string
	// ErrorSeparator goes between the text of an error and its hint, and
	// ErrorEllipsis ends the text where it is cut. The defaults are " · "
	// and "…".
	ErrorSeparator, ErrorEllipsis string
	// Prompt styles the "/" before the search input, and InputCursor its
	// cursor, with its foreground.
	Prompt      lipgloss.Style
	InputCursor lipgloss.Style
}

// DefaultStyles returns calm styles for a light or dark terminal.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))
	warnColor := ld(lipgloss.Color("#9a6700"), lipgloss.Color("#e0af68"))
	debug := ld(lipgloss.Color("#7c4dbd"), lipgloss.Color("#bb9af7"))
	match := ld(lipgloss.Color("#f6e7a8"), lipgloss.Color("#4a4430"))
	current := ld(lipgloss.Color("#f2c14e"), lipgloss.Color("#e0af68"))
	onCurrent := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#1a1b26"))

	return Styles{
		Text:           lipgloss.NewStyle().Foreground(text),
		Command:        lipgloss.NewStyle().Foreground(accent),
		Debug:          lipgloss.NewStyle().Foreground(debug),
		ErrorLine:      lipgloss.NewStyle().Foreground(errColor),
		WarningLine:    lipgloss.NewStyle().Foreground(warnColor),
		NoticeLine:     lipgloss.NewStyle().Foreground(text),
		Group:          lipgloss.NewStyle().Foreground(text),
		Section:        lipgloss.NewStyle().Foreground(text).Bold(true),
		FailedSection:  lipgloss.NewStyle().Foreground(errColor).Bold(true),
		Duration:       lipgloss.NewStyle().Foreground(subtle),
		Marker:         lipgloss.NewStyle().Foreground(muted),
		ErrorMark:      lipgloss.NewStyle().Foreground(errColor),
		WarningMark:    lipgloss.NewStyle().Foreground(warnColor),
		NoticeMark:     lipgloss.NewStyle().Foreground(accent),
		Cursor:         lipgloss.NewStyle().Foreground(accent),
		BlurredCursor:  lipgloss.NewStyle().Foreground(subtle),
		LineNumber:     lipgloss.NewStyle().Foreground(subtle),
		Time:           lipgloss.NewStyle().Foreground(subtle),
		Match:          lipgloss.NewStyle().Foreground(text).Background(match),
		CurrentMatch:   lipgloss.NewStyle().Foreground(onCurrent).Background(current),
		Title:          lipgloss.NewStyle().Foreground(text).Bold(true),
		Status:         lipgloss.NewStyle().Foreground(muted),
		NoMatches:      lipgloss.NewStyle().Foreground(errColor),
		Message:        lipgloss.NewStyle().Foreground(muted),
		Spinner:        lipgloss.NewStyle().Foreground(accent),
		LoadError:      lipgloss.NewStyle().Foreground(errColor),
		ErrorGlyph:     "✗",
		ErrorSeparator: " · ",
		ErrorEllipsis:  "…",
		Prompt:         lipgloss.NewStyle().Foreground(accent),
		InputCursor:    lipgloss.NewStyle().Foreground(accent),
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.esc = newEsc(s)
	m.spin.Style = s.Spinner
	m.renderTitle()
	st := textinput.StyleState{
		Text:        s.Text,
		Placeholder: s.Status,
		Suggestion:  s.Status,
		Prompt:      s.Prompt,
	}
	m.input.SetStyles(textinput.Styles{
		Focused: st,
		Blurred: st,
		Cursor:  textinput.CursorStyle{Color: s.InputCursor.GetForeground(), Shape: tea.CursorBlock},
	})
}

// renderTitle renders the title for the status line, on one line and
// without escape sequences.
func (m *Model) renderTitle() {
	m.titleView = m.styles.Title.Render(cleanTitle(m.title))
}

// pair is the escape sequences that turn a style on and off.
// A pair's pen writes content's own styles over its style.
type pair struct {
	on, off string
	pen     termtext.Pen
}

func newPair(s lipgloss.Style) pair {
	// Escape sequences never hold an x, so it marks the content.
	on, off, _ := strings.Cut(s.Render("x"), "x")
	return pair{on: on, off: off, pen: termtext.NewPen(on)}
}

func (p pair) wrap(s string) string { return p.on + s + p.off }

// esc holds the rendered styles. Styling a piece by concatenating its
// escape sequences is much cheaper than rendering it with lipgloss.
type esc struct {
	kinds                        [Debug + 1]pair
	section, failed, duration    pair
	number, time, match, current pair
	status, noMatches            pair
	// Glyphs rendered in their styles.
	open, closed, cursor, blurred      string
	errorMark, warningMark, noticeMark string
}

func newEsc(s Styles) esc {
	e := esc{
		section:     newPair(s.Section),
		failed:      newPair(s.FailedSection),
		duration:    newPair(s.Duration),
		number:      newPair(s.LineNumber),
		time:        newPair(s.Time),
		match:       newPair(s.Match),
		current:     newPair(s.CurrentMatch),
		status:      newPair(s.Status),
		noMatches:   newPair(s.NoMatches),
		open:        s.Marker.Render(openGlyph),
		closed:      s.Marker.Render(closedGlyph),
		cursor:      s.Cursor.Render(cursorGlyph),
		blurred:     s.BlurredCursor.Render(cursorGlyph),
		errorMark:   s.ErrorMark.Render(oneCell(s.ErrorGlyph)),
		warningMark: s.WarningMark.Render(warningGlyph),
		noticeMark:  s.NoticeMark.Render(noticeGlyph),
	}
	e.kinds[Plain] = newPair(s.Text)
	e.kinds[Group] = newPair(s.Group)
	e.kinds[Error] = newPair(s.ErrorLine)
	e.kinds[Warning] = newPair(s.WarningLine)
	e.kinds[Notice] = newPair(s.NoticeLine)
	e.kinds[Command] = newPair(s.Command)
	e.kinds[Debug] = newPair(s.Debug)
	return e
}

// text returns the base style of a row: of its kind, or of its section.
func (e *esc) text(m *Model, r *row) pair {
	switch {
	case r.kind == kindSection && m.secs[r.sec].failed:
		return e.failed
	case r.kind == kindSection:
		return e.section
	case int(r.kind) < len(e.kinds):
		return e.kinds[r.kind]
	default:
		return e.kinds[Plain]
	}
}

// mark returns the mark in the gutter of a row, in its style, or "".
func (e *esc) mark(m *Model, r *row) string {
	switch {
	case r.kind == Error, r.kind == kindSection && m.secs[r.sec].failed:
		return e.errorMark
	case r.kind == Warning:
		return e.warningMark
	case r.kind == Notice:
		return e.noticeMark
	default:
		return ""
	}
}

// oneCell returns g cut or padded to one cell, as the gutter's marks are.
func oneCell(g string) string {
	g = ansi.Truncate(g, 1, "")
	if g == "" {
		return " "
	}
	return g
}
