package feed

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Texter is implemented by an item that says what its row reads, so that
// find and the quick filter match what the user sees. Without it, the feed
// matches the text the item's [Render] draws, without its styles.
type Texter interface {
	// Text returns the visible text of the row, on one line or several.
	Text() string
}

// noteNotFound is shown in place of the find status when nothing matches.
const noteNotFound = "Pattern not found"

// Prompts, which also say what the line typed after them is for.
const (
	promptFind   = "/"
	promptFilter = "&"
)

// rowWidth is the width an item without [Texter] is rendered in, to read
// its text without it being cut.
const rowWidth = 1000

// newPrompt returns the prompt of a find or filter. It has only the keys
// that close it on an empty line: the feed submits and cancels it itself,
// rather than waiting for the prompt's messages, so a key typed right
// after enter isn't lost to it. It has no completion and no history.
func (m Model[T]) newPrompt() cmdline.Model {
	k := cmdline.KeyMap{CancelEmpty: m.promptKeys.CancelEmpty}
	p := cmdline.New(0, cmdline.WithPrompt(promptFind), cmdline.WithKeyMap(k))
	p.SetStyles(m.promptStyles())
	p.SetSize(m.width, 1)
	return p
}

// Capturing reports whether the prompt of find or the quick filter is open,
// so that the parent leaves every key to the feed, which types them.
func (m Model[T]) Capturing() bool { return m.prompt.Focused() }

// FindQuery returns the text of the find shown, or "" if there is none.
func (m Model[T]) FindQuery() string { return m.query }

// FilterQuery returns the text of the quick filter shown, or "" if there is
// none.
func (m Model[T]) FilterQuery() string { return m.filter }

// Matches returns the number of rows the find shown matches.
func (m Model[T]) Matches() int { return len(m.hits) }

// Shown returns the number of rows the feed shows: those the quick filter
// matches, or every row known without one.
func (m Model[T]) Shown() int { return m.shown() }

// Loaded returns the number of items that are loaded, which is what a
// quick filter looks at.
func (m Model[T]) Loaded() int {
	n := 0
	for _, c := range m.chunks {
		if c.loaded {
			n += c.n
		}
	}
	return n
}

// shown returns the number of rows in the window's list.
func (m Model[T]) shown() int {
	if m.filter == "" {
		return m.total
	}
	return len(m.rows)
}

// at returns the index among all items of the row at position p, or -1.
func (m Model[T]) at(p int) int {
	if m.filter == "" {
		if p < 0 || p >= m.total {
			return -1
		}
		return p
	}
	if p < 0 || p >= len(m.rows) {
		return -1
	}
	return m.rows[p]
}

// posOf returns the position of the first row at or after item g, or the
// last row.
func (m Model[T]) posOf(g int) int {
	if m.filter == "" {
		return g
	}
	return min(sort.SearchInts(m.rows, g), len(m.rows)-1)
}

// matcher returns what tells whether text holds query, ignoring case unless
// the query has a capital.
func matcher(query string) func(string) bool {
	if strings.ContainsFunc(query, unicode.IsUpper) {
		return func(text string) bool { return strings.Contains(text, query) }
	}
	return func(text string) bool { return strings.Contains(strings.ToLower(text), query) }
}

// text returns what the row of it reads, on one line.
func (m Model[T]) text(it T) string {
	var s string
	if t, ok := any(it).(Texter); ok {
		s = t.Text()
	} else {
		s = ansi.Strip(m.render(it, false, rowWidth))
	}
	return strings.Join(strings.Fields(s), " ")
}

// textOf returns what the row of loaded item i reads, kept with its chunk.
func (m *Model[T]) textOf(i int) (string, bool) {
	c := m.chunkAt(i)
	if c < 0 || !m.chunks[c].loaded {
		return "", false
	}
	ch := &m.chunks[c]
	if ch.texts == nil {
		ch.texts = make([]string, len(ch.items))
		for j, it := range ch.items {
			ch.texts[j] = m.text(it)
		}
	}
	return ch.texts[i-m.starts[c]], true
}

// rebuildRows finds the rows the quick filter matches among the loaded
// items, and the rows the find matches among those.
func (m *Model[T]) rebuildRows() {
	if m.filter != "" {
		match := matcher(m.filter)
		m.rows = nil
		for i := range m.total {
			if t, ok := m.textOf(i); ok && match(t) {
				m.rows = append(m.rows, i)
			}
		}
	} else {
		m.rows = nil
	}
	m.refind()
}

