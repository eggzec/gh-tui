package pulls

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// modalTab is a tab of the modal. Each keeps its state while another shows.
type modalTab int

const (
	conversationTab modalTab = iota
	checksTab
	filesTab
)

// titleRoom is how many columns of the top edge of the frame stay for the
// title for the tabs to be named in full.
const titleRoom = 18

// has reports whether the modal has tab t: the files and the conversation
// always, the checks unless the pull request has none.
func (m *detailModal) has(t modalTab) bool {
	if t == checksTab {
		return m.hasChecks()
	}
	return true
}

// hasChecks reports whether there is a Checks tab: the section reads
// checks, and the pull request has some, or may have, since its detail
// isn't known yet. The tab stays while it shows.
func (m *detailModal) hasChecks() bool {
	switch {
	case m.newChecks == nil:
		return false
	case m.tab == checksTab || !m.loaded:
		return true
	}
	if c, ok := m.knownChecks(); ok && c.Total > 0 {
		return true
	}
	return m.detail.Checks != core.ChecksNone || m.detail.CheckCounts.Total() > 0
}

// tabList returns the tabs the modal has, in order.
func (m *detailModal) tabList() []modalTab {
	if m.hasChecks() {
		return []modalTab{filesTab, conversationTab, checksTab}
	}
	return []modalTab{filesTab, conversationTab}
}

// hasTabs reports whether there is more than one tab, for the keys that
// switch them.
func (m *detailModal) hasTabs() bool { return len(m.tabList()) > 1 }

// Tabs implements ui.Tabbed. Without a second tab there is none to show.
// The names are in full while the title keeps room in the top edge of the
// frame, and short otherwise.
func (m *detailModal) Tabs() (names []string, active int) {
	list := m.tabList()
	if len(list) < 2 {
		return nil, -1
	}
	for _, short := range []bool{false, true} {
		names = names[:0]
		width := 2
		for i, t := range list {
			n := m.label(t, short)
			names = append(names, n)
			width += ansi.StringWidth(n)
			if i > 0 {
				width += ansi.StringWidth(m.icons.Separator)
			}
		}
		// The frame is four columns wider than the modal; its edge takes
		// six more beside the title and the tabs.
		if m.width+4-6-width >= titleRoom {
			break
		}
	}
	for i, t := range list {
		if t == m.tab {
			active = i
		}
	}
	return names, active
}

// label names tab t for the top edge: "Files" with the count of files,
// "Conversation" and "Checks" with how the checks stand, or "Fi", "Co" and
// "Ch" with the count of comments and how the checks stand when short.
func (m *detailModal) label(t modalTab, short bool) string {
	switch t {
	case filesTab:
		name := "Files"
		if short {
			name = "Fi"
		}
		if m.loaded {
			name += " " + strconv.Itoa(m.detail.ChangedFiles)
		}
		return name
	case checksTab:
		name := "Checks"
		if short {
			name = "Ch"
		}
		if g := m.checkGlyph(); g != "" {
			name += " " + g
		}
		return name
	default:
		if !short {
			return "Conversation"
		}
		if m.loaded && m.detail.Comments > 0 {
			return "Co " + strconv.Itoa(m.detail.Comments)
		}
		return "Co"
	}
}

// checkGlyph says how the checks stand: the failing ones counted, or that
// some are pending, or that all passed. It is empty while that isn't known.
func (m *detailModal) checkGlyph() string {
	var failing, pending, passing int
	switch c, ok := m.knownChecks(); {
	case ok && c.Total > 0:
		failing, pending, passing = c.Count()
	case m.loaded && m.detail.CheckCounts.Total() > 0:
		n := m.detail.CheckCounts
		failing, pending, passing = n.Failed, n.Pending, n.Passed
	case m.loaded:
		// The state says how they stand, not how many.
		switch m.detail.Checks {
		case core.ChecksFailure:
			return m.icons.Run(ui.RunFailure)
		case core.ChecksPending:
			pending = 1
		case core.ChecksSuccess:
			passing = 1
		case core.ChecksNone:
		}
	}
	switch {
	case failing > 0:
		return m.icons.Run(ui.RunFailure) + strconv.Itoa(failing)
	case pending > 0:
		return m.icons.Run(ui.RunInProgress)
	case passing > 0:
		return m.icons.Run(ui.RunSuccess)
	}
	return ""
}

// knownChecks returns the checks as the step has them if it read them, or
// else as the cache has them.
func (m *detailModal) knownChecks() (core.Checks, bool) {
	if m.checks != nil {
		if c, ok := m.checks.Checks(); ok {
			return c, true
		}
	}
	return m.cachedChecks()
}

// cycle shows the tab d places on from the one shown, wrapping around.
func (m *detailModal) cycle(d int) tea.Cmd {
	list := m.tabList()
	i := 0
	for j, t := range list {
		if t == m.tab {
			i = j
		}
	}
	return m.switchTo(list[((i+d)%len(list)+len(list))%len(list)])
}

// switchTo shows tab t. The Checks step is built when its tab first shows,
// and polls only while it shows.
func (m *detailModal) switchTo(t modalTab) tea.Cmd {
	if t == m.tab || !m.has(t) {
		return nil
	}
	if m.tab == checksTab && m.checks != nil {
		m.checks.Hide()
	}
	m.tab = t
	switch t {
	case checksTab:
		return m.showChecks()
	case filesTab:
		return m.showFiles()
	case conversationTab:
	}
	if !m.loaded {
		return nil
	}
	// The header counts the checks as the step last read them.
	return m.show()
}

// showChecks starts the Checks step on its first showing, and starts it
// again later.
func (m *detailModal) showChecks() tea.Cmd {
	if m.checks != nil {
		return m.checks.Show()
	}
	m.checks = m.newChecks()
	m.checks.SetTheme(m.theme)
	m.checks.SetSize(m.width, m.height)
	return m.checks.Init()
}
