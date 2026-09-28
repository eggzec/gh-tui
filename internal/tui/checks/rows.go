package checks

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// row is a line of the checks: the title of a group, a check run, or a
// commit status.
type row struct {
	// group titles the rows after it, until the next group.
	group  string
	check  *core.Check
	status *core.StatusContext
}

// job reports whether r is a check of a job of GitHub Actions, which opens
// into its log.
func (r row) job() bool {
	return r.check != nil && r.check.JobID != 0 && r.check.RunID != 0
}

// title reports whether r is the title of a group.
func (r row) title() bool { return r.check == nil && r.status == nil }

// key names r across reads of the checks: a re-run gives a check a new ID,
// but not a new name.
func (r row) key() string {
	switch {
	case r.check != nil:
		return "check\x00" + r.check.Workflow + "\x00" + r.check.Name
	case r.status != nil:
		return "status\x00" + r.status.Context
	}
	return "group\x00" + r.group
}

// name is what r is called.
func (r row) name() string {
	switch {
	case r.check != nil:
		return ui.OneLine(r.check.Name)
	case r.status != nil:
		return ui.OneLine(r.status.Context)
	}
	return r.group
}

// outcome is how r stands.
func (r row) outcome() core.CheckOutcome {
	switch {
	case r.check != nil:
		return r.check.Outcome()
	case r.status != nil:
		return r.status.Outcome()
	}
	return core.CheckPassing
}

// url is where r points on GitHub, or at its app.
func (r row) url() string {
	switch {
	case r.check != nil:
		return r.check.DetailsURL
	case r.status != nil:
		return r.status.TargetURL
	}
	return ""
}

// required reports whether the pull request needs r to pass to merge.
func (r row) required() bool {
	return r.check != nil && r.check.Required || r.status != nil && r.status.Required
}

// state is the run state that r's glyph shows.
func (r row) state() ui.RunState {
	switch {
	case r.check != nil:
		return ui.RunStateOf(r.check.Status, r.check.Conclusion)
	case r.status != nil:
		switch r.status.State {
		case "success":
			return ui.RunSuccess
		case "pending", "expected":
			return ui.RunQueued
		}
		return ui.RunFailure
	}
	return ui.RunNeutral
}

// Group titles of the checks that aren't jobs of a workflow.
const (
	otherGroup  = "Other checks"
	statusGroup = "Statuses"
)

// buildRows lists the checks of c in groups: each workflow, then the
// checks of other apps, then the commit statuses. The groups that fail
// come first, then those pending, and the rows of a group the same way,
// each in the order GitHub gave them.
func buildRows(c *core.Checks) []row {
	type group struct {
		name  string
		worst core.CheckOutcome
		rank  int
		rows  []row
	}
	var groups []*group
	byName := map[string]*group{}
	add := func(name string, rank int, r row) {
		g, ok := byName[name]
		if !ok {
			g = &group{name: name, rank: rank}
			byName[name] = g
			groups = append(groups, g)
		}
		g.rows = append(g.rows, r)
		g.worst = max(g.worst, r.outcome())
	}
	for i := range c.Runs {
		ch := &c.Runs[i]
		if ch.Workflow != "" {
			add(ch.Workflow, 0, row{check: ch})
		} else {
			add(otherGroup, 1, row{check: ch})
		}
	}
	for i := range c.Statuses {
		add(statusGroup, 2, row{status: &c.Statuses[i]})
	}
	slices.SortStableFunc(groups, func(a, b *group) int {
		return cmp.Or(cmp.Compare(b.worst, a.worst), cmp.Compare(a.rank, b.rank))
	})
	rows := make([]row, 0, len(c.Runs)+len(c.Statuses)+len(groups))
	for _, g := range groups {
		slices.SortStableFunc(g.rows, func(a, b row) int { return cmp.Compare(b.outcome(), a.outcome()) })
		rows = append(rows, row{group: g.name})
		rows = append(rows, g.rows...)
	}
	return rows
}

// setChecks shows c, with the cursor on the check it was on, or on the
// first failing check the first time.
func (s *Step) setChecks(c core.Checks) {
	prev, had := s.selected()
	s.checks, s.loaded, s.err = c, true, nil
	s.rows = buildRows(&s.checks)
	switch {
	case had && s.opened:
		i := slices.IndexFunc(s.rows, func(r row) bool { return r.key() == prev.key() })
		s.cursor = s.firstRow(max(i, 0))
	default:
		s.cursor = s.firstRow(firstFailing(s.rows))
		s.opened = true
	}
	s.scroll()
	if s.mode != listMode {
		s.refreshShown()
	}
}

// firstFailing is the row the checks open on: the first that fails, else
// the first pending, else the first.
func firstFailing(rows []row) int {
	for _, o := range []core.CheckOutcome{core.CheckFailing, core.CheckPending} {
		if i := slices.IndexFunc(rows, func(r row) bool { return !r.title() && r.outcome() == o }); i >= 0 {
			return i
		}
	}
	return 0
}

// firstRow is the first row from i on that isn't a title, or i.
func (s *Step) firstRow(i int) int {
	for j := i; j < len(s.rows); j++ {
		if !s.rows[j].title() {
			return j
		}
	}
	return i
}

