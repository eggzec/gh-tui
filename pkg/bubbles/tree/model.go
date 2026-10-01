// Package tree provides a lazily loaded tree.
//
// A tree loads the children of a branch from a [Children] function the first
// time the branch is expanded, keeps them when it is collapsed, and renders
// only the rows that fit in its window.
package tree

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Node is one entry of a tree.
type Node struct {
	// ID identifies the node. It must be unique in the tree and not empty;
	// a path works well.
	ID string
	// Name is the text shown for the node.
	Name string
	// Detail is optional text shown dimmed at the right edge of the row,
	// such as the size of a file. It is dropped when the row is too narrow
	// to show it next to enough of the name.
	Detail string
	// Branch reports whether the node can have children. Branches expand;
	// leaves open.
	Branch bool
	// Link is an address the name links to, such as the node's page on
	// the web, which a terminal that knows links (OSC 8) opens on a click.
	// It is optional, and only a plain https address links.
	Link string
	// Value carries whatever the producer wants back with the node, such as
	// the object a leaf stands for.
	Value any
}

// Children returns the children of parent in the order they are shown. The
// zero Node stands for the root, so its children are the top-level nodes.
// Children with an empty or repeated ID are ignored.
type Children func(ctx context.Context, parent Node) ([]Node, error)

var lastID atomic.Int64

func nextID() int {
	return int(lastID.Add(1))
}

// entry is a node and what the tree knows about it.
type entry struct {
	node Node
	// label is the name rendered in its style, and detail the detail.
	label  string
	detail string
	// icon is the icon of the node, and iconOpen that of a branch while
	// it is expanded, or nil for none. Nodes of a kind share one.
	icon     *glyph
	iconOpen *glyph
	parent   string
	// depth is 0 for top-level nodes and -1 for the root.
	depth int
	// kids are the IDs of the children, valid once loaded.
	kids     []string
	loaded   bool
	expanded bool
	loading  bool
	err      error
	// errText and errHint are err rendered as the row says it, worded
	// once as it is set and again when the styles or the words change.
	errText, errHint string
	// seq identifies the load in flight, so older results are dropped.
	seq    int
	cancel context.CancelFunc
}

// Model is a tree of nodes. Create one with [New].
//
// Nodes live in a map shared by copies of the model, as the Elm pattern
// hands each Update the latest copy and drops the old one.
type Model struct {
	settings

	id       int
	children Children
	ctx      context.Context
	cancel   context.CancelFunc

	nodes map[string]*entry
	// glyphs holds each icon once, however many nodes draw it, since a
	// tree has many nodes but few kinds of them.
	glyphs map[string]*glyph
	// rows are the visible entries in order.
	rows []*entry
	sel  int
	top  int
	// seq numbers loads; loads counts those in flight.
	seq   int
	loads int
	bulk  bulk
	// goal is the path of a Reveal in progress, from a top-level node
	// down, or nil.
	goal []string

	initCmd  tea.Cmd
	spin     spinner.Model
	spinning bool

	// Rendered once in SetStyles and SetKeyMap, so View only copies them.
	// guides[d] is the indentation of a row at depth d.
	guides        []string
	gutterFocused string
	gutterBlurred string
	gutterNone    string
	markerOpen    string
	markerClosed  string
	markerLeaf    string
	loadingText   string
	emptyLine     string
	errHint       string
}

// New returns a tree that loads children with children. Call Init to load
// the top-level nodes.
func New(children Children, opts ...Option) Model {
	m := Model{
		settings: defaultSettings(),
		id:       nextID(),
		glyphs:   map[string]*glyph{},
		children: children,
		spin:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	for _, opt := range opts {
		opt(&m.settings)
	}
	m.ctx, m.cancel = context.WithCancel(m.parent)
	// Init sends the first tick, and it cannot record that itself.
	m.spinning = true
	m.initCmd = m.clear()
	m.SetKeyMap(m.keyMap)
	m.SetStyles(m.styles)
	return m
}

// Init loads the top-level nodes and starts the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.initCmd, m.spin.Tick)
}

// Reset forgets every node and loads the top-level nodes again, for example
// for another revision of the same tree. Loads in flight are cancelled.
func (m *Model) Reset() tea.Cmd {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parent)
	return m.clear()
}

