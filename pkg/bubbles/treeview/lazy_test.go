package treeview

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// kid makes a child row, relative to its parent, whose text is its ID.
func kid(id string, branch bool) Node {
	return Node{ID: id, Lines: []string{id}, Branch: branch}
}

// world is the tree the lazy tests load from, and the loads it served.
type world struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]error
}

func (w *world) load(_ context.Context, node Node) ([]Node, error) {
	parent := node.ID
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, parent)
	if err := w.fail[parent]; err != nil {
		return nil, err
	}
	switch parent {
	case "":
		return []Node{kid("a", true), kid("b", false)}, nil
	case "a":
		return []Node{kid("a1", false), kid("a2", true)}, nil
	case "a2":
		return []Node{kid("x", false)}, nil
	}
	return nil, nil
}

func (w *world) count(parent string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.calls {
		if c == parent {
			n++
		}
	}
	return n
}

// run executes cmd and feeds the messages it makes back to m, leaving spinner
// ticks out.
func run(m Model, cmd tea.Cmd) Model {
	for cmd != nil {
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				m = run(m, c)
			}
			return m
		case tickMsg, nil:
			return m
		default:
			m, cmd = m.Update(msg)
		}
	}
	return m
}

func lazyTree(w *world, opts ...Option) Model {
	opts = append([]Option{WithKeyMap(testKeyMap), WithFocused(true), WithSize(30, 10)}, opts...)
	m := New(Lazy(w.load), opts...)
	return run(m, m.Init())
}

// press runs one key and what it loads.
func pressRun(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		m = run(m, cmd)
	}
	return m
}

func TestInitNilWhenNothingLoads(t *testing.T) {
	for name, src := range map[string]Source{
		"empty": {},
		"rows":  Rows([]Node{leaf("a", 0)}),
	} {
		if cmd := New(src).Init(); cmd != nil {
			t.Errorf("%s: Init() is not nil", name)
		}
	}
	if New(Lazy((&world{}).load)).Init() == nil {
		t.Error("a lazy source: Init() is nil")
	}
}

func TestCachedIsSynchronous(t *testing.T) {
	m := New(Rows([]Node{leaf("a", 0), leaf("b", 0)}))
	if got := visible(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("rows = %v before any Update", got)
	}
}

func TestLoadsTopLevelThenOnExpand(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	if got := visible(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("top = %v", got)
	}
	m = pressRun(t, m, "l")
	if got := visible(m); !slices.Equal(got, []string{"a", "a1", "a2", "b"}) {
		t.Fatalf("after expand = %v", got)
	}
	m = pressRun(t, m, "h", "l")
	if n := w.count("a"); n != 1 {
		t.Errorf("a loaded %d times, want 1", n)
	}
	if m.Loading() {
		t.Error("still loading")
	}
}

func TestEnterNeverToggles(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m, msgs := keys(t, m, "enter")
	if len(msgs) != 1 || !m.Folded("a") || w.count("a") != 0 {
		t.Errorf("enter changed a fold or loaded: %v, folded %v", msgs, m.Folded("a"))
	}
}

func TestErrorInlineAndRetry(t *testing.T) {
	w := &world{fail: map[string]error{"a": errors.New("boom")}}
	m := lazyTree(w)
	m = pressRun(t, m, "l")
	if m.Err("a") == nil {
		t.Fatal("no error recorded for a")
	}
	if got := lines(m)[0]; !contains(got, "boom") {
		t.Errorf("row = %q, want the error inline", got)
	}
	delete(w.fail, "a")
	m = pressRun(t, m, "l")
	if m.Err("a") != nil || !slices.Contains(visible(m), "a1") {
		t.Errorf("retry: err %v, rows %v", m.Err("a"), visible(m))
	}
}

func TestTopLevelError(t *testing.T) {
	w := &world{fail: map[string]error{"": errors.New("offline")}}
	m := lazyTree(w, WithErrorText(func(err error) (string, string) { return "! " + err.Error(), "press r" }))
	if got := lines(m)[0]; !contains(got, "! offline") {
		t.Errorf("view = %q", got)
	}
	if got := lines(m)[1]; !contains(got, "press r") {
		t.Errorf("hint line = %q", got)
	}
}

