package treeview

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Defaults of an expand-all: the nodes it reveals, the levels it opens and the
// loads it keeps in flight, at most.
const (
	// DefaultExpandAllNodes is the most rows an expand-all adds to the tree.
	DefaultExpandAllNodes = 1000
	// DefaultExpandAllDepth is the most levels an expand-all opens.
	DefaultExpandAllDepth = 16
	// DefaultMaxLoads is the most loads an expand-all keeps in flight.
	DefaultMaxLoads = 4
)

// spinInterval is the time between frames of the loading spinner.
const spinInterval = 80 * time.Millisecond

// Cache returns the children of the row parent, the zero Node being the top
// level, and true when it has them at once, with no I/O. Each child's Depth
// is relative: 0 is a direct child, and deeper rows are its descendants.
type Cache func(parent Node) ([]Node, bool)

// Load fetches the children of the row parent, the zero Node being the top
// level, as [Cache] describes them. It runs in a [tea.Cmd], and ctx is done
// when the load is dropped.
type Load func(ctx context.Context, parent Node) ([]Node, error)

// Source is where a tree gets its rows. Cached answers at once when it can
// and Load fetches what it could not. Either may be nil.
type Source struct {
	Cached Cache
	Load   Load
}

// Rows returns a source of fixed rows, all given at once. A branch that has
// no rows below it in the list has no children.
func Rows(rows []Node) Source {
	return Source{Cached: func(parent Node) ([]Node, bool) {
		if parent.ID == "" {
			return rows, true
		}
		return nil, true
	}}
}

// Lazy returns a source that loads the children of each branch when it is
// first opened.
func Lazy(load Load) Source { return Source{Load: load} }

// childrenMsg carries the result of one load.
type childrenMsg struct {
	Tree int
	ID   string
	Seq  int
	Kids []Node
	Err  error
}

// tickMsg advances the spinner of a tree.
type tickMsg struct{ Tree int }

// pending is a load in flight.
type pending struct {
	seq    int
	cancel context.CancelFunc
}

// bulk is an expand-all in progress.
type bulk struct {
	on     bool
	queue  []string
	capped bool
	// base is the number of rows the tree had when the expand-all started.
	base int
}

// lazy is the state of loading. Its maps are replaced, never changed in
// place, since models are values that share them.
type lazy struct {
	loaded  map[string]bool
	busy    map[string]pending
	failed  map[string]error
	seq     int
	frame   int
	ticking bool
	boot    tea.Cmd
	reload  []string
	bulk    bulk
	goal    []string
	filter  string
	keep    func(Node) bool
}

func with[V any](c map[string]V, k string, v V) map[string]V {
	out := make(map[string]V, len(c)+1)
	maps.Copy(out, c)
	out[k] = v
	return out
}

func without[V any](c map[string]V, k string) map[string]V {
	if _, ok := c[k]; !ok {
		return c
	}
	out := maps.Clone(c)
	delete(out, k)
	return out
}

// errLine is the text of err on one line, since a newline in it would change
// the height of its row.
func (m Model) errLine(err error) string {
	text, _ := m.errText(err)
	return strings.Join(strings.Fields(text), " ")
}

// errHint is the hint that goes with the text of err, on one line.
func (m Model) errHint(err error) string {
	_, hint := m.errText(err)
	return strings.Join(strings.Fields(hint), " ")
}

// ExpandAllLimits returns the most rows and the most levels an expand-all may
// open, as [WithExpandAllLimits] sets them.
func (m Model) ExpandAllLimits() (nodes, depth int) { return m.maxNodes, m.maxDepth }

// SetExpandAllLimits sets the limits that [WithExpandAllLimits] does, for a
// tree that is already made.
func (m *Model) SetExpandAllLimits(nodes, depth int) {
	m.maxNodes, m.maxDepth = max(nodes, 1), max(depth, 1)
}

// Loading reports whether any load is in flight.
func (m Model) Loading() bool { return len(m.lz.busy) > 0 }