// clear replaces the nodes with a root that is loading.
func (m *Model) clear() tea.Cmd {
	root := &entry{depth: -1, expanded: true}
	m.nodes = map[string]*entry{"": root}
	m.rows = nil
	m.sel, m.top = 0, 0
	m.loads = 0
	m.bulk = bulk{}
	m.goal = nil
	return m.startLoad(root)
}

// Reload loads the children of every expanded branch again, for example
// after a sync event. See [Model.ReloadNode].
func (m *Model) Reload() tea.Cmd {
	return m.ReloadNode("")
}

// ReloadNode loads the children of the expanded branches under the node with
// the given ID again, including the node itself; "" is the whole tree. The
// old rows stay until the new ones arrive, branches that still exist stay
// expanded, and the cursor stays on the selected node. Collapsed branches
// forget their children and load them the next time they expand. It stops
// an expand-all in progress.
func (m *Model) ReloadNode(id string) tea.Cmd {
	e := m.nodes[id]
	if e == nil {
		return nil
	}
	m.bulk = bulk{}
	anchor := m.anchor(m.current())
	cmds := m.refresh(e, nil)
	m.flatten(anchor)
	return tea.Batch(cmds...)
}

func (m *Model) refresh(e *entry, cmds []tea.Cmd) []tea.Cmd {
	if !e.node.Branch && e.depth >= 0 {
		return cmds
	}
	if !e.expanded {
		m.forget(e)
		return cmds
	}
	for _, k := range e.kids {
		if c := m.nodes[k]; c != nil {
			cmds = m.refresh(c, cmds)
		}
	}
	return append(cmds, m.startLoad(e))
}

// ID returns the unique ID of the tree.
func (m Model) ID() int {
	return m.id
}

// Selected returns the node under the cursor, or false if there is none.
func (m Model) Selected() (Node, bool) {
	if e := m.current(); e != nil {
		return e.node, true
	}
	return Node{}, false
}

// Index returns the row of the cursor.
func (m Model) Index() int {
	return m.sel
}

// Len returns the number of visible rows.
func (m Model) Len() int {
	return len(m.rows)
}

// Err returns the error of the last load of the top-level nodes, if it
// failed.
func (m Model) Err() error {
	return m.nodes[""].err
}

// SetSize sets the width and height of the tree.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.scroll()
}

// Width returns the width of the tree.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the tree.
func (m Model) Height() int {
	return m.height
}

// Focus makes the tree react to keys.
func (m *Model) Focus() {
	m.focused = true
}

// Blur makes the tree ignore keys.
func (m *Model) Blur() {
	m.focused = false
}

// Focused reports whether the tree reacts to keys.
func (m Model) Focused() bool {
	return m.focused
}

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keyMap = k
	m.refreshHint()
	m.rewordErrors()
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap {
	return m.keyMap
}

// SetStyles sets the styles and renders the fragments that depend on them.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.spin.Style = s.Spinner
	m.gutterFocused = s.Cursor.Render(cursorGlyph) + " "
	m.gutterBlurred = s.BlurredCursor.Render(cursorGlyph) + " "
	m.gutterNone = "  "
	m.markerOpen = s.Marker.Render("▾") + " "
	m.markerClosed = s.Marker.Render("▸") + " "
	m.markerLeaf = "  "
	m.loadingText = s.Loading.Render(" Loading…")
	m.emptyLine = s.Empty.Render(m.emptyText)
	m.guides = nil
	maxDepth := 0
	for _, e := range m.nodes {
		e.label, e.detail = m.label(e.node), m.detail(e.node)
		maxDepth = max(maxDepth, e.depth)
	}
	m.growGuides(maxDepth)
	m.refreshHint()
	m.rewordErrors()
}

// Styles returns the styles.
func (m Model) Styles() Styles {
	return m.styles
}

// SetEmptyText sets the text shown when the tree has no nodes.
func (m *Model) SetEmptyText(text string) {
	m.emptyText = text
	m.emptyLine = m.styles.Empty.Render(text)
}

// SetErrorText sets how a failed load reads, as [WithErrorText] does.
func (m *Model) SetErrorText(say func(error) (text, hint string)) {
	m.errorText = say
	m.rewordErrors()
}

// SetExpandAllLimits changes the caps of an expand-all, for example once
// the producer is known to answer without requests. See
// [WithExpandAllLimits].
func (m *Model) SetExpandAllLimits(nodes, depth int) {
	m.expandNodes, m.expandDepth = max(nodes, 1), max(depth, 1)
}

