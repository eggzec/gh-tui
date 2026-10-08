package tree

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// files is a file tree served by children. Node IDs are paths.
type files struct {
	mu   sync.Mutex
	kids map[string][]Node
	fail map[string]error
	// details holds the detail of nodes by ID.
	details map[string]string
	calls   []string
}

// newFiles builds a tree from file paths. A path ending in "/" is an empty
// directory.
func newFiles(paths ...string) *files {
	f := &files{kids: map[string][]Node{}, fail: map[string]error{}, details: map[string]string{}}
	for _, p := range paths {
		f.add(p)
	}
	return f
}

func (f *files) add(p string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dir := strings.HasSuffix(p, "/")
	parts := strings.Split(strings.TrimSuffix(p, "/"), "/")
	parent := ""
	for i, name := range parts {
		id := path.Join(parent, name)
		branch := dir || i < len(parts)-1
		if !slices.ContainsFunc(f.kids[parent], func(n Node) bool { return n.ID == id }) {
			f.kids[parent] = append(f.kids[parent], Node{ID: id, Name: name, Branch: branch})
			slices.SortFunc(f.kids[parent], func(a, b Node) int {
				if a.Branch != b.Branch {
					if a.Branch {
						return -1
					}
					return 1
				}
				return cmp.Compare(a.Name, b.Name)
			})
		}
		parent = id
	}
}

// remove deletes the node with the given ID from its parent.
func (f *files) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parent := path.Dir(id)
	if parent == "." {
		parent = ""
	}
	f.kids[parent] = slices.DeleteFunc(f.kids[parent], func(n Node) bool { return n.ID == id })
}

func (f *files) children(ctx context.Context, parent Node) ([]Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, parent.ID)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := f.fail[parent.ID]; err != nil {
		return nil, err
	}
	kids := slices.Clone(f.kids[parent.ID])
	for i, n := range kids {
		kids[i].Detail = f.details[n.ID]
	}
	return kids, nil
}

func (f *files) setDetail(id, detail string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.details[id] = detail
}

func (f *files) setFail(id string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.fail, id)
		return
	}
	f.fail[id] = err
}

func (f *files) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == id {
			n++
		}
	}
	return n
}

// repo is a small repository layout used by most tests.
func repo() *files {
	return newFiles(
		"cmd/gh-tui/main.go",
		"docs/",
		"internal/core/repo.go",
		"internal/core/pull.go",
		"internal/tui/app.go",
		"README.md",
		"go.mod",
	)
}

// run executes cmd and feeds every resulting message back into m, until no
// commands are left. Spinner ticks are dropped so tests never sleep.
func run(tb testing.TB, m Model, cmd tea.Cmd) Model {
	tb.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case spinner.TickMsg:
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			queue = append(queue, next)
		}
	}
	return m
}

// load builds a focused tree over f and loads the top-level nodes.
func load(tb testing.TB, f *files, opts ...Option) Model {
	tb.Helper()
	opts = append([]Option{WithSize(40, 10), WithFocused(true)}, opts...)
	m := newModel(f.children, opts...)
	return run(tb, m, m.Init())
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// keys presses each key and runs the resulting commands.
func keys(tb testing.TB, m Model, ks ...string) Model {
	tb.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		m = run(tb, m, cmd)
	}
	return m
}

// rowIDs returns the IDs of the visible rows.
func rowIDs(m Model) []string {
	ids := make([]string, 0, len(m.rows))
	for _, e := range m.rows {
		ids = append(ids, e.node.ID)
	}
	return ids
}

func selectedID(m Model) string {
	n, _ := m.Selected()
	return n.ID
}

func assertVisible(tb testing.TB, m Model) {
	tb.Helper()
	if m.Len() > 0 && (m.sel < m.top || m.sel >= m.top+m.height) {
		tb.Fatalf("cursor %d outside window [%d, %d)", m.sel, m.top, m.top+m.height)
	}
}

var roots = []string{"cmd", "docs", "internal", "README.md", "go.mod"}