// Err returns why the last load of the children of the row with the ID
// failed, or nil.
func (m Model) Err(id string) error { return m.lz.failed[id] }

// Capped reports whether the last expand-all stopped at its limits.
func (m Model) Capped() bool { return m.lz.bulk.capped }

// Filter returns the filter text.
func (m Model) Filter() string { return m.lz.filter }

// SetFilter shows only the rows whose name holds q, without regard to case,
// and the rows above them, with their folds held open. An empty q shows every
// row again. Only rows that are loaded are matched. It combines with
// [Model.SetFilterFunc]: a row must pass both.
func (m *Model) SetFilter(q string) {
	m.lz.filter = strings.ToLower(strings.TrimSpace(q))
	m.reflow()
}

// SetFilterFunc shows only the rows that keep returns true for, and the rows
// above them, with their folds held open, as [Model.SetFilter] does for a
// name. A nil keep shows every row again. Only rows that are loaded are asked.
func (m *Model) SetFilterFunc(keep func(Node) bool) {
	m.lz.keep = keep
	m.reflow()
}

// filtering reports whether a filter of either kind is set.
func (m Model) filtering() bool { return m.lz.filter != "" || m.lz.keep != nil }

// Filtered returns how many of the children of the row with the ID, "" being
// the top level, show for the filters, and how many it has in all. Without a
// filter the two are equal. A child that shows only because a row below it
// matches counts as shown.
func (m Model) Filtered(id string) (shown, total int) {
	depth, from := 0, 0
	if id != "" {
		r := m.rowIndex(id)
		if r < 0 {
			return 0, 0
		}
		depth, from = m.rows[r].Depth+1, r+1
	}
	hit := m.filterHits()
	for i := from; i < len(m.rows) && (id == "" || m.rows[i].Depth >= depth); i++ {
		if m.rows[i].Depth != depth || m.rows[i].Inline {
			continue
		}
		total++
		if len(hit) == 0 || hit[i] {
			shown++
		}
	}
	return shown, total
}

// filterHits returns which of rows show for the filters, or nil when there is
// none.
func (m Model) filterHits() []bool {
	if !m.filtering() {
		return nil
	}
	hit := make([]bool, len(m.rows))
	for i := range m.rows {
		if m.lz.filter != "" {
			name := m.rows[i].Name
			if name == "" && len(m.rows[i].Lines) > 0 {
				name = ansi.Strip(m.rows[i].Lines[0])
			}
			if !strings.Contains(strings.ToLower(name), m.lz.filter) {
				continue
			}
		}
		if m.lz.keep != nil && !m.lz.keep(m.rows[i]) {
			continue
		}
		hit[i] = true
		need := m.rows[i].Depth
		for k := i - 1; k >= 0 && need > 0; k-- {
			if m.rows[k].Depth < need {
				need = m.rows[k].Depth
				hit[k] = true
			}
		}
	}
	return hit
}

// hasKids reports whether the row r of rows has rows below it.
func (m Model) hasKids(r int) bool {
	return r+1 < len(m.rows) && m.rows[r+1].Depth > m.rows[r].Depth
}

// lazyable reports whether row r of rows is a branch whose children were
// never asked for.
func (m Model) lazyable(r int) bool {
	return (m.src.Load != nil || m.src.Cached != nil) && m.rows[r].Branch &&
		!m.hasKids(r) && !m.lz.loaded[m.rows[r].ID]
}

// wantsLoad reports whether opening row r of rows must load: its children
// were never asked for, or the last load failed. A load in flight is waited
// for.
func (m Model) wantsLoad(r int) bool {
	id := m.rows[r].ID
	if _, busy := m.lz.busy[id]; busy {
		return false
	}
	return m.lz.failed[id] != nil || m.lazyable(r)
}