// selected returns the check or status under the cursor.
func (s *Step) selected() (row, bool) {
	if s.cursor < 0 || s.cursor >= len(s.rows) || s.rows[s.cursor].title() {
		return row{}, false
	}
	return s.rows[s.cursor], true
}

// current returns the check shown, or else the one under the cursor.
func (s *Step) current() (row, bool) {
	if s.mode != listMode {
		return s.check, true
	}
	return s.selected()
}

// move moves the cursor d rows, over the titles of the groups, and no
// further than the ends.
func (s *Step) move(d int) {
	if len(s.rows) == 0 {
		return
	}
	to := min(max(s.cursor+d, 0), len(s.rows)-1)
	step := 1
	if d < 0 {
		step = -1
	}
	for to >= 0 && to < len(s.rows) && s.rows[to].title() {
		to += step
	}
	if to < 0 || to >= len(s.rows) {
		// Past the last check, or before the first: stay on the nearest.
		to = s.firstRow(0)
		if d > 0 {
			to = s.cursor
		}
	}
	s.cursor = to
	s.scroll()
}

// scroll keeps the cursor in view, with the title of its group when there
// is room.
func (s *Step) scroll() {
	h := max(s.listRows(), 1)
	s.top = min(s.top, max(s.cursor-1, 0))
	if s.cursor > 0 && s.rows[s.cursor-1].title() && s.cursor <= s.top {
		s.top = s.cursor - 1
	}
	if s.cursor >= s.top+h {
		s.top = s.cursor - h + 1
	}
	s.top = max(min(s.top, len(s.rows)-h), 0)
}

// listRows is how many rows of checks the list shows: all but the crumb.
func (s *Step) listRows() int {
	return max(s.height-1, 0)
}

// summary counts the checks by how they stand.
func (s *Step) summary() string {
	return Summary(s.checks, s.st.run)
}

// Summary counts the checks of c by how they stand, in the glyphs and
// styles of st, such as "✗ 3 failing, ◐ 1 pending, ✓ 12 passed", or is
// empty without checks.
func Summary(c core.Checks, st ui.RunStyles) string {
	failing, pending, passing := c.Count()
	parts := make([]string, 0, 3)
	if failing > 0 {
		parts = append(parts, st.Glyphs[ui.RunFailure]+" "+st.Text.Render(strconv.Itoa(failing)+" failing"))
	}
	if pending > 0 {
		parts = append(parts, st.Glyphs[ui.RunInProgress]+" "+st.Text.Render(strconv.Itoa(pending)+" pending"))
	}
	if passing > 0 {
		parts = append(parts, st.Glyphs[ui.RunSuccess]+" "+st.Text.Render(strconv.Itoa(passing)+" passed"))
	}
	return strings.Join(parts, st.Subtle.Render(", "))
}

// listLines renders the checks, h lines of w cells.
func (s *Step) listLines(w, h int) []string {
	st := &s.st
	switch {
	case !s.loaded && s.err != nil:
		return ui.FitLines(ui.Wrap(s.errorLine("Couldn't load the checks: ", s.err), w), w, h)
	case !s.loaded:
		return ui.FitLines([]string{s.spin.View() + st.run.Muted.Render("Loading the checks…")}, w, h)
	case len(s.rows) == 0:
		return ui.FitLines(ui.Wrap(st.run.Muted.Render("No checks have reported on the head commit of this pull request."), w), w, h)
	}
	lines := make([]string, 0, h)
	now := s.now()
	for i := s.top; i < len(s.rows) && len(lines) < h; i++ {
		lines = append(lines, s.renderRow(s.rows[i], i == s.cursor, w, now))
	}
	if s.checks.Truncated && len(lines) < h {
		text := "Only the first " + strconv.Itoa(len(s.checks.Runs)+len(s.checks.Statuses)) + " of " + strconv.Itoa(s.checks.Total) + " checks are listed."
		lines = append(lines, ui.Fit(st.noGutter+st.run.Subtle.Render(text), w))
	}
	return ui.PadLines(lines, w, h)
}

// renderRow renders a row: the title of a group, or a check with its
// state, name, whether it is required, and how long it ran.
func (s *Step) renderRow(r row, cursor bool, w int, now time.Time) string {
	st := &s.st
	if r.title() {
		return ui.Fit(st.group.Render(ui.OneLine(r.group)), w)
	}
	gutter := st.noGutter
	name := st.run.Text.Render(r.name())
	if cursor {
		gutter = st.gutter
		name = st.run.Strong.Render(r.name())
	}
	// The name links to where the check points, as the open key does.
	left := gutter + st.run.Glyphs[r.state()] + " " + s.links.Link(r.url(), name)
	if r.required() {
		left += " " + st.required.Render("required")
	}
	var right string
	switch {
	case r.check != nil:
		c := r.check
		right = st.run.Took(c.Status, c.Conclusion, c.StartedAt, c.CompletedAt, now)
	case r.status != nil && r.status.Description != "":
		left += "  " + st.run.Subtle.Render(ui.OneLine(r.status.Description))
	}
	return ui.Spread(left, right, w)
}

// errorLine renders an error that the refresh key reads again.
func (s *Step) errorLine(what string, err error) string {
	text := s.st.run.Error.Render("✗ " + what + ui.FirstLine(err.Error()))
	if k := s.keys.Refresh.Help().Key; k != "" {
		text += s.st.run.Subtle.Render(" · " + k + " to retry")
	}
	return text
}