func TestInitLoadsTopLevelNodes(t *testing.T) {
	f := repo()
	m := newModel(f.children, WithSize(40, 10))
	if !m.hasStatus() || m.Len() != 0 {
		t.Fatal("new tree should show the loading row")
	}
	m = run(t, m, m.Init())
	if got := rowIDs(m); !slices.Equal(got, roots) {
		t.Fatalf("rows = %v, want %v", got, roots)
	}
	if selectedID(m) != "cmd" {
		t.Fatalf("Selected() = %q, want cmd", selectedID(m))
	}
	if f.callCount("") != 1 || len(f.calls) != 1 {
		t.Fatalf("calls = %v, want only the root", f.calls)
	}
}

func TestKeys(t *testing.T) {
	internalAll := []string{"cmd", "docs", "internal", "internal/core", "internal/core/pull.go",
		"internal/core/repo.go", "internal/tui", "internal/tui/app.go", "README.md", "go.mod"}
	cmdOpen := []string{"cmd", "cmd/gh-tui", "docs", "internal", "README.md", "go.mod"}
	docsOpen := []string{"cmd", "docs", "internal", "README.md", "go.mod"}
	tests := []struct {
		name     string
		keys     []string
		wantRows []string
		wantSel  string
	}{
		{"down", []string{"down"}, roots, "docs"},
		{"j and k", []string{"j", "j", "k"}, roots, "docs"},
		{"up stops at start", []string{"up"}, roots, "cmd"},
		{"end", []string{"end"}, roots, "go.mod"},
		{"G then g", []string{"G", "g"}, roots, "cmd"},
		{"page down", []string{"pgdown"}, roots, "go.mod"},
		{"page up", []string{"pgdown", "pgup"}, roots, "cmd"},
		{"ctrl+f and ctrl+b", []string{"ctrl+f", "ctrl+b"}, roots, "cmd"},
		{"half page down", []string{"j", "j", "*", "ctrl+d"}, internalAll, "internal/tui/app.go"},
		{"half page up", []string{"j", "j", "*", "ctrl+d", "ctrl+u"}, internalAll, "internal"},
		{"plus expands", []string{"+"}, cmdOpen, "cmd"},
		{"plus on expanded stays", []string{"+", "+"}, cmdOpen, "cmd"},
		{"right expands", []string{"right"}, cmdOpen, "cmd"},
		{"right enters", []string{"l", "l"}, cmdOpen, "cmd/gh-tui"},
		{"right goes deeper", []string{"l", "l", "l", "l"},
			[]string{"cmd", "cmd/gh-tui", "cmd/gh-tui/main.go", "docs", "internal", "README.md", "go.mod"},
			"cmd/gh-tui/main.go"},
		{"minus collapses", []string{"+", "-"}, roots, "cmd"},
		{"left moves to parent", []string{"l", "l", "left"}, cmdOpen, "cmd"},
		{"h on collapsed child moves to parent", []string{"l", "l", "h", "h"}, roots, "cmd"},
		{"left on top level stays", []string{"h"}, roots, "cmd"},
		{"enter expands", []string{"enter"}, cmdOpen, "cmd"},
		{"enter collapses", []string{"enter", "enter"}, roots, "cmd"},
		{"empty branch", []string{"j", "+"}, docsOpen, "docs"},
		{"right on empty branch stays", []string{"j", "l", "l"}, docsOpen, "docs"},
		{"plus on leaf does nothing", []string{"G", "+"}, roots, "go.mod"},
		{"collapse on leaf moves to parent", []string{"j", "j", "l", "l", "l", "l", "-"},
			[]string{"cmd", "docs", "internal", "internal/core", "internal/core/pull.go",
				"internal/core/repo.go", "internal/tui", "README.md", "go.mod"}, "internal/core"},
		{"expand all", []string{"j", "j", "*"}, internalAll, "internal"},
		{"expand all on leaf does nothing", []string{"G", "*"}, roots, "go.mod"},
		{"collapse all", []string{"j", "j", "*", "j", "j", "="}, roots, "internal"},
		{"collapse all keeps children", []string{"j", "j", "*", "=", "l"},
			[]string{"cmd", "docs", "internal", "internal/core", "internal/tui", "README.md", "go.mod"},
			"internal"},
		{"unbound key", []string{"x"}, roots, "cmd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, repo())
			m = keys(t, m, tt.keys...)
			if got := rowIDs(m); !slices.Equal(got, tt.wantRows) {
				t.Fatalf("rows = %v, want %v", got, tt.wantRows)
			}
			if got := selectedID(m); got != tt.wantSel {
				t.Fatalf("Selected() = %q, want %q", got, tt.wantSel)
			}
			assertVisible(t, m)
		})
	}
}