func contains(s, sub string) bool { return sub == "" || indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestStaleResultDropped(t *testing.T) {
	w := &world{}
	m := New(Lazy(w.load), WithSize(30, 5))
	old := m.Init()
	next := m.Reset()
	if next == nil {
		t.Fatal("Reset has no load")
	}
	// The result of the load before the reset arrives after it.
	stale := old()
	if bm, ok := stale.(tea.BatchMsg); ok {
		for _, c := range bm {
			if msg, ok := c().(childrenMsg); ok {
				stale = msg
			}
		}
	}
	msg, ok := stale.(childrenMsg)
	if !ok {
		t.Fatalf("no result of the first load, got %T", stale)
	}
	m, _ = m.Update(msg)
	if m.Len() != 0 {
		t.Errorf("a stale result was applied: %v", visible(m))
	}
	m = run(m, next)
	if got := visible(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("rows = %v", got)
	}
}

func TestResetCancelsLoad(t *testing.T) {
	started := make(chan context.Context, 1)
	done := make(chan struct{})
	load := func(ctx context.Context, _ Node) ([]Node, error) {
		started <- ctx
		<-ctx.Done()
		close(done)
		return nil, ctx.Err()
	}
	m := New(Lazy(load))
	cmd := m.Init()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init does not batch the load with the spinner")
	}
	for _, c := range batch {
		go c()
	}
	ctx := <-started
	m.Reset()
	<-done
	if ctx.Err() == nil {
		t.Error("the load's context is not cancelled")
	}
}

func TestWithContextCancelsLoads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan context.Context, 1)
	load := func(c context.Context, _ Node) ([]Node, error) {
		got <- c
		return nil, nil
	}
	m := New(Lazy(load), WithContext(ctx))
	run(m, m.Init())
	cancel()
	if (<-got).Err() == nil {
		t.Error("cancelling the tree's context did not cancel the load")
	}
}

func TestReloadKeepsRows(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(t, m, "l")
	cmd := m.Reload()
	if got := visible(m); !slices.Equal(got, []string{"a", "a1", "a2", "b"}) {
		t.Errorf("rows dropped while reloading: %v", got)
	}
	m = run(m, cmd)
	if w.count("") != 2 {
		t.Errorf("top loaded %d times, want 2", w.count(""))
	}
	if !slices.Contains(visible(m), "a1") {
		t.Errorf("open subtree lost on reload: %v", visible(m))
	}
}

func TestReloadNode(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(t, m, "l")
	m = run(m, m.ReloadNode("a"))
	if w.count("a") != 2 {
		t.Errorf("a loaded %d times, want 2", w.count("a"))
	}
}

func TestExpandAllLoadsEverything(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(t, m, "*")
	if got := visible(m); !slices.Equal(got, []string{"a", "a1", "a2", "x", "b"}) {
		t.Errorf("rows = %v", got)
	}
	if m.Capped() {
		t.Error("capped without a limit")
	}
}

func TestExpandAllLimits(t *testing.T) {
	w := &world{}
	m := lazyTree(w, WithExpandAllLimits(2, 16))
	m = pressRun(t, m, "*")
	if !m.Capped() {
		t.Errorf("not capped, rows %v", visible(m))
	}
	if w.count("a2") != 0 {
		t.Error("loaded past the node limit")
	}
}

func TestExpandAllMaxLoads(t *testing.T) {
	var live, peak atomic.Int32
	load := func(_ context.Context, parent Node) ([]Node, error) {
		n := live.Add(1)
		defer live.Add(-1)
		peak.Store(max(peak.Load(), n))
		if parent.ID != "" {
			return nil, nil
		}
		kids := make([]Node, 6)
		for i := range kids {
			kids[i] = kid(fmt.Sprint("k", i), true)
		}
		return kids, nil
	}
	m := New(Lazy(load), WithMaxLoads(2), WithKeyMap(testKeyMap), WithFocused(true), WithSize(20, 10))
	m = run(m, m.Init())
	// The commands run one after another, so the limit shows in the loads
	// started, not in the loads that overlap.
	m, cmd := m.Update(press("*"))
	if n := len(m.lz.busy); n != 2 {
		t.Fatalf("%d loads started by expand-all, want 2", n)
	}
	m = run(m, cmd)
	if len(m.lz.busy) != 0 || len(m.lz.bulk.queue) != 0 {
		t.Errorf("expand-all left work: busy %d, queued %d", len(m.lz.busy), len(m.lz.bulk.queue))
	}
	if peak.Load() > 2 {
		t.Errorf("%d loads at once, want at most 2", peak.Load())
	}
}