// startLoad asks the source for the children of id: a cache hit is set at
// once, else a load starts. force skips the cache and keeps the rows that
// show until the new ones arrive.
func (m *Model) startLoad(id string, force bool) tea.Cmd {
	if m.ctx.Err() != nil {
		return nil
	}
	var parent Node
	if id != "" {
		r := m.rowIndex(id)
		if r < 0 {
			return nil
		}
		parent = m.rows[r]
	}
	if !force && m.src.Cached != nil {
		if kids, ok := m.src.Cached(parent); ok {
			m.setKids(id, kids)
			return nil
		}
	}
	if m.src.Load == nil {
		return nil
	}
	m.cancel(id)
	m.lz.seq++
	ctx, cancel := context.WithCancel(m.ctx)
	seq, tree, load := m.lz.seq, m.id, m.src.Load
	m.lz.busy = with(m.lz.busy, id, pending{seq: seq, cancel: cancel})
	m.lz.failed = without(m.lz.failed, id)
	run := func() tea.Msg {
		kids, err := load(ctx, parent)
		return childrenMsg{Tree: tree, ID: id, Seq: seq, Kids: kids, Err: err}
	}
	return tea.Batch(run, m.tick())
}

// cancel drops the load of id that is in flight, if any.
func (m *Model) cancel(id string) {
	if p, ok := m.lz.busy[id]; ok {
		p.cancel()
		m.lz.busy = without(m.lz.busy, id)
	}
}

func (m *Model) cancelAll() {
	for _, p := range m.lz.busy {
		p.cancel()
	}
	m.lz.busy = nil
	m.lz.reload = nil
	m.lz.bulk = bulk{}
	m.lz.goal = nil
}

// tick starts the spinner when it is not running.
func (m *Model) tick() tea.Cmd {
	if m.lz.ticking {
		return nil
	}
	m.lz.ticking = true
	return m.tickCmd()
}

func (m Model) tickCmd() tea.Cmd {
	id := m.id
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return tickMsg{Tree: id} })
}

func (m *Model) onTick(msg tickMsg) tea.Cmd {
	if msg.Tree != m.id {
		return nil
	}
	if len(m.lz.busy) == 0 {
		m.lz.ticking = false
		return nil
	}
	m.lz.frame++
	return m.tickCmd()
}

// receive takes the result of a load. A result that is not of the load in
// flight for its row is dropped.
func (m *Model) receive(msg childrenMsg) tea.Cmd {
	p, ok := m.lz.busy[msg.ID]
	if msg.Tree != m.id || !ok || p.seq != msg.Seq {
		return nil
	}
	p.cancel()
	m.lz.busy = without(m.lz.busy, msg.ID)
	switch {
	case errors.Is(msg.Err, context.Canceled):
		m.lz.goal = nil
	case msg.Err != nil:
		m.lz.failed = with(m.lz.failed, msg.ID, msg.Err)
		if len(m.lz.goal) > 0 && slices.Contains(m.lz.goal, msg.ID) {
			m.lz.goal = nil
		}
	default:
		m.setKids(msg.ID, msg.Kids)
	}
	_, reveal := m.resume()
	return tea.Batch(m.pump(), reveal)
}

// setKids puts kids in place of what is below the row with the ID id. Rows
// below a kid that was already there and has none now are kept, so that a
// reload does not close what the user opened.
func (m *Model) setKids(id string, kids []Node) {
	base, at, end := 0, 0, len(m.rows)
	if id != "" {
		r := m.rowIndex(id)
		if r < 0 {
			return
		}
		base, at = m.rows[r].Depth+1, r+1
		end = at
		for end < len(m.rows) && m.rows[end].Depth > m.rows[r].Depth {
			end++
		}
	}
	fresh := normal(kids, base)
	for i := range fresh {
		if fresh[i].Branch && (i+1 >= len(fresh) || fresh[i+1].Depth <= fresh[i].Depth) {
			fresh[i].Closed = true
		}
	}
	fresh = carry(m.rows[at:end], fresh)
	m.lz.loaded = with(m.lz.loaded, id, true)
	m.lz.failed = without(m.lz.failed, id)
	m.install(slices.Concat(m.rows[:at], fresh, m.rows[end:]))
	if m.lz.bulk.on {
		for i := range fresh {
			n := &fresh[i]
			switch {
			case !n.Branch:
			case n.Depth+1 >= m.maxDepth:
				m.lz.bulk.capped = true
			default:
				m.lz.bulk.queue = append(m.lz.bulk.queue, n.ID)
			}
		}
	}
}