// refind finds the rows the find matches.
func (m *Model[T]) refind() {
	m.hits = nil
	if m.query == "" {
		m.enableKeys()
		return
	}
	match := matcher(m.query)
	for p := range m.shown() {
		if t, ok := m.textOf(m.at(p)); ok && match(t) {
			m.hits = append(m.hits, p)
		}
	}
	m.enableKeys()
}

// enableKeys enables the keys that do something in the state of the find
// and the filter, so help shows only those.
func (m *Model[T]) enableKeys() {
	keymap.Enable(&m.keyMap.Next, len(m.hits) > 0)
	keymap.Enable(&m.keyMap.Prev, len(m.hits) > 0)
}

func (m *Model[T]) openPrompt(p string) tea.Cmd {
	m.prompt.SetPrompt(p)
	cmd := m.prompt.Open("")
	m.enableKeys()
	m.scroll()
	return cmd
}

func (m *Model[T]) closePrompt() {
	m.prompt.Blur()
	m.enableKeys()
	m.scroll()
}

// updatePrompt passes msg to the open prompt. The feed submits and cancels
// the prompt itself, rather than waiting for the prompt's messages, so a
// key typed right after enter isn't lost to it.
func (m Model[T]) updatePrompt(msg tea.Msg) (Model[T], tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(k, m.promptKeys.Submit):
			line, p := m.prompt.Value(), m.prompt.Prompt()
			m.closePrompt()
			var cmd tea.Cmd
			if p == promptFilter {
				cmd = m.filterFor(line)
			} else {
				cmd = m.findFor(line)
			}
			return m, cmd
		case key.Matches(k, m.promptKeys.Cancel):
			m.closePrompt()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	if !m.prompt.Focused() {
		// Backspace on an empty line closed it, and said so in cmd.
		m.closePrompt()
		return m, nil
	}
	return m, cmd
}

// findFor finds the rows that hold line and moves to the first at or after
// the selection, wrapping around the end. Nothing found clears the find,
// with a note that says so. An empty line changes nothing.
func (m *Model[T]) findFor(line string) tea.Cmd {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	m.query = line
	m.refind()
	if len(m.hits) == 0 {
		m.query = ""
		m.enableKeys()
		m.note = noteNotFound
		m.scroll()
		return nil
	}
	i := sort.SearchInts(m.hits, m.sel)
	return m.moveTo(m.hits[i%len(m.hits)])
}

// step moves to the next match after the selection, or the previous one
// before it for a negative d, wrapping around at the ends.
func (m *Model[T]) step(d int) tea.Cmd {
	n := len(m.hits)
	if n == 0 {
		return nil
	}
	if d > 0 {
		return m.moveTo(m.hits[sort.SearchInts(m.hits, m.sel+1)%n])
	}
	return m.moveTo(m.hits[(sort.SearchInts(m.hits, m.sel)-1+n)%n])
}

// moveTo selects the row at position p.
func (m *Model[T]) moveTo(p int) tea.Cmd {
	m.sel = p
	m.anchored = false
	return m.sync()
}

// filterFor shows only the rows that hold line, among the loaded ones, or
// every row again for an empty line.
func (m *Model[T]) filterFor(line string) tea.Cmd {
	line = strings.TrimSpace(line)
	if line == "" {
		return m.clearFilter()
	}
	g := m.at(m.sel)
	if g >= 0 {
		m.held = g
	}
	m.filter = line
	m.rebuildRows()
	m.sel, m.top = max(m.posOf(g), 0), 0
	m.anchored = false
	return m.sync()
}

// clearFilter shows every row again, with the selection on the row it was.
func (m *Model[T]) clearFilter() tea.Cmd {
	if m.filter == "" {
		return nil
	}
	g := m.at(m.sel)
	if g < 0 {
		// The filter keeps no row: go back to the one it started at.
		g = m.held
	}
	m.filter, m.rows = "", nil
	m.sel = max(g, 0)
	m.anchored = false
	m.refind()
	return m.sync()
}

// clearFind forgets the find.
func (m *Model[T]) clearFind() {
	m.query, m.hits = "", nil
	m.enableKeys()
	m.scroll()
}

// hasFooter reports whether the last line of the feed holds the prompt, the
// chip of the filter, the find, the count of marks or a note, rather than a
// row.
func (m Model[T]) hasFooter() bool {
	return m.prompt.Focused() || m.cancels() || m.note != ""
}