func TestReloadKeepsToMaxLoads(t *testing.T) {
	load := func(_ context.Context, parent Node) ([]Node, error) {
		if parent.ID != "" {
			return []Node{kid(parent.ID+"-c", false)}, nil
		}
		kids := make([]Node, 6)
		for i := range kids {
			kids[i] = kid(fmt.Sprint("k", i), true)
		}
		return kids, nil
	}
	m := New(Lazy(load), WithMaxLoads(2), WithKeyMap(testKeyMap), WithFocused(true), WithSize(20, 20))
	m = run(m, m.Init())
	m = pressRun(t, m, "*")
	cmd := m.Reload()
	if n := len(m.lz.busy); n != 2 {
		t.Fatalf("%d loads started by Reload, want 2", n)
	}
	m = run(m, cmd)
	if len(m.lz.busy) != 0 || len(m.lz.reload) != 0 {
		t.Errorf("reload left work: busy %d, queued %d", len(m.lz.busy), len(m.lz.reload))
	}
	if len(visible(m)) != 12 {
		t.Errorf("rows = %v", visible(m))
	}
}

func TestLoadGetsParentNode(t *testing.T) {
	var got []Node
	load := func(_ context.Context, parent Node) ([]Node, error) {
		got = append(got, parent)
		if parent.ID == "" {
			return []Node{{ID: "a", Link: "u", Lines: []string{"a"}, Branch: true}}, nil
		}
		return nil, nil
	}
	m := New(Lazy(load), WithKeyMap(testKeyMap), WithFocused(true), WithSize(20, 5))
	m = run(m, m.Init())
	pressRun(t, m, "l")
	if len(got) != 2 || got[0].ID != "" || got[1].ID != "a" || got[1].Link != "u" {
		t.Errorf("parents = %+v", got)
	}
}

func TestReloadOutOfOrder(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(t, m, "*")
	cmd := m.Reload()
	var msgs []childrenMsg
	var collect func(tea.Cmd)
	collect = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				collect(c)
			}
		case childrenMsg:
			msgs = append(msgs, msg)
		}
	}
	collect(cmd)
	// With the load limit, only some results exist at first; feed what there
	// is, deepest first, then the rest as they are asked for.
	for len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		msgs = msgs[:len(msgs)-1]
		var next tea.Cmd
		m, next = m.Update(last)
		collect(next)
	}
	if got := visible(m); !slices.Equal(got, []string{"a", "a1", "a2", "x", "b"}) {
		t.Errorf("rows after out-of-order reload = %v", got)
	}
	if m.Loading() {
		t.Error("still loading")
	}
}

func TestRevealFailedLoadClearsGoal(t *testing.T) {
	w := &world{fail: map[string]error{"a": errors.New("boom")}}
	m := lazyTree(w)
	ok, cmd := m.Reveal("a", "a2", "x")
	if !ok {
		t.Fatal("Reveal refused a path that may come")
	}
	m = run(m, cmd)
	if m.lz.goal != nil {
		t.Errorf("goal kept after a failed load: %v", m.lz.goal)
	}
	if m.Err("a") == nil {
		t.Error("no error on the failed row")
	}
	if ok, _ := m.Reveal("a", "a2", "x"); ok && m.lz.goal == nil {
		t.Error("Reveal reports a path that is not there")
	}
}

func TestRevealStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	load := func(c context.Context, parent Node) ([]Node, error) {
		calls.Add(1)
		if parent.ID == "" {
			return []Node{kid("a", true)}, nil
		}
		<-c.Done()
		return nil, c.Err()
	}
	m := New(Lazy(load), WithContext(ctx), WithKeyMap(testKeyMap), WithSize(20, 5))
	m = run(m, m.Init())
	_, cmd := m.Reveal("a", "b")
	cancel()
	m = run(m, cmd)
	if m.lz.goal != nil {
		t.Errorf("goal kept after cancel: %v", m.lz.goal)
	}
	before := calls.Load()
	m = run(m, m.fetch(0))
	if calls.Load() != before {
		t.Error("a load started on a cancelled context")
	}
	if m.Loading() {
		t.Error("still loading")
	}
}