func TestCollapseKeepsChildren(t *testing.T) {
	f := repo()
	m := load(t, f)
	m = keys(t, m, "+", "-", "+", "-", "l")
	if got := f.callCount("cmd"); got != 1 || m.Len() != len(roots)+1 {
		t.Fatalf("cmd loaded %d times with %d rows; want 1, %d", got, m.Len(), len(roots)+1)
	}
}

func TestBranchLoading(t *testing.T) {
	m := load(t, repo())
	m, cmd := m.Update(press("+"))
	if cmd == nil {
		t.Fatal("expanding an unloaded branch returned no command")
	}
	if !m.rows[0].loading || m.Len() != len(roots) {
		t.Fatal("the branch should show as loading, without children yet")
	}
	// Pressing again while it loads does not load twice.
	if _, again := m.Update(press("+")); again != nil {
		t.Fatal("expanding a loading branch loaded it again")
	}
	m = run(t, m, cmd)
	if m.rows[0].loading || m.Len() != len(roots)+1 {
		t.Fatalf("rows = %v after loading", rowIDs(m))
	}
}

func TestBranchErrorAndRetry(t *testing.T) {
	boom := errors.New("502 Bad Gateway\nmore details")
	f := repo()
	f.setFail("internal", boom)
	m := load(t, f)
	m.SetSize(60, 10)
	m = keys(t, m, "j", "j", "+")
	e := m.nodes["internal"]
	if !errors.Is(e.err, boom) || e.loading {
		t.Fatalf("err = %v, loading = %v; want the error", e.err, e.loading)
	}
	if m.Err() != nil {
		t.Fatal("Err() should only report the top-level load")
	}
	v := m.View()
	if !strings.Contains(v, "502 Bad Gateway") || strings.Contains(v, "more details") {
		t.Fatalf("View() = %q, want the first line of the error", v)
	}
	if !strings.Contains(v, "+ to retry") {
		t.Fatalf("View() = %q, want the retry hint", v)
	}

	// Right on a failed branch retries too, rather than moving in.
	m = keys(t, m, "l")
	if f.callCount("internal") != 2 || selectedID(m) != "internal" {
		t.Fatalf("right on a failed branch: calls = %d, Selected() = %q", f.callCount("internal"), selectedID(m))
	}

	f.setFail("internal", nil)
	m = keys(t, m, "+")
	if e := m.nodes["internal"]; e.err != nil || len(e.kids) != 2 {
		t.Fatalf("after retry: err = %v, kids = %v", e.err, e.kids)
	}
}

func TestRootErrorAndRetry(t *testing.T) {
	f := repo()
	f.setFail("", errors.New("offline"))
	m := load(t, f)
	if m.Err() == nil || m.Len() != 0 {
		t.Fatalf("Err() = %v, Len() = %d", m.Err(), m.Len())
	}
	if text, hint := m.statusLine(); !strings.Contains(text, "offline") || !strings.Contains(hint, "retry") {
		t.Fatalf("status = %q %q", text, hint)
	}
	f.setFail("", nil)
	m = keys(t, m, "+")
	if m.Err() != nil || m.Len() != len(roots) {
		t.Fatalf("after retry: Err() = %v, Len() = %d", m.Err(), m.Len())
	}
}