// ExpandAllLimits returns the caps of an expand-all: the nodes it reveals
// and the levels it opens at most.
func (m Model) ExpandAllLimits() (nodes, depth int) {
	return m.expandNodes, m.expandDepth
}

// SetIcons sets the icons drawn before names, and asks icons again for
// those of every node known. See [WithIcons].
func (m *Model) SetIcons(icons Icons) {
	m.icons = icons
	m.glyphs = map[string]*glyph{}
	for _, e := range m.nodes {
		if e.depth >= 0 {
			m.setIcons(e)
		}
	}
}

// glyph is an icon with the space after it, and its width in cells.
type glyph struct {
	text  string
	width int
}

// setIcons renders the icons of e, once per state, so that View only
// copies them.
func (m *Model) setIcons(e *entry) {
	e.icon, e.iconOpen = m.icon(e.node, false), nil
	if e.node.Branch {
		e.iconOpen = m.icon(e.node, true)
	}
}

// icon returns the icon of n, or nil for none.
func (m *Model) icon(n Node, expanded bool) *glyph {
	if m.icons == nil {
		return nil
	}
	ic := m.icons(n, expanded)
	if ic == "" {
		return nil
	}
	g, ok := m.glyphs[ic]
	if !ok {
		g = &glyph{text: ic + " ", width: ansi.StringWidth(ic) + 1}
		m.glyphs[ic] = g
	}
	return g
}

func (m Model) detail(n Node) string {
	if n.Detail == "" {
		return ""
	}
	return m.styles.Detail.Render(termtext.OneLine(n.Detail))
}

// label renders the name of n, which may come from elsewhere, such as a
// file's name, and hold anything but a slash.
func (m Model) label(n Node) string {
	st := m.styles.Leaf
	if n.Branch {
		st = m.styles.Branch
	}
	return termtext.Link(n.Link, st.Render(termtext.OneLine(n.Name)))
}

// growGuides renders the indentation of every depth up to depth.
func (m *Model) growGuides(depth int) {
	for d := len(m.guides); d <= depth; d++ {
		g := ""
		if d > 0 {
			g = m.styles.Guide.Render(strings.Repeat(guideGlyph+" ", d))
		}
		m.guides = append(m.guides, g)
	}
}

// setErr records err, or nil, as the failure of e's load, and words it.
func (m *Model) setErr(e *entry, err error) {
	e.err, e.errText, e.errHint = err, "", ""
	if err != nil {
		e.errText, e.errHint = m.errorWords(err, e.depth < 0)
	}
}

// rewordErrors words the failed loads again, after the styles, the keys
// or the error text changed.
func (m *Model) rewordErrors() {
	for _, e := range m.nodes {
		if e.err != nil {
			m.setErr(e, e.err)
		}
	}
}

func (m *Model) refreshHint() {
	m.errHint = ""
	if h := m.keyMap.Expand.Help(); h.Key != "" {
		m.errHint = m.styles.Hint.Render(m.styles.ErrorSeparator + h.Key + " to retry")
	}
}

// current returns the entry under the cursor.
func (m Model) current() *entry {
	if m.sel < 0 || m.sel >= len(m.rows) {
		return nil
	}
	return m.rows[m.sel]
}

