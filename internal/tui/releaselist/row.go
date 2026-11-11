package releaselist

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// A row is "● v1.2.0   Title…   latest  ◦ 3  octocat  3d": the glyph of the
// state, the tag, the title, then the latest mark, the file count, the author
// and the age in fixed columns on the right.
const (
	tagWidth    = 12
	latestWidth = 6 // "latest"
	authorWidth = 8
	filesWidth  = 4 // "999 "
	gap         = 2
	minTitle    = 12
)

// paint wraps text in the escape codes of a style, built once per theme.
type paint struct{ pre, post string }

func newPaint(s lipgloss.Style) paint {
	pre, post, _ := strings.Cut(s.Render("x"), "x")
	return paint{pre, post}
}

func (p paint) wrap(text string) string { return p.pre + text + p.post }

// cells are the glyphs and paints of a row in one look.
type cells struct {
	states                [ui.NumStates]string
	tag, title, meta, age paint
	latest, files         paint
}

func newCells(t ui.Theme, icons ui.Icons, look int) cells {
	selected := look != ui.LookPlain
	cell := func(s lipgloss.Style) lipgloss.Style { return t.Cell(s, selected) }
	var states [ui.NumStates]string
	for _, s := range []ui.State{ui.ReleaseDraft, ui.ReleasePre, ui.ReleasePublished} {
		states[s] = cell(t.State(s)).Render(icons.State(s))
	}
	return cells{
		states: states,
		tag:    newPaint(cell(t.Muted)),
		title:  newPaint(t.Cell(t.RowTitle(selected, look == ui.LookFocused), selected)),
		meta:   newPaint(cell(t.Muted)),
		age:    newPaint(cell(t.Subtle)),
		latest: newPaint(cell(t.State(ui.ReleasePublished))),
		files:  newPaint(cell(t.Muted)),
	}
}

func (s *Section) ageRoom() int { return max(s.dates.Width(), len(ui.AgeTitle)) }

// render draws a release as one line of the list.
func (s *Section) render(r core.Release, selected bool, width int) string {
	cl := &s.looks[ui.Look(selected, s.feed.Focused())]
	state := ui.ReleasePublished
	switch {
	case r.Draft:
		state = ui.ReleaseDraft
	case r.Prerelease:
		state = ui.ReleasePre
	}
	age := ""
	if t := r.PublishedAt; !t.IsZero() {
		age = s.dates.Short(t, s.now())
	} else if !r.CreatedAt.IsZero() {
		age = s.dates.Short(r.CreatedAt, s.now())
	}
	ageW := s.ageRoom()

	showAuthor, showFiles, showLatest := true, true, true
	right := func() int {
		w := gap + ageW
		if showLatest {
			w += gap + latestWidth
		}
		if showFiles {
			w += gap + filesWidth
		}
		if showAuthor {
			w += gap + authorWidth
		}
		return w
	}
	prefix := 2 + tagWidth + 1
	for _, drop := range []*bool{&showAuthor, &showFiles, &showLatest} {
		if width-prefix-right() >= minTitle {
			break
		}
		*drop = false
	}
	titleW := max(width-prefix-right(), 0)

	title := r.Name
	if title == r.Tag {
		title = ""
	}
	if r.Draft {
		title = strings.TrimSpace("draft " + title)
	} else if r.Prerelease {
		title = strings.TrimSpace("pre-release " + title)
	}

	var b strings.Builder
	b.WriteString(cl.states[state])
	b.WriteByte(' ')
	b.WriteString(cl.tag.wrap(termtext.Fit(ui.OneLine(r.Tag), tagWidth, s.icons.Ellipsis)))
	b.WriteByte(' ')
	b.WriteString(cl.title.wrap(termtext.Fit(ui.OneLine(title), titleW, s.icons.Ellipsis)))
	if showLatest {
		b.WriteString(strings.Repeat(" ", gap))
		if r.Latest {
			b.WriteString(cl.latest.wrap("latest"))
		} else {
			b.WriteString(strings.Repeat(" ", latestWidth))
		}
	}
	if showFiles {
		b.WriteString(strings.Repeat(" ", gap))
		n := ""
		if len(r.Assets) > 0 {
			n = s.icons.Event + strconv.Itoa(len(r.Assets))
		}
		b.WriteString(cl.files.wrap(termtext.Fit(n, filesWidth, "")))
	}
	if showAuthor {
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(cl.meta.wrap(termtext.Fit(ui.OneLine(r.Author.Login), authorWidth, s.icons.Ellipsis)))
	}
	b.WriteString(strings.Repeat(" ", gap))
	b.WriteString(cl.age.wrap(termtext.Fit(age, ageW, "")))
	return ansi.Truncate(b.String(), width, "")
}