func TestErrorText(t *testing.T) {
	offline := func(error) (string, string) { return "Can't reach GitHub", "r to retry" }
	tests := []struct {
		name       string
		opts       []Option
		root, hint string
		row        string
	}{
		{"default", nil, "✗ Couldn't load: boom", " · + to retry", "internal ✗ boom · + to retry"},
		{"custom", []Option{WithErrorText(offline)}, "✗ Can't reach GitHub", " · r to retry", "internal ✗ Can't reach GitHub · r to retry"},
		{"custom without a hint", []Option{WithErrorText(func(error) (string, string) { return "Not there.", "" })}, "✗ Not there.", "", "internal ✗ Not there."},
		{"empty", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, "", "", "internal"},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, "✗ GitHub says a · b", "", "internal ✗ GitHub says a · b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boom := errors.New("boom\nsecond line")
			f := repo()
			f.setFail("", boom)
			m := load(t, f, append([]Option{WithSize(60, 6)}, tt.opts...)...)
			text, hint := m.statusLine()
			if text, hint = ansi.Strip(text), ansi.Strip(hint); text != tt.root || hint != tt.hint {
				t.Errorf("statusLine() = %q, %q; want %q, %q", text, hint, tt.root, tt.hint)
			}

			f = repo()
			f.setFail("internal", boom)
			m = keys(t, load(t, f, append([]Option{WithSize(60, 6)}, tt.opts...)...), "j", "j", "+")
			if v := ansi.Strip(m.View()); !strings.Contains(v, tt.row+" ") || tt.row == "internal" && strings.Contains(v, "✗") {
				t.Errorf("View() = %q, want the row %q", v, tt.row)
			}
		})
	}
}

// The words of a failed load are asked for once, as it fails, not on
// every render.
func TestErrorTextWordedOnce(t *testing.T) {
	calls := 0
	say := func(error) (string, string) {
		calls++
		return "Can't reach GitHub", "r to retry"
	}
	f := repo()
	f.setFail("internal", errors.New("boom"))
	m := keys(t, load(t, f, WithSize(60, 6), WithErrorText(say)), "j", "j", "+")
	before := calls
	for range 3 {
		_ = m.View()
	}
	if before != 1 || calls != before {
		t.Errorf("asked for the words %d times as the load failed and %d more on render, want once and none", before, calls-before)
	}
}

func TestSetErrorText(t *testing.T) {
	f := repo()
	f.setFail("", errors.New("boom"))
	m := load(t, f)
	m.SetErrorText(func(error) (string, string) { return "Something went wrong", "r to retry" })
	if text, hint := m.statusLine(); ansi.Strip(text+hint) != "✗ Something went wrong · r to retry" {
		t.Errorf("statusLine() = %q, %q; want the new error text", text, hint)
	}
}

func TestEmpty(t *testing.T) {
	m := load(t, newFiles(), WithEmptyText("This repository is empty."))
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() on an empty tree returned a node")
	}
	if !strings.Contains(m.View(), "This repository is empty.") {
		t.Fatalf("View() = %q, want the empty text", m.View())
	}
	m.SetEmptyText("Nothing here.")
	if !strings.Contains(m.View(), "Nothing here.") {
		t.Fatal("SetEmptyText did not change the view")
	}
	// Keys on an empty tree do nothing.
	m = keys(t, m, "j", "+", "l", "-", "*", "=", "enter")
	if m.Len() != 0 {
		t.Fatal("keys changed an empty tree")
	}
}

func TestOpen(t *testing.T) {
	m := load(t, repo())
	m = keys(t, m, "G")
	_, cmd := m.Update(press("enter"))
	if cmd == nil {
		t.Fatal("enter on a leaf returned no command")
	}
	msg, ok := cmd().(OpenMsg)
	if !ok || msg.ID != m.ID() || msg.Node.ID != "go.mod" {
		t.Fatalf("enter on a leaf sent %#v, want OpenMsg for go.mod", msg)
	}
}