// carry puts back, below each row of fresh that has no rows below it, the
// rows that were below the row of the same ID in old.
func carry(old, fresh []Node) []Node {
	type sub struct {
		depth int
		rows  []Node
	}
	subs := map[string]sub{}
	for j := range old {
		e := j + 1
		for e < len(old) && old[e].Depth > old[j].Depth {
			e++
		}
		if e > j+1 {
			subs[old[j].ID] = sub{old[j].Depth, old[j+1 : e]}
		}
	}
	if len(subs) == 0 {
		return fresh
	}
	out := make([]Node, 0, len(fresh))
	for i := range fresh {
		n := &fresh[i]
		out = append(out, *n)
		s, ok := subs[n.ID]
		if !ok || (i+1 < len(fresh) && fresh[i+1].Depth > n.Depth) {
			continue
		}
		for j := range s.rows {
			r := s.rows[j]
			r.Depth += n.Depth - s.depth
			out = append(out, r)
		}
	}
	return out
}

// pump starts the queued loads of a reload and of an expand-all that fit in
// the limit of loads in flight.
func (m *Model) pump() tea.Cmd {
	var cmds []tea.Cmd
	for len(m.lz.reload) > 0 && len(m.lz.busy) < m.maxLoads {
		id := m.lz.reload[0]
		m.lz.reload = m.lz.reload[1:]
		if id == "" || m.rowIndex(id) >= 0 {
			cmds = append(cmds, m.startLoad(id, true))
		}
	}
	b := &m.lz.bulk
	if !b.on {
		return tea.Batch(cmds...)
	}
	for len(b.queue) > 0 && len(m.lz.busy) < m.maxLoads {
		id := b.queue[0]
		b.queue = b.queue[1:]
		r := m.rowIndex(id)
		if r < 0 || !m.lazyable(r) {
			continue
		}
		if len(m.rows)-b.base >= m.maxNodes {
			b.capped = true
			b.queue = nil
			break
		}
		m.closed = withKey(m.closed, id, false)
		cmds = append(cmds, m.startLoad(id, false))
	}
	if len(b.queue) == 0 && len(m.lz.busy) == 0 {
		b.on = false
		m.reflow()
	}
	return tea.Batch(cmds...)
}

// expandAll opens every fold and loads the branches that were never asked
// for, within the limits of the tree.
func (m *Model) expandAll() tea.Cmd {
	m.lz.bulk = bulk{on: true, base: len(m.rows)}
	for i := range m.rows {
		if !m.folds[i] {
			continue
		}
		r := &m.rows[i]
		m.closed = withKey(m.closed, r.ID, false)
		switch {
		case !m.lazyable(i):
		case r.Depth+1 >= m.maxDepth:
			m.lz.bulk.capped = true
		default:
			m.lz.bulk.queue = append(m.lz.bulk.queue, r.ID)
		}
	}
	m.reflow()
	return m.pump()
}

// fetch loads the children of row r of rows when opening it needs them.
func (m *Model) fetch(r int) tea.Cmd {
	if !m.wantsLoad(r) {
		return nil
	}
	return m.startLoad(m.rows[r].ID, false)
}

// SetSource replaces the source, drops every row and load, and loads the top
// level of the new one. The folds the user left stay.
func (m *Model) SetSource(src Source) tea.Cmd {
	m.cancelAll()
	m.src = src
	m.lz.loaded, m.lz.failed = nil, nil
	m.lz.boot = nil
	m.install(nil)
	return m.startLoad("", false)
}

// Reset drops every row and load and loads the top level again.
func (m *Model) Reset() tea.Cmd { return m.SetSource(m.src) }

