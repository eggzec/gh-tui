package pager

import (
	"context"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The window scrolls through the lines shown, which are every line of the
// content unless vis picks some of them. Positions count the lines shown;
// line indices count the lines of the content, so the line numbers,
// matches and highlights keep to the content whatever is shown.

// count returns the number of lines shown.
func (m Model) count() int {
	if m.vis != nil {
		return len(m.vis)
	}
	return len(m.lines)
}

// at returns the index of the line shown at position p.
func (m Model) at(p int) int {
	if m.vis != nil {
		return int(m.vis[p])
	}
	return p
}

// posOf returns the position of line i if it is shown, or else of the
// first line shown after it, or count past the last.
func (m Model) posOf(i int) int {
	if m.vis == nil {
		return i
	}
	p, _ := slices.BinarySearch(m.vis, int32(i))
	return p
}

// topLine returns the index of the line at the top of the window, or 0
// when no line is shown.
func (m Model) topLine() int {
	if m.top >= m.count() {
		return 0
	}
	return m.at(m.top)
}

// filter picks the lines that re matches, or doesn't match with invert.
// query is what the user typed after the prompt, "" for no filter.
type filter struct {
	query  string
	re     *regexp.Regexp
	invert bool
}

// projection is what picks the lines shown.
type projection struct {
	filter filter
}

// none reports whether p shows every line.
func (p projection) none() bool { return p.filter.re == nil }

// projectMsg carries the lines shown that projection pgen of the pager
// with ID id picked, and how many lines its filter kept.
type projectMsg struct {
	id   int64
	pgen int
	vis  []int32
	kept int
}

// Filter returns what the filter shown was typed as, such as "fix" or
// "!test", or "" if there is none.
func (m Model) Filter() string { return m.proj.filter.query }

// Shown returns the number of lines shown, which is all of them unless a
// filter hides some.
func (m Model) Shown() int { return m.count() }

// project shows the lines p picks. For content smaller than syncLimit, or
// when p has no pattern to match, they are picked at once; otherwise the
// returned command picks them, and the lines shown stay as they are until
// they arrive. A search shown is run again over the new lines.
func (m *Model) project(p projection) tea.Cmd {
	m.stopProjecting()
	m.want = p
	if m.size < syncLimit || p.filter.re == nil {
		vis, kept, _ := pick(context.Background(), m.lines, p)
		return m.picked(vis, kept)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stopProject = cancel
	m.projecting = true
	m.enableSearchKeys()
	id, pgen, all := m.id, m.pgen, m.lines
	return func() tea.Msg {
		defer cancel()
		vis, kept, err := pick(ctx, all, p)
		if err != nil {
			return nil
		}
		return projectMsg{id: id, pgen: pgen, vis: vis, kept: kept}
	}
}

// stopProjecting stops the lines being picked, if they are, and drops what
// they would bring, so what is shown stays.
func (m *Model) stopProjecting() {
	if m.stopProject != nil {
		m.stopProject()
		m.stopProject = nil
	}
	m.pgen++
	m.projecting = false
	m.want = m.proj
	m.enableSearchKeys()
}

// clearProjection shows every line of new content.
func (m *Model) clearProjection() {
	m.stopProjecting()
	m.proj, m.want, m.vis, m.kept = projection{}, projection{}, nil, 0
}

// picked shows the lines the projection asked for picked, and keeps the
// line at the top of the window there, or the first shown after it. A
// filter that kept no line is dropped, with a note that says so, and what
// was shown stays.
func (m *Model) picked(vis []int32, kept int) tea.Cmd {
	m.stopProject = nil
	m.projecting = false
	if m.want.filter.re != nil && kept == 0 {
		m.want = m.proj
		m.flash = noteNotFound
		m.enableSearchKeys()
		return nil
	}
	top := m.topLine()
	m.proj, m.vis, m.kept = m.want, vis, kept
	m.top, m.row = m.posOf(top), 0
	m.hits = hits{}
	m.enableSearchKeys()
	m.clamp()
	if s := m.search; s.re != nil {
		return m.researchShown()
	}
	return nil
}

// pick returns the indices of the lines of all that p shows, or nil for
// all of them, and how many lines its filter kept, until ctx is done.
func pick(ctx context.Context, all []string, p projection) (vis []int32, kept int, err error) {
	if p.none() {
		return nil, len(all), nil
	}
	f := p.filter
	for i, l := range all {
		if i%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
		}
		if f.re != nil && f.re.MatchString(l) == f.invert {
			continue
		}
		kept++
		vis = append(vis, int32(i))
	}
	return vis, kept, nil
}

// filterFor filters the lines by what the user typed after the prompt: a
// pattern as a search takes it, or after a "!" the lines it doesn't match.
// Nothing after the prompt shows every line again. A pattern that doesn't
// compile leaves the filter shown as it was.
func (m *Model) filterFor(line string) tea.Cmd {
	pattern, invert := strings.CutPrefix(line, "!")
	p := m.want
	if strings.TrimSpace(pattern) == "" {
		if p.filter.re == nil && m.proj.filter.re == nil {
			return nil
		}
		p.filter = filter{}
		return m.project(p)
	}
	re, err := compile(pattern)
	if err != nil {
		m.flash = noteInvalid + reason(err)
		return nil
	}
	p.filter = filter{query: line, re: re, invert: invert}
	return m.project(p)
}

// clearFilter stops the filter being picked, if one is, or else shows
// every line again.
func (m *Model) clearFilter() tea.Cmd {
	if m.projecting {
		m.stopProjecting()
		return nil
	}
	p := m.proj
	p.filter = filter{}
	return m.project(p)
}