func TestIgnoresInvalidIDs(t *testing.T) {
	children := func(context.Context, Node) ([]Node, error) {
		return []Node{{ID: "a", Name: "a"}, {Name: "no id"}, {ID: "a", Name: "again"}, {ID: "b", Name: "b"}}, nil
	}
	m := newModel(children, WithSize(20, 5))
	m = run(t, m, m.Init())
	if got := rowIDs(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("rows = %v, want [a b]", got)
	}
}

// generated returns a tree under the single top-level directory "d", in
// which every directory down to depth levels holds two directories and a
// file.
func generated(depth int) *files {
	f := newFiles()
	var gen func(dir string, level int)
	gen = func(dir string, level int) {
		if level == depth {
			f.add(dir + "/")
			return
		}
		f.add(dir + "/f")
		for i := range 2 {
			gen(fmt.Sprintf("%s/%d", dir, i), level+1)
		}
	}
	gen("d", 0)
	return f
}

func TestExpandAllLimits(t *testing.T) {
	tests := []struct {
		name     string
		opts     []Option
		wantRows int
	}{
		// d, then 3, 6, 12 and 24 children per level.
		{"everything", nil, 1 + 3 + 6 + 12 + 24},
		{"one level", []Option{WithExpandAllLimits(1000, 1)}, 1 + 3},
		{"two levels", []Option{WithExpandAllLimits(1000, 2)}, 1 + 3 + 6},
		{"node budget spent at once", []Option{WithExpandAllLimits(3, 10)}, 1 + 3},
		// Branches expanded before the budget ran out still finish.
		{"node budget spent later", []Option{WithExpandAllLimits(4, 10)}, 1 + 3 + 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, generated(4), append(tt.opts, WithSize(40, 60))...)
			m = keys(t, m, "*")
			if m.Len() != tt.wantRows {
				t.Fatalf("Len() = %d, want %d", m.Len(), tt.wantRows)
			}
			if m.bulk.active() {
				t.Fatal("expand-all still in progress")
			}
		})
	}
}

func TestSetExpandAllLimits(t *testing.T) {
	m := load(t, generated(4), WithSize(40, 60))
	m.SetExpandAllLimits(1000, 2)
	if n, d := m.ExpandAllLimits(); n != 1000 || d != 2 {
		t.Fatalf("ExpandAllLimits() = %d, %d; want 1000, 2", n, d)
	}
	if m = keys(t, m, "*"); m.Len() != 1+3+6 {
		t.Errorf("Len() = %d, want two levels", m.Len())
	}
	m.SetExpandAllLimits(0, -1)
	if n, d := m.ExpandAllLimits(); n != 1 || d != 1 {
		t.Errorf("ExpandAllLimits() = %d, %d; want both clamped to 1", n, d)
	}
}

// collect runs cmd and returns the messages it produces, without delivering
// them. Spinner ticks are dropped.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, collect(c)...)
		}
		return msgs
	case spinner.TickMsg:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func TestExpandAllBoundsLoadsInFlight(t *testing.T) {
	f := generated(3)
	m := load(t, f, WithMaxLoads(2))
	m, cmd := m.Update(press("*"))
	// Results not yet delivered stand for loads in flight.
	pending := collect(cmd)
	peak := len(pending)
	for len(pending) > 0 {
		msg := pending[0]
		pending = pending[1:]
		m, cmd = m.Update(msg)
		pending = append(pending, collect(cmd)...)
		peak = max(peak, len(pending))
	}
	if peak > 2 {
		t.Fatalf("%d loads in flight at once, want at most 2", peak)
	}
	if want := 1 + 3 + 6 + 12; m.Len() != want {
		t.Fatalf("Len() = %d, want %d", m.Len(), want)
	}
}

func TestExpandAllLevelByLevel(t *testing.T) {
	f := generated(3)
	m := load(t, f, WithMaxLoads(1))
	keys(t, m, "*")
	dirs := slices.Clone(f.calls[1:])
	want := []string{"d", "d/0", "d/1", "d/0/0", "d/0/1", "d/1/0", "d/1/1",
		"d/0/0/0", "d/0/0/1", "d/0/1/0", "d/0/1/1", "d/1/0/0", "d/1/0/1", "d/1/1/0", "d/1/1/1"}
	if !slices.Equal(dirs, want) {
		t.Fatalf("load order = %v, want %v", dirs, want)
	}
}