func TestSetFoldedLoads(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	cmd := m.SetFolded("a", false)
	if cmd == nil {
		t.Fatal("opening an unloaded branch returns no load")
	}
	m = run(m, cmd)
	if w.count("a") != 1 || !slices.Contains(visible(m), "a1") {
		t.Errorf("rows = %v, loads of a = %d", visible(m), w.count("a"))
	}
}

func TestVanishedRowCancelsItsLoad(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(t, m, "l")
	cmd := m.ReloadNode("a2")
	if _, busy := m.lz.busy["a2"]; !busy {
		t.Fatal("no load in flight for a2")
	}
	m.SetRows([]Node{kid("b", false)})
	if m.Loading() {
		t.Error("a load of a row that is gone is still in flight")
	}
	_ = cmd
}

func TestSetSourceClearsBoot(t *testing.T) {
	w := &world{}
	m := New(Lazy(w.load))
	m.SetSource(Rows([]Node{kid("z", false)}))
	if m.Init() != nil {
		t.Error("Init reruns the load of the old source")
	}
}

func TestRevealUnloadedPath(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	ok, cmd := m.Reveal("a", "a2", "x")
	if !ok || cmd == nil {
		t.Fatalf("Reveal = %v, cmd %v; want a path that is loading", ok, cmd != nil)
	}
	m = run(m, cmd)
	if selected(m) != "x" {
		t.Errorf("cursor on %q, want x; rows %v", selected(m), visible(m))
	}
	if ok, _ := m.Reveal("a", "nope"); ok {
		t.Error("revealed a row that is not there")
	}
}

func TestFilter(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(t, m, "*")
	m.SetFilter("a2")
	got := visible(m)
	if !slices.Contains(got, "a2") || slices.Contains(got, "b") {
		t.Errorf("filtered rows = %v", got)
	}
	if m.Filter() != "a2" {
		t.Errorf("Filter() = %q", m.Filter())
	}
	m.SetFilter("")
	if len(visible(m)) != 5 {
		t.Errorf("cleared filter rows = %v", visible(m))
	}
}

func TestIconsAndEmpty(t *testing.T) {
	m := New(Rows([]Node{leaf("a", 0)}), WithSize(20, 3), WithIcons(func(Node) string { return "#" }))
	if got := lines(m)[0]; !contains(got, "# a") {
		t.Errorf("row = %q, want an icon", got)
	}
	e := New(Rows(nil), WithSize(20, 3), WithEmpty("no files", "press a"))
	if got := lines(e)[0]; !contains(got, "no files") {
		t.Errorf("empty = %q", got)
	}
	if got := lines(e)[1]; !contains(got, "press a") {
		t.Errorf("empty hint = %q", got)
	}
}

func TestSpinnerWhileLoading(t *testing.T) {
	m := New(Lazy((&world{}).load), WithSize(20, 3))
	if got := lines(m)[0]; !contains(got, "Loading") {
		t.Errorf("loading view = %q", got)
	}
	if !m.Loading() {
		t.Error("Loading() = false")
	}
}

func TestSetSourceReplacesRows(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = run(m, m.SetSource(Rows([]Node{leaf("z", 0)})))
	if got := visible(m); !slices.Equal(got, []string{"z"}) {
		t.Errorf("rows = %v", got)
	}
}

func BenchmarkLazyExpandAll(b *testing.B) {
	w := &world{}
	b.ReportAllocs()
	for b.Loop() {
		m := lazyTree(w)
		m = pressRun(&testing.T{}, m, "*")
		_ = m.View()
	}
}

func BenchmarkLazyUpdate(b *testing.B) {
	w := &world{}
	m := lazyTree(w)
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(press("j"))
		m, _ = m.Update(press("k"))
	}
}

func BenchmarkLazyView(b *testing.B) {
	w := &world{}
	m := lazyTree(w)
	m = pressRun(&testing.T{}, m, "*")
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
