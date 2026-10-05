package owner

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// calendarLines is what the calendar draws: the total, the months, the
// seven days and the legend, as on the dashboard; calendarHeight is its
// pane, frame included.
const (
	calendarLines  = 10
	calendarHeight = calendarLines + 2
)

// sideConf is what the panes beside the list share across the pages.
type sideConf struct {
	// md renders the READMEs, once the first is.
	md *markdown.Renderer
	// keys are those of the README's pager.
	keys readmeKeys
	// glyph is the cell of a day in the calendar, as
	// dashboard.calendar_glyph sets it, and days its range, as
	// dashboard.contributions does: 0 for the year.
	glyph string
	days  int
	// focus is the pane each page opens with the focus on.
	focus paneID
}

func newSideConf() sideConf {
	def := config.Default()
	return sideConf{
		keys: newReadmeKeys(), glyph: def.Dashboard.CalendarGlyph, days: def.Dashboard.ContributionDays(),
		focus: focusFor(def.Owner.DefaultTab),
	}
}

// focusFor returns the pane a page opens with the focus on, for
// owner.default_tab name: the README's for the README, and the list's for
// the tabs.
func focusFor(name string) paneID {
	if name == config.OwnerTabReadme {
		return readmePane
	}
	return listPane
}

// WithCalendar sets the cell of a day in the contribution calendar of a
// user, which must be one cell wide, and how many days back it goes, 0
// for the year, as the dashboard's calendar does. Without it, they are
// the config's defaults.
func WithCalendar(glyph string, days int) Option {
	return func(s *Section) { s.sc.glyph, s.sc.days = glyph, max(days, 0) }
}

// calendar returns the calendar of p, made at its first use.
func (s *Section) calendar(p *page) *calendar.Model {
	if p.side.cal == nil {
		c := calendar.New(
			calendar.WithGlyph(ownerui.CalendarGlyph(s.sc.glyph, s.icons)),
			calendar.WithRange(s.sc.days),
			calendar.WithStyles(s.theme.Calendar(s.icons)),
			calendar.WithEmptyText("Loading contributions"+s.icons.Ellipsis),
		)
		p.side.cal = &c
		if p == s.page {
			s.resizeSide()
			s.focusSide()
		}
	}
	return p.side.cal
}

// setCalendar shows the contributions of p, once read.
func (s *Section) setCalendar(p *page) {
	if !p.side.contribs.ok {
		return
	}
	ownerui.SetContributions(s.calendar(p), p.side.contribs.value, s.sc.days)
	if p == s.page {
		s.resizeSide()
	}
}

// calendarBody centers the calendar of the page on view in w cells.
func (s *Section) calendarBody(w int) []string {
	p, st := s.page, &s.st.shared
	r := p.side.contribs
	switch {
	case !r.ok && r.err != nil:
		return ownerui.Indent(s.failure("load the contributions of "+p.login, r.err, w-1))
	case !r.ok:
		return []string{" " + st.Muted.Render("Loading contributions"+s.icons.Ellipsis)}
	}
	c := s.calendar(p)
	view := c.View()
	if view == "" {
		return nil
	}
	pad := strings.Repeat(" ", max((w-c.Width())/2, 0))
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return lines
}

// pressCalendar passes msg to the calendar while its pane has the focus.
func (s *Section) pressCalendar(msg tea.KeyPressMsg) tea.Cmd {
	c := s.page.side.cal
	if c == nil {
		return nil
	}
	var cmd tea.Cmd
	*c, cmd = c.Update(msg)
	return cmd
}

// themeSide restyles the pagers, the calendars and the markdown of the
// pages.
func (s *Section) themeSide() {
	if s.sc.md != nil {
		s.sc.md.SetStyle(s.theme.Thread(s.icons).Markdown)
	}
	for _, p := range s.pages() {
		if pg := p.side.pager; pg != nil {
			pg.SetStyles(s.theme.Pager(s.icons))
			pg.Rerender()
		}
		if c := p.side.cal; c != nil {
			c.SetStyles(s.theme.Calendar(s.icons))
			c.SetGlyph(ownerui.CalendarGlyph(s.sc.glyph, s.icons))
		}
	}
}

// configureSide keeps the glyph and the range of the calendars, which the
// set command may have changed, as the dashboard's do.
func (s *Section) configureSide(c config.Config) {
	s.sc.glyph = c.Dashboard.CalendarGlyph
	days := c.Dashboard.ContributionDays()
	changed := days != s.sc.days
	s.sc.days = days
	for _, p := range s.pages() {
		cal := p.side.cal
		if cal == nil {
			continue
		}
		cal.SetGlyph(ownerui.CalendarGlyph(s.sc.glyph, s.icons))
		if changed {
			cal.SetRange(days)
			// What the calendar says of a range without contributions
			// names the range.
			s.setCalendar(p)
		}
	}
}

// redrawSide draws the README of the page on view again, with the images
// that arrived or failed since, if any may change it.
func (s *Section) redrawSide() {
	if p := s.page; p != nil && p.side.pager != nil && s.readmeStale(p) {
		p.side.pager.Rerender()
	}
}