func TestCollapseAllStopsExpandAll(t *testing.T) {
	m := load(t, generated(4), WithMaxLoads(1))
	m, cmd := m.Update(press("*"))
	m, _ = m.Update(press("="))
	m = run(t, m, cmd)
	if m.bulk.active() || m.Len() != 1 {
		t.Fatalf("Len() = %d, expand-all active = %v; want 1, false", m.Len(), m.bulk.active())
	}
}

func TestReloadKeepsState(t *testing.T) {
	f := repo()
	m := load(t, f)
	m = keys(t, m, "l", "-", "j", "j", "l", "l", "l", "j", "j")
	if selectedID(m) != "internal/core/repo.go" {
		t.Fatalf("Selected() = %q", selectedID(m))
	}
	before := rowIDs(m)

	f.add("internal/core/issue.go")
	f.remove("internal/tui")
	cmd := m.Reload()
	if got := rowIDs(m); !slices.Equal(got, before) {
		t.Fatalf("rows changed before the reload arrived: %v", got)
	}
	m = run(t, m, cmd)
	want := []string{"cmd", "docs", "internal", "internal/core", "internal/core/issue.go",
		"internal/core/pull.go", "internal/core/repo.go", "README.md", "go.mod"}
	if got := rowIDs(m); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if selectedID(m) != "internal/core/repo.go" {
		t.Fatalf("Selected() = %q after Reload, want internal/core/repo.go", selectedID(m))
	}
	// cmd was collapsed, so it forgot its children and loads on expand.
	if f.callCount("cmd") != 1 {
		t.Fatal("Reload loaded a collapsed branch")
	}
	m = keys(t, m, "g", "+")
	if f.callCount("cmd") != 2 {
		t.Fatal("a collapsed branch did not load again after Reload")
	}
}

func TestReloadUpdatesDetail(t *testing.T) {
	f := sized()
	m := load(t, f, WithSize(40, 6))
	f.setDetail("go.mod", "9.9K")
	m = run(t, m, m.Reload())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "9.9K") || strings.Contains(v, "1.2K") {
		t.Errorf("view = %q, want the new detail of go.mod", v)
	}
}

func TestReloadMovesCursorUpWhenNodeIsGone(t *testing.T) {
	f := repo()
	m := load(t, f)
	m = keys(t, m, "j", "j", "*", "G", "k", "k")
	if selectedID(m) != "internal/tui/app.go" {
		t.Fatalf("Selected() = %q", selectedID(m))
	}
	f.remove("internal/tui")
	m = run(t, m, m.Reload())
	if selectedID(m) != "internal" {
		t.Fatalf("Selected() = %q, want internal", selectedID(m))
	}
}

func TestReloadNodeMovesChild(t *testing.T) {
	moved := false
	children := func(_ context.Context, parent Node) ([]Node, error) {
		x := []Node{{ID: "x", Name: "x"}}
		switch parent.ID {
		case "":
			return []Node{{ID: "a", Name: "a", Branch: true}, {ID: "b", Name: "b", Branch: true}}, nil
		case "a":
			if !moved {
				return x, nil
			}
		case "b":
			if moved {
				return x, nil
			}
		}
		return nil, nil
	}
	m := newModel(children, WithSize(20, 5), WithFocused(true))
	m = run(t, m, m.Init())
	m = keys(t, m, "+", "j", "j", "+")
	if got := rowIDs(m); !slices.Equal(got, []string{"a", "x", "b"}) {
		t.Fatalf("rows = %v", got)
	}
	moved = true
	m = run(t, m, m.ReloadNode("b"))
	if got := rowIDs(m); !slices.Equal(got, []string{"a", "b", "x"}) {
		t.Fatalf("rows = %v, want [a b x]", got)
	}
	if m.nodes["x"].parent != "b" {
		t.Fatal("x did not move to b")
	}
	if cmd := m.ReloadNode("nope"); cmd != nil {
		t.Fatal("ReloadNode of an unknown node returned a command")
	}
}