// startLoad marks e as loading and returns the command that loads its
// children. A load already in flight is cancelled.
func (m *Model) startLoad(e *entry) tea.Cmd {
	if e.cancel != nil {
		e.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.seq++
	e.seq, e.cancel = m.seq, cancel
	m.setErr(e, nil)
	if !e.loading {
		e.loading = true
		m.loads++
	}
	children, id, node, seq := m.children, m.id, e.node, e.seq
	cmd := func() tea.Msg {
		kids, err := children(ctx, node)
		return childrenMsg{tree: id, node: node.ID, seq: seq, kids: kids, err: err}
	}
	if !m.spinning {
		m.spinning = true
		return tea.Batch(cmd, m.spin.Tick)
	}
	return cmd
}

// stop cancels the load of e, if any.
func (m *Model) stop(e *entry) {
	if !e.loading {
		return
	}
	e.cancel()
	e.cancel, e.loading = nil, false
	m.loads--
	m.bulk.unwait(e.node.ID)
}

// forget drops the children of e, so they load again on the next expand.
func (m *Model) forget(e *entry) {
	m.stop(e)
	for _, k := range e.kids {
		if c := m.nodes[k]; c != nil && c.parent == e.node.ID {
			m.drop(c)
		}
	}
	e.kids, e.loaded = nil, false
	m.setErr(e, nil)
}

// drop removes e and everything below it.
func (m *Model) drop(e *entry) {
	m.forget(e)
	delete(m.nodes, e.node.ID)
}

// setKids stores the loaded children of e. Children that were known before
// keep their state, so reloading keeps expanded branches expanded.
func (m *Model) setKids(e *entry, nodes []Node) {
	old := e.kids
	e.kids = make([]string, 0, len(nodes))
	seen := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		e.kids = append(e.kids, n.ID)
		c := m.nodes[n.ID]
		if c != nil && c.parent != e.node.ID {
			// The node moved here from another branch.
			m.detach(c)
			c = nil
		}
		if c == nil {
			c = &entry{node: n, label: m.label(n), detail: m.detail(n), parent: e.node.ID, depth: e.depth + 1}
			m.setIcons(c)
			m.nodes[n.ID] = c
			continue
		}
		c.node, c.label, c.detail = n, m.label(n), m.detail(n)
		m.setIcons(c)
		if !n.Branch {
			m.forget(c)
			c.expanded = false
		}
	}
	for _, k := range old {
		if c := m.nodes[k]; c != nil && !seen[k] {
			m.drop(c)
		}
	}
	e.loaded = true
}

// detach drops e and removes it from its parent's children.
func (m *Model) detach(e *entry) {
	if p := m.nodes[e.parent]; p != nil {
		p.kids = slices.DeleteFunc(slices.Clone(p.kids), func(k string) bool { return k == e.node.ID })
	}
	m.drop(e)
}

// anchor returns the IDs of e and its ancestors, nearest first, so the
// cursor can find its way back after e or its ancestors are dropped.
func (m Model) anchor(e *entry) []string {
	var ids []string
	for e != nil && e.depth >= 0 {
		ids = append(ids, e.node.ID)
		e = m.nodes[e.parent]
	}
	return ids
}

// flatten lists the visible rows again and keeps the cursor on the first
// node of anchor that is still visible.
func (m *Model) flatten(anchor []string) {
	var sel *entry
	if len(anchor) > 0 {
		sel = m.nodes[anchor[0]]
	}
	m.rows = m.rows[:0]
	found := false
	maxDepth := 0
	stack := slices.Clone(m.nodes[""].kids)
	slices.Reverse(stack)
	for len(stack) > 0 {
		e := m.nodes[stack[len(stack)-1]]
		stack = stack[:len(stack)-1]
		if e == nil {
			// Only a producer that repeats IDs across branches gets here.
			continue
		}
		if e == sel && sel != nil {
			m.sel, found = len(m.rows), true
		}
		m.rows = append(m.rows, e)
		maxDepth = max(maxDepth, e.depth)
		if e.expanded {
			for _, k := range slices.Backward(e.kids) {
				stack = append(stack, k)
			}
		}
	}
	for _, id := range anchor {
		if found {
			break
		}
		if e := m.nodes[id]; e != nil {
			if i := slices.Index(m.rows, e); i >= 0 {
				m.sel, found = i, true
			}
		}
	}
	m.growGuides(maxDepth)
	m.scroll()
}

// scroll keeps the cursor in range and in view, with scrollOff rows of
// context above and below it where there are any.
func (m *Model) scroll() {
	m.sel = max(min(m.sel, len(m.rows)-1), 0)
	h := max(m.height, 1)
	rows := len(m.rows)
	if m.hasStatus() {
		rows++
	}
	off := min(m.scrollOff, (h-1)/2)
	m.top = min(m.top, m.sel-off)
	m.top = max(m.top, m.sel+off-h+1)
	m.top = max(min(m.top, rows-h), 0)
	// On the last row, show the status row below it too.
	if m.sel == len(m.rows)-1 && rows > len(m.rows) {
		m.top = max(m.top, rows-h)
	}
}

// hasStatus reports whether a loading, error or empty row follows the rows.
func (m Model) hasStatus() bool {
	return len(m.rows) == 0 || m.nodes[""].err != nil
}