// Reload loads again the children of every row that was loaded, bypassing
// the cache. The rows that show stay until the new ones arrive. The loads
// keep to the limit of loads in flight, the rest wait their turn.
func (m *Model) Reload() tea.Cmd {
	m.lz.reload = m.lz.reload[:0:0]
	if m.lz.loaded[""] {
		m.lz.reload = append(m.lz.reload, "")
	}
	for i := range m.rows {
		if id := m.rows[i].ID; m.lz.loaded[id] {
			m.lz.reload = append(m.lz.reload, id)
		}
	}
	return m.pump()
}

// Retry loads again the children of every row whose last load failed, the top
// level included, within the limit of loads in flight.
func (m *Model) Retry() tea.Cmd {
	if len(m.lz.failed) == 0 {
		return nil
	}
	m.lz.reload = m.lz.reload[:0:0]
	if m.lz.failed[""] != nil {
		m.lz.reload = append(m.lz.reload, "")
	}
	for i := range m.rows {
		if id := m.rows[i].ID; m.lz.failed[id] != nil {
			m.lz.reload = append(m.lz.reload, id)
		}
	}
	return m.pump()
}

// Load loads the children of the row with the ID, "" being the top level,
// without opening it. It does nothing when they are loaded or loading, or when
// there is no such row; a failed load is tried again.
func (m *Model) Load(id string) tea.Cmd {
	if id != "" && m.rowIndex(id) < 0 {
		return nil
	}
	if _, busy := m.lz.busy[id]; busy || (m.lz.loaded[id] && m.lz.failed[id] == nil) {
		return nil
	}
	return m.startLoad(id, false)
}

// ReloadNode loads again the children of the row with the ID id, "" being the
// top level, and keeps the rows that show until they arrive.
func (m *Model) ReloadNode(id string) tea.Cmd {
	if id != "" && m.rowIndex(id) < 0 {
		return nil
	}
	return m.startLoad(id, true)
}

// Reveal opens the folds along path, the IDs of the rows from the top down
// to the one wanted, loading what it must, puts the cursor on the last and
// scrolls to it. It reports whether the path is, or may still come, in the
// tree; the command loads the rows on the way, and the cursor moves when
// they arrive, even when the user has moved the cursor since. A key press
// does not cancel it, and another Reveal replaces it.
func (m *Model) Reveal(path ...string) (bool, tea.Cmd) {
	if len(path) == 0 {
		return false, nil
	}
	m.lz.goal = slices.Clone(path)
	return m.resume()
}

// resume goes on with the path of a Reveal as far as the loaded rows allow.
func (m *Model) resume() (bool, tea.Cmd) {
	goal := m.lz.goal
	for i, id := range goal {
		r := m.rowIndex(id)
		if r < 0 {
			if _, busy := m.lz.busy[""]; busy && i == 0 {
				return true, nil
			}
			m.lz.goal = nil
			return false, nil
		}
		if i == len(goal)-1 {
			m.lz.goal = nil
			return m.reveal(id), nil
		}
		if _, busy := m.lz.busy[id]; busy {
			m.closed = withKey(m.closed, id, false)
			m.reflow()
			return true, nil
		}
		if m.wantsLoad(r) {
			m.closed = withKey(m.closed, id, false)
			m.reflow()
			cmd := m.startLoad(id, false)
			if _, busy := m.lz.busy[id]; busy {
				return true, cmd
			}
			if cmd == nil && !m.lz.loaded[id] {
				m.lz.goal = nil
				return false, nil
			}
			return m.resume()
		}
	}
	return false, nil
}

// normal returns a copy of rows, with the depth of each moved down by base,
// the text of each made from its name when it has none, and tabs expanded.
func normal(rows []Node, base int) []Node {
	out := make([]Node, len(rows))
	for i := range rows {
		r := &out[i]
		*r = rows[i]
		r.Depth = max(r.Depth, 0) + base
		if len(r.Lines) == 0 && r.Name != "" {
			r.Lines = []string{termtext.Link(r.Link, r.Name)}
		}
		r.Lines = tidy(r.Lines)
	}
	return out
}