// footer renders the line below the rows: the prompt while it is open, else
// the chip of the quick filter, the find, the count of marks and a note on
// the last key.
func (m Model[T]) footer() string {
	if m.prompt.Focused() {
		return m.prompt.View()
	}
	var parts []string
	if m.filter != "" {
		parts = append(parts, m.styles.Chip.Render(promptFilter+m.filter), m.styles.Hint.Render(m.filterCount()))
	}
	if m.query != "" {
		s := promptFind + m.query
		if i, ok := slices.BinarySearch(m.hits, m.sel); ok {
			s += "  " + strconv.Itoa(i+1) + "/" + strconv.Itoa(len(m.hits))
		} else {
			s += "  " + strconv.Itoa(len(m.hits)) + " matches"
		}
		if !m.allLoaded() {
			s += " in " + strconv.Itoa(m.Loaded()) + " loaded"
		}
		parts = append(parts, m.styles.Hint.Render(s))
	}
	if len(m.marks) > 0 {
		parts = append(parts, m.styles.Chip.Render(strconv.Itoa(len(m.marks))+" marked"))
	}
	if m.note != "" {
		parts = append(parts, m.styles.Notice.Render(m.note))
	}
	return strings.Join(parts, "  ")
}

// filterCount says how many of the loaded items the filter shows, and that
// it looks only at those.
func (m Model[T]) filterCount() string {
	n, loaded := strconv.Itoa(len(m.rows)), strconv.Itoa(m.Loaded())
	if m.allLoaded() {
		return n + " of " + loaded
	}
	return n + " in " + loaded + " loaded"
}

// allLoaded reports whether every item of the list is loaded, so that a
// find or filter looks at the whole list.
func (m Model[T]) allLoaded() bool {
	return m.done && m.Loaded() == m.total
}

// ShortHelp implements help.KeyMap: the keys of the list, or while the
// prompt is open, those that run and close it.
func (m Model[T]) ShortHelp() []key.Binding {
	if m.prompt.Focused() {
		return []key.Binding{m.submitKey(), m.promptKeys.Cancel}
	}
	return m.keyMap.ShortHelp()
}

// submitKey returns the binding that runs the prompt, which says what it
// does in the prompt open.
func (m Model[T]) submitKey() key.Binding {
	b := m.promptKeys.Submit
	if m.prompt.Prompt() == promptFilter {
		b.SetHelp(b.Help().Key, "quick filter")
	} else {
		b.SetHelp(b.Help().Key, "search")
	}
	return b
}

// FullHelp implements help.KeyMap: the bindings of [KeyMap], with the
// state of the model applied, and the keys of the prompt. While the prompt
// is open, only those that run and close it act. Otherwise the cancel
// key is listed only while a find, a filter or a mark is shown, which it
// clears, in this order; [Model.Update] returns no command for it, whether
// it cleared one or not, so a parent that gives esc other meanings checks
// [Model.Takes] before passing the key on.
func (m Model[T]) FullHelp() [][]key.Binding {
	k := m.keyMap
	if m.prompt.Focused() {
		for _, b := range []*key.Binding{
			&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.HalfPageUp, &k.HalfPageDown, &k.Home, &k.End,
			&k.Retry, &k.Find, &k.QuickFilter, &k.Next, &k.Prev,
		} {
			b.SetEnabled(false)
		}
		return append(m.helpRows(k), []key.Binding{m.submitKey(), m.promptKeys.Cancel, m.promptKeys.CancelEmpty})
	}
	rows := m.helpRows(k)
	if m.cancels() {
		cancel := m.promptKeys.Cancel
		if m.query == "" && m.filter == "" {
			// Only the marks are left to clear.
			cancel.SetHelp(cancel.Help().Key, "clear marks")
		}
		rows = append(rows, []key.Binding{cancel})
	}
	return rows
}

// helpRows returns the rows of k's full help, and the mark key where the
// feed has one.
func (m Model[T]) helpRows(k KeyMap) [][]key.Binding {
	rows := k.FullHelp()
	if m.markKeys.Mark.Help().Desc != "" {
		// The row stays, with its key off, where the rows can't be
		// marked or the prompt takes the keys.
		mark := m.markKeys.Mark
		mark.SetEnabled(m.canMark() && !m.prompt.Focused())
		rows = append(rows, []key.Binding{mark})
	}
	return rows
}

// Takes reports whether msg is a key the feed handles before its parent's
// own keys: any key while the prompt is open, and the cancel key while a
// find, a filter or a mark is shown, which clears it. [Model.Update]
// returns no command for the cancel key whether it cleared one or not.
func (m Model[T]) Takes(msg tea.KeyPressMsg) bool {
	if !m.focused {
		return false
	}
	return m.prompt.Focused() || m.cancels() && key.Matches(msg, m.promptKeys.Cancel)
}
