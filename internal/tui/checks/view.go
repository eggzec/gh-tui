package checks

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// View renders the step in exactly the size of the last SetSize: a crumb
// of where it is, over the checks, the job or the detail. A confirmation
// or a notice takes the last line.
func (s *Step) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	w, h := s.width, s.bodyHeight()
	lines := make([]string, 0, s.height)
	lines = append(lines, s.crumbLine(w))
	switch s.mode {
	case jobMode:
		lines = append(lines, s.jobLines(w, h)...)
	case detailMode:
		lines = append(lines, s.detailLines(w, h)...)
	case listMode:
		lines = append(lines, s.listLines(w, h)...)
	}
	if p := s.prompt(w); p != "" {
		lines[len(lines)-1] = p
	}
	return strings.Join(lines, "\n")
}

// bodyHeight is the height below the crumb.
func (s *Step) bodyHeight() int {
	return max(s.height-1, 0)
}

// layout sizes the job view and the detail to the room below the crumb.
func (s *Step) layout() {
	h := s.bodyHeight()
	s.view.SetSize(s.width, h)
	// The detail has how the check stands above it.
	s.detail.SetWidth(s.width)
	s.detail.SetHeight(max(h-1, 0))
	s.renderDetail()
	if len(s.rows) > 0 {
		s.scroll()
	}
}

// crumbLine shows where the step is: the checks, and the group and name of
// the check shown, with the count of the checks by how they stand at the
// right of the list.
func (s *Step) crumbLine(w int) string {
	crumbs := []string{Title}
	if s.mode != listMode {
		if g := s.groupOf(s.check); g != "" {
			crumbs = append(crumbs, g)
		}
		crumbs = append(crumbs, s.check.name())
	}
	const sep = " › "
	// Drop the first crumbs until the rest fit.
	for len(crumbs) > 1 && ansi.StringWidth(strings.Join(crumbs, sep)) > w {
		crumbs = crumbs[1:]
		if crumbs[0] != "…" {
			crumbs = append([]string{"…"}, crumbs[1:]...)
		}
	}
	var b strings.Builder
	for i, c := range crumbs {
		if i > 0 {
			b.WriteString(s.st.run.Subtle.Render(sep))
		}
		st := s.st.crumb
		if i == len(crumbs)-1 {
			st = s.st.lastCrumb
		}
		b.WriteString(st.Render(c))
	}
	var right string
	if s.mode == listMode && s.loaded {
		right = s.summary()
	}
	return ui.Spread(ansi.Truncate(b.String(), w, "…"), right, w)
}

// groupOf is the title of the group of r.
func (s *Step) groupOf(r row) string {
	group := ""
	want := r.key()
	for _, x := range s.rows {
		if x.title() {
			group = x.group
		} else if x.key() == want {
			return group
		}
	}
	return ""
}

// jobLines renders the job of the check shown, h lines of w cells.
func (s *Step) jobLines(w, h int) []string {
	st := &s.st
	switch {
	case !s.job.hasJob && s.job.err != nil:
		return ui.FitLines(ui.Wrap(s.errorLine("Couldn't load the job: ", s.job.err), w), w, h)
	case !s.job.hasJob:
		return ui.FitLines([]string{s.spin.View() + st.run.Muted.Render("Loading the job…")}, w, h)
	}
	return ui.PadLines(strings.Split(s.view.View(), "\n"), w, h)
}

// prompt renders the confirmation or the notice on the last line, if
// there is one.
func (s *Step) prompt(w int) string {
	switch {
	case s.ask != nil:
		return s.ask.Line(s.st.confirm, s.keys.Confirm, w)
	case s.notice != "":
		return ui.Fit(s.st.run.Warning.Render(ansi.Truncate(s.notice, w, "…")), w)
	}
	return ""
}