func TestReset(t *testing.T) {
	var ctxs []context.Context
	rev := "a"
	children := func(ctx context.Context, _ Node) ([]Node, error) {
		ctxs = append(ctxs, ctx)
		return []Node{{ID: rev, Name: rev}}, nil
	}
	m := newModel(children, WithSize(20, 5), WithFocused(true))
	stale := m.Init()
	rev = "b"
	cmd := m.Reset()
	m = run(t, m, stale)
	if m.Len() != 0 {
		t.Fatal("a result from before Reset was kept")
	}
	if ctxs[0].Err() == nil {
		t.Fatal("Reset should cancel the loads in flight")
	}
	m = run(t, m, cmd)
	if selectedID(m) != "b" {
		t.Fatalf("Selected() = %q, want b", selectedID(m))
	}
}

func TestDropsStaleResults(t *testing.T) {
	f := repo()
	m := load(t, f)
	m, first := m.Update(press("+"))
	second := m.ReloadNode("cmd")
	m = run(t, m, first)
	if !m.nodes["cmd"].loading {
		t.Fatal("a superseded load was accepted")
	}
	m = run(t, m, second)
	if m.nodes["cmd"].loading || m.Len() != len(roots)+1 {
		t.Fatalf("rows = %v", rowIDs(m))
	}
}

func TestIgnoresOtherInstances(t *testing.T) {
	a := load(t, repo())
	b := newModel(repo().children)
	if a.ID() == b.ID() {
		t.Fatal("two trees share an ID")
	}
	for _, msg := range collect(b.Init()) {
		a2, cmd := a.Update(msg)
		if cmd != nil || a2.Len() != a.Len() {
			t.Fatal("tree reacted to another tree's message")
		}
	}
	if _, cmd := a.Update(spinner.TickMsg{ID: -1}); cmd != nil {
		t.Fatal("tree reacted to another spinner's tick")
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := load(t, repo(), WithFocused(false))
	m = keys(t, m, "j", "+")
	if m.Index() != 0 || m.Len() != len(roots) {
		t.Fatal("a blurred tree reacted to keys")
	}
	m.Focus()
	if m = keys(t, m, "j"); m.Index() != 1 {
		t.Fatal("Focus did not make the tree react to keys")
	}
	m.Blur()
	if m.Focused() {
		t.Fatal("Focused() = true after Blur")
	}
}

func TestScrollOff(t *testing.T) {
	paths := make([]string, 30)
	for i := range paths {
		paths[i] = fmt.Sprintf("f%02d", i)
	}
	tests := []struct {
		name    string
		opts    []Option
		keys    []string
		wantSel int
		wantTop int
	}{
		{"default keeps two rows below", nil, []string{"j", "j", "j", "j", "j", "j", "j", "j"}, 8, 1},
		{"default keeps two rows above", nil, []string{"G", "k", "k", "k", "k", "k", "k", "k", "k"}, 21, 19},
		{"none", []Option{WithScrollOff(0)}, []string{"j", "j", "j", "j", "j", "j", "j", "j"}, 8, 0},
		{"end", nil, []string{"G"}, 29, 20},
		{"half the window at most", []Option{WithScrollOff(50)}, []string{"j", "j", "j", "j", "j", "j"}, 6, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, newFiles(paths...), tt.opts...)
			m = keys(t, m, tt.keys...)
			if m.Index() != tt.wantSel || m.top != tt.wantTop {
				t.Fatalf("Index() = %d, top = %d; want %d, %d", m.Index(), m.top, tt.wantSel, tt.wantTop)
			}
			assertVisible(t, m)
		})
	}
}

