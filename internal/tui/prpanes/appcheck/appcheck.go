// Package appcheck draws what a check run of an App other than GitHub Actions
// reported: its name, state, title, summary and annotations, with a cursor
// over the annotations. It does no I/O; what the user asks for, such as the
// jump to an annotation, is for the caller to do with [Model.Selected].
package appcheck

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Notes says how far the read of the annotations of the run is.
type Notes int

// The states of the annotations.
const (
	NotesLoaded Notes = iota
	NotesLoading
	NotesFailed
)

// Data is what the page is made of.
type Data struct {
	Check core.CommitCheck
	// Notes are the annotations of the run, and State how far their read is.
	Notes []core.Annotation
	State Notes
}

// Styles are what the page is drawn with. Build them once, with [NewStyles],
// and again when the theme or the icons change.
type Styles struct {
	links *termtext.Links
	bold  lipgloss.Style
	dim   lipgloss.Style
	// state holds the word of each state of a check, colored.
	state [core.CheckFailing + 1]lipgloss.Style
	// failure, warning and notice are the glyphs of the levels of annotations.
	failure, warning, notice string
	ellipsis                 string
	// sep goes between the name of a check, its App and its state.
	sep    string
	cursor string
}

// NewStyles returns the styles for theme t and the icons ic.
func NewStyles(t ui.Theme, ic ui.Icons) Styles {
	s := Styles{
		links:    new(termtext.Links),
		bold:     lipgloss.NewStyle().Bold(true),
		dim:      t.Subtle,
		failure:  t.Run(ui.RunFailure).Render(ic.No),
		warning:  t.Run(ui.RunInProgress).Render(ic.Warning),
		notice:   t.Subtle.Render(ic.Info),
		ellipsis: ic.Ellipsis,
		sep:      " " + ic.Separator + " ",
		cursor:   t.Muted.Render("▌"),
	}
	s.state[core.CheckFailing] = t.Run(ui.RunFailure)
	s.state[core.CheckPending] = t.Run(ui.RunInProgress)
	s.state[core.CheckPassing] = t.Run(ui.RunSuccess)
	return s
}

// Options are what the page needs besides its data.
type Options struct {
	Styles Styles
	// MD renders the summary and the text, which are markdown. Without it they
	// are wrapped as plain text.
	MD *markdown.Renderer
}

// Page is the text of a check run, as lines that fit a width, and the lines
// that are annotations.
type Page struct {
	Lines []string
	// Rows are the indexes of the lines that are annotations, and Notes the
	// annotation of each.
	Rows  []int
	Notes []core.Annotation
}

// String returns the page as text, which a preview reads as it is.
func (p Page) String() string { return strings.Join(p.Lines, "\n") }

// stateWord returns how the check stands, colored.
func (s *Styles) stateWord(c core.CommitCheck) string {
	switch o := c.Outcome(); o {
	case core.CheckFailing:
		return s.state[o].Render("failing")
	case core.CheckPending:
		return s.state[o].Render("pending")
	case core.CheckPassing:
		return s.state[o].Render("passing")
	case core.CheckNone:
	}
	return ""
}

func (s *Styles) link(addr, text string) string {
	if s.links == nil {
		return termtext.Link(addr, text)
	}
	return s.links.Link(addr, text)
}

// Build returns the page of d, in lines of at most width cells.
func Build(d Data, o Options, width int) Page {
	s := &o.Styles
	width = max(width, 1)
	c := d.Check
	var p Page
	add := func(lines ...string) { p.Lines = append(p.Lines, lines...) }
	head := s.bold.Render(termtext.OneLine(c.Name))
	if c.App != nil && c.App.Name != "" {
		head += s.dim.Render(s.sep + termtext.OneLine(c.App.Name) + s.sep)
	} else {
		head += s.dim.Render(s.sep)
	}
	head += s.stateWord(c)
	add(ansi.Truncate(head, width, s.ellipsis))
	if c.DetailsURL != "" {
		text := ansi.Truncate(termtext.OneLine(c.DetailsURL), width, s.ellipsis)
		add(s.dim.Render(s.link(c.DetailsURL, text)))
	}
	if c.Title != "" {
		add("", s.bold.Render(ansi.Truncate(termtext.OneLine(c.Title), width, s.ellipsis)))
	}
	for _, md := range []string{c.Summary, c.Text} {
		if strings.TrimSpace(md) == "" {
			continue
		}
		add("")
		add(strings.Split(strings.TrimRight(render(o.MD, md, width), "\n"), "\n")...)
	}
	n := max(len(d.Notes), c.Annotations)
	if n == 0 {
		return p
	}
	add("", s.bold.Render("Annotations ("+strconv.Itoa(n)+")"))
	switch d.State {
	case NotesLoading:
		add(s.dim.Render("Loading" + s.ellipsis))
	case NotesFailed:
		add(s.dim.Render("The annotations couldn't be read."))
	case NotesLoaded:
	}
	for _, a := range d.Notes {
		p.Rows = append(p.Rows, len(p.Lines))
		p.Notes = append(p.Notes, a)
		add(s.row(a, width))
	}
	return p
}

func render(md *markdown.Renderer, src string, width int) string {
	if md == nil {
		return ansi.Wrap(src, width, "")
	}
	return md.Render(src, width)
}

// row is the line of an annotation: where it is, its level and its message.
func (s *Styles) row(a core.Annotation, width int) string {
	glyph := s.notice
	switch a.Level {
	case core.AnnotationFailure:
		glyph = s.failure
	case core.AnnotationWarning:
		glyph = s.warning
	case core.AnnotationNotice:
	}
	where := a.Path
	if a.StartLine > 0 {
		where += ":" + strconv.Itoa(a.StartLine)
	}
	msg := a.Message
	if msg == "" {
		msg = a.Title
	}
	line := termtext.OneLine(where) + "  " + glyph + " " + termtext.OneLine(msg)
	return ansi.Truncate(line, width, s.ellipsis)
}