func TestResize(t *testing.T) {
	paths := make([]string, 30)
	for i := range paths {
		paths[i] = fmt.Sprintf("f%02d", i)
	}
	m := load(t, newFiles(paths...))
	m = keys(t, m, "G")
	m.SetSize(40, 4)
	assertVisible(t, m)
	m.SetSize(40, 50)
	if m.top != 0 || m.Width() != 40 || m.Height() != 50 {
		t.Fatalf("top = %d, size = %dx%d after growing", m.top, m.Width(), m.Height())
	}
}

func TestSpinnerStopsWhenLoaded(t *testing.T) {
	m := newModel(repo().children, WithSize(40, 5))
	tick := m.spin.Tick()
	m, cmd := m.Update(tick)
	if cmd == nil {
		t.Fatal("spinner should keep ticking while loading")
	}
	m = run(t, m, m.Init())
	if m, cmd = m.Update(tick); cmd != nil || m.spinning {
		t.Fatal("spinner should stop once nothing is loading")
	}
	// A new load starts it again.
	m.Focus()
	if _, cmd = m.Update(press("+")); len(collect(cmd)) != 1 || cmd == nil {
		t.Fatal("expanding should load")
	}
}

func TestAccessors(t *testing.T) {
	m := load(t, repo())
	k := testKeyMap
	k.Expand.SetKeys("o")
	k.Expand.SetHelp("o", "expand")
	m.SetKeyMap(k)
	m = keys(t, m, "o")
	if m.Len() != len(roots)+1 {
		t.Fatal("custom key map was not applied")
	}
	if !strings.Contains(m.errHint, "o to retry") {
		t.Fatalf("retry hint %q should follow the key map", m.errHint)
	}
	st := DefaultStyles(false)
	m.SetStyles(st)
	if m.Styles().Cursor.GetForeground() != st.Cursor.GetForeground() {
		t.Fatal("Styles() did not return the styles set")
	}
	if len(m.KeyMap().ShortHelp()) == 0 || len(m.KeyMap().FullHelp()) == 0 {
		t.Fatal("key map has no help")
	}
}

// TestRetry checks that Retry loads again the top-level nodes if they
// failed, else each open branch that failed, and nothing when none did.
func TestRetry(t *testing.T) {
	f := repo()
	f.setFail("", errors.New("offline"))
	m := load(t, f)
	f.setFail("", nil)
	m = run(t, m, m.Retry())
	if m.Err() != nil || m.Len() != len(roots) {
		t.Fatalf("after Retry: Err() = %v, Len() = %d", m.Err(), m.Len())
	}

	f.setFail("internal", errors.New("offline"))
	m.SetSize(60, 10)
	m = keys(t, m, "j", "j", "+")
	f.setFail("internal", nil)
	m = run(t, m, m.Retry())
	if e := m.nodes["internal"]; e.err != nil || len(e.kids) != 2 {
		t.Fatalf("branch after Retry: err = %v, kids = %v", e.err, e.kids)
	}
	if cmd := m.Retry(); cmd != nil {
		t.Error("Retry with nothing failed returned a command")
	}
}

func TestAt(t *testing.T) {
	m := newModel(repo().children, WithSize(40, 10))
	m = run(t, m, m.Init())
	for i, id := range roots {
		if n, ok := m.At(i); !ok || n.ID != id {
			t.Errorf("At(%d) = %q, %v, want %q", i, n.ID, ok, id)
		}
	}
	for _, i := range []int{-1, len(roots)} {
		if n, ok := m.At(i); ok {
			t.Errorf("At(%d) = %q, want no row", i, n.ID)
		}
	}
}

// The keys that once paged do nothing in a tree whose cursor is halfway
// down; each is tried on its own.
func TestOldPagingKeysUnbound(t *testing.T) {
	for _, k := range []string{"b", "d", "u", "f"} {
		t.Run(k, func(t *testing.T) {
			m := load(t, repo())
			m = keys(t, m, "down", "down", "down")
			sel, top := selectedID(m), m.top
			m = keys(t, m, k)
			if selectedID(m) != sel || m.top != top {
				t.Fatalf("after %q: Selected() = %q, top = %d; want %q, %d", k, selectedID(m), m.top, sel, top)
			}
		})
	}
}
