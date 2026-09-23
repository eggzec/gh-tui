package feed

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type item struct {
	id    string
	title string
}

func renderItem(it item, _ bool, _ int) string {
	return "#" + it.id + " " + it.title
}

// source serves items in chunks of size, with the start index as cursor.
type source struct {
	mu    sync.Mutex
	items []item
	size  int
	fail  map[string]error
	calls []string
}

func newSource(n, size int) *source {
	s := &source{size: size, fail: map[string]error{}}
	for i := range n {
		s.items = append(s.items, item{id: strconv.Itoa(i), title: fmt.Sprintf("item %d", i)})
	}
	return s
}

func (s *source) fetch(_ context.Context, cursor string) ([]item, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, cursor)
	if err := s.fail[cursor]; err != nil {
		return nil, "", err
	}
	start := 0
	if cursor != "" {
		start, _ = strconv.Atoi(cursor)
	}
	end := min(start+s.size, len(s.items))
	next := ""
	if end < len(s.items) {
		next = strconv.Itoa(end)
	}
	return append([]item(nil), s.items[start:end]...), next, nil
}

func (s *source) setFail(cursor string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.fail, cursor)
		return
	}
	s.fail[cursor] = err
}

func (s *source) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// run executes cmd and feeds every resulting message back into m, until no
// commands are left. Spinner ticks are dropped so tests never sleep.
func run[T any](tb testing.TB, m Model[T], cmd tea.Cmd) Model[T] {
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

// load builds a focused feed over src and fetches its first chunk.
func load(tb testing.TB, src *source, opts ...Option) Model[item] {
	tb.Helper()
	opts = append([]Option{WithSize(40, 5), WithFocused(true)}, opts...)
	m := New(src.fetch, renderItem, opts...)
	return run(tb, m, m.Init())
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// keys presses each key and runs the resulting commands.
func keys[T any](tb testing.TB, m Model[T], ks ...string) Model[T] {
	tb.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		m = run(tb, m, cmd)
	}
	return m
}

func assertVisible[T any](tb testing.TB, m Model[T]) {
	tb.Helper()
	if m.sel < m.top || m.sel >= m.top+m.slots() {
		tb.Fatalf("selection %d outside window [%d, %d)", m.sel, m.top, m.top+m.slots())
	}
}

func TestInitLoadsFirstChunk(t *testing.T) {
	src := newSource(100, 10)
	m := New(src.fetch, renderItem, WithSize(40, 5))
	if text, _ := m.statusLine(); !m.hasStatus() || text == m.emptyLine {
		t.Fatal("new feed should show the loading row")
	}
	m = run(t, m, m.Init())
	if got := m.Len(); got != 10 {
		t.Fatalf("Len() = %d, want 10", got)
	}
	if it, ok := m.Selected(); !ok || it.id != "0" {
		t.Fatalf("Selected() = %v, %v; want item 0", it, ok)
	}
	if got := src.callCount(); got != 1 {
		t.Fatalf("fetched %d times, want 1", got)
	}
}

func TestNavigation(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		wantSel int
		wantTop int
	}{
		{"down", []string{"down"}, 1, 0},
		{"j", []string{"j", "j"}, 2, 0},
		{"up stops at start", []string{"up", "k"}, 0, 0},
		{"down scrolls", []string{"down", "down", "down", "down", "down"}, 5, 1},
		{"up scrolls back", []string{"pgdown", "pgdown", "up", "up", "up", "up", "up", "up"}, 3, 3},
		{"page down", []string{"pgdown"}, 5, 1},
		{"page down stops at end", []string{"f", "f"}, 9, 5},
		{"page up", []string{"pgdown", "pgdown", "pgup"}, 4, 4},
		{"page up stops at start", []string{"pgdown", "b", "b"}, 0, 0},
		{"end", []string{"end"}, 9, 5},
		{"home", []string{"end", "home"}, 0, 0},
		{"g", []string{"G", "g"}, 0, 0},
		{"unbound key", []string{"x"}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// One chunk of 10 with nothing after it keeps the arithmetic plain.
			m := load(t, newSource(10, 10))
			m = keys(t, m, tt.keys...)
			if m.Index() != tt.wantSel || m.top != tt.wantTop {
				t.Fatalf("Index() = %d, top = %d; want %d, %d", m.Index(), m.top, tt.wantSel, tt.wantTop)
			}
			assertVisible(t, m)
		})
	}
}

func TestSelectionStaysVisible(t *testing.T) {
	for _, h := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("item height %d", h), func(t *testing.T) {
			m := load(t, newSource(50, 7), WithItemHeight(h), WithSize(40, 7))
			for _, k := range []string{"down", "pgdown", "pgdown", "end", "up", "pgup", "home", "end"} {
				m = keys(t, m, k)
				assertVisible(t, m)
			}
		})
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := load(t, newSource(10, 10), WithFocused(false))
	m = keys(t, m, "down", "end")
	if m.Index() != 0 {
		t.Fatalf("Index() = %d, want 0", m.Index())
	}
	m.Focus()
	m = keys(t, m, "down")
	if m.Index() != 1 {
		t.Fatalf("Index() = %d after Focus, want 1", m.Index())
	}
	m.Blur()
	if m.Focused() {
		t.Fatal("Focused() = true after Blur")
	}
}

func TestFetchesNextChunk(t *testing.T) {
	tests := []struct {
		name    string
		height  int
		keys    []string
		wantLen int
	}{
		{"first chunk fills the window", 5, nil, 10},
		{"short chunks fill the window", 12, nil, 20},
		{"end of chunk", 5, []string{"end"}, 20},
		{"end of feed", 5, []string{"end", "end", "end", "end"}, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, newSource(25, 10), WithSize(40, tt.height))
			m = keys(t, m, tt.keys...)
			if m.Len() != tt.wantLen {
				t.Fatalf("Len() = %d, want %d", m.Len(), tt.wantLen)
			}
		})
	}
}

func TestDoneStopsFetching(t *testing.T) {
	src := newSource(15, 10)
	m := load(t, src)
	m = keys(t, m, "end", "end", "down", "end")
	if !m.Done() || m.Len() != 15 {
		t.Fatalf("Done() = %v, Len() = %d; want true, 15", m.Done(), m.Len())
	}
	if got := src.callCount(); got != 2 {
		t.Fatalf("fetched %d times, want 2", got)
	}
}

func TestErrorAndRetry(t *testing.T) {
	boom := errors.New("boom\nsecond line")
	src := newSource(10, 10)
	src.setFail("", boom)
	m := load(t, src)
	if !errors.Is(m.Err(), boom) {
		t.Fatalf("Err() = %v, want %v", m.Err(), boom)
	}
	if !m.KeyMap().Retry.Enabled() {
		t.Fatal("retry should be enabled after an error")
	}
	text, hint := m.statusLine()
	if strings.Contains(text, "second line") {
		t.Fatal("error row should show only the first line")
	}
	if !strings.Contains(hint, "r to retry") {
		t.Fatalf("error hint %q should mention the retry key", hint)
	}

	src.setFail("", nil)
	m = keys(t, m, "r")
	if m.Err() != nil || m.Len() != 10 {
		t.Fatalf("after retry: Err() = %v, Len() = %d", m.Err(), m.Len())
	}
	if m.KeyMap().Retry.Enabled() {
		t.Fatal("retry should be disabled once the fetch succeeds")
	}
	// Retrying with nothing failed does nothing.
	if _, cmd := m.Update(press("r")); cmd != nil {
		t.Fatal("retry without an error returned a command")
	}
}

func TestEmpty(t *testing.T) {
	m := load(t, newSource(0, 10), WithEmptyText("No pull requests."))
	if !m.Done() || m.Len() != 0 {
		t.Fatalf("Done() = %v, Len() = %d", m.Done(), m.Len())
	}
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() on an empty feed returned an item")
	}
	if !strings.Contains(m.View(), "No pull requests.") {
		t.Fatalf("View() = %q, want the empty text", m.View())
	}
	m.SetEmptyText("Nothing yet.")
	if m.EmptyText() != "Nothing yet." || !strings.Contains(m.View(), "Nothing yet.") {
		t.Fatal("SetEmptyText did not change the view")
	}
}

func TestIgnoresOtherInstances(t *testing.T) {
	a := load(t, newSource(10, 10))
	b := New(newSource(10, 10).fetch, renderItem)
	if a.ID() == b.ID() {
		t.Fatal("two feeds share an ID")
	}
	// b's first chunk must not reach a.
	a2, cmd := a.Update(b.fetchCmd(0, "")())
	if cmd != nil || a2.Len() != a.Len() {
		t.Fatal("feed reacted to another feed's message")
	}
	// Nor must another spinner's ticks.
	if _, cmd := a.Update(spinner.TickMsg{ID: -1}); cmd != nil {
		t.Fatal("feed reacted to another spinner's tick")
	}
}

func TestSpinnerStopsWhenLoaded(t *testing.T) {
	m := New(newSource(10, 10).fetch, renderItem, WithSize(40, 5))
	tick := m.spin.Tick()
	m, cmd := m.Update(tick)
	if cmd == nil {
		t.Fatal("spinner should keep ticking while loading")
	}
	m = run(t, m, m.Init())
	if m, cmd = m.Update(tick); cmd != nil || m.spinning {
		t.Fatal("spinner should stop once nothing is loading")
	}
}

func TestResizeOnlyMovesWindow(t *testing.T) {
	src := newSource(10, 10)
	m := load(t, src)
	m = keys(t, m, "end")
	calls := src.callCount()

	m.SetSize(40, 3)
	assertVisible(t, m)
	if m.top != 7 {
		t.Fatalf("top = %d after shrinking, want 7", m.top)
	}
	m.SetHeight(20)
	if m.top != 0 {
		t.Fatalf("top = %d after growing, want 0", m.top)
	}
	m.SetWidth(10)
	if m.Width() != 10 || m.Height() != 20 {
		t.Fatalf("size = %dx%d, want 10x20", m.Width(), m.Height())
	}
	if src.callCount() != calls {
		t.Fatal("resizing fetched")
	}
}

func TestAccessors(t *testing.T) {
	m := load(t, newSource(10, 10))
	k := DefaultKeyMap()
	k.Down.SetKeys("n")
	m.SetKeyMap(k)
	m = keys(t, m, "n")
	if m.Index() != 1 {
		t.Fatalf("Index() = %d with a custom key map, want 1", m.Index())
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

func TestPrefetch(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		downs   int
		wantLen int
	}{
		{"default threshold, outside", nil, 3, 10},
		{"default threshold, inside", nil, 4, 20},
		{"custom threshold, outside", []Option{WithPrefetch(2)}, 6, 10},
		{"custom threshold, inside", []Option{WithPrefetch(2)}, 7, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, newSource(30, 10), tt.opts...)
			for range tt.downs {
				m = keys(t, m, "down")
			}
			if m.Len() != tt.wantLen {
				t.Fatalf("Len() = %d, want %d", m.Len(), tt.wantLen)
			}
		})
	}
}

// loadedChunks returns the indexes of the chunks whose items are in memory.
func loadedChunks[T any](m Model[T]) []int {
	var got []int
	for i, c := range m.chunks {
		if c.loaded {
			got = append(got, i)
		}
	}
	return got
}

// scrolledToEnd returns a feed over 100 chunks of 10 that walked to the end
// while keeping at most three chunks.
func scrolledToEnd(t *testing.T, src *source) Model[item] {
	t.Helper()
	m := load(t, src, WithMaxChunks(3))
	for !m.Done() {
		m = keys(t, m, "end")
	}
	return keys(t, m, "end")
}

func TestEviction(t *testing.T) {
	src := newSource(1000, 10)
	m := scrolledToEnd(t, src)
	if got := loadedChunks(m); len(got) > 3 || got[len(got)-1] != 99 {
		t.Fatalf("loaded chunks = %v, want at most 3 ending in 99", got)
	}
	if m.Len() != 1000 {
		t.Fatalf("Len() = %d after eviction, want 1000", m.Len())
	}
	if it, ok := m.Selected(); !ok || it.id != "999" {
		t.Fatalf("Selected() = %v, %v; want item 999", it, ok)
	}

	// Going home shows placeholders until chunk 0 is fetched again.
	m, cmd := m.Update(press("home"))
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() returned an evicted item")
	}
	if !strings.Contains(m.View(), "…") {
		t.Fatalf("View() = %q, want placeholders", m.View())
	}
	calls := src.callCount()
	m = run(t, m, cmd)
	if src.callCount() != calls+1 {
		t.Fatalf("fetched %d chunks, want 1", src.callCount()-calls)
	}
	if it, ok := m.Selected(); !ok || it.id != "0" {
		t.Fatalf("Selected() = %v, %v; want item 0", it, ok)
	}
	if got := loadedChunks(m); len(got) > 3 || got[0] != 0 {
		t.Fatalf("loaded chunks = %v, want at most 3 starting at 0", got)
	}
	if m.Len() != 1000 {
		t.Fatalf("Len() = %d, want 1000", m.Len())
	}
}

func TestWindowChunksAreNeverEvicted(t *testing.T) {
	// A window of 30 rows over chunks of 5 needs more than one chunk.
	m := load(t, newSource(200, 5), WithMaxChunks(1), WithSize(40, 30))
	m = keys(t, m, "pgdown", "pgdown", "pgdown")
	for i := m.top; i < m.top+m.slots() && i < m.Len(); i++ {
		if _, ok := m.item(i); !ok {
			t.Fatalf("row %d in the window is not loaded", i)
		}
	}
}

func TestRefetchError(t *testing.T) {
	src := newSource(1000, 10)
	m := scrolledToEnd(t, src)
	src.setFail("", errors.New("offline"))
	m = keys(t, m, "home")
	if m.Err() == nil || !m.KeyMap().Retry.Enabled() {
		t.Fatal("a failed refetch should set Err and enable retry")
	}
	v := m.View()
	if !strings.Contains(v, "offline") || strings.Count(v, "offline") != 1 {
		t.Fatalf("View() = %q, want the error once", v)
	}
	// Moving within the failed chunk does not retry by itself.
	calls := src.callCount()
	m = keys(t, m, "down")
	if src.callCount() != calls {
		t.Fatal("a failed chunk was fetched again without retry")
	}

	src.setFail("", nil)
	m = keys(t, m, "r")
	if m.Err() != nil {
		t.Fatalf("Err() = %v after retry", m.Err())
	}
	if it, ok := m.Selected(); !ok || it.id != "1" {
		t.Fatalf("Selected() = %v, %v; want item 1", it, ok)
	}
}

func TestResizeFetchesEvictedRows(t *testing.T) {
	src := newSource(1000, 10)
	m := scrolledToEnd(t, src)
	m.SetSize(40, 60)
	calls := src.callCount()
	m, cmd := m.Update(nil)
	if cmd == nil {
		t.Fatal("Update after growing should fetch the rows now in view")
	}
	m = run(t, m, cmd)
	if src.callCount() == calls {
		t.Fatal("nothing was fetched")
	}
	for i := m.top; i < m.Len(); i++ {
		if _, ok := m.item(i); !ok {
			t.Fatalf("row %d in the window is not loaded", i)
		}
	}
	if m.Len() != 1000 {
		t.Fatalf("Len() = %d, want 1000", m.Len())
	}
}

func TestDropsResultsNobodyWaitsFor(t *testing.T) {
	m := load(t, newSource(30, 10))
	before := m.Len()
	for _, msg := range []chunkMsg[item]{
		{id: m.id, index: 0, cursor: ""},     // already loaded
		{id: m.id, index: 5, cursor: "50"},   // no such chunk
		{id: m.id, index: 1, cursor: "nope"}, // wrong cursor
	} {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil || m.Len() != before {
			t.Fatalf("Update(%+v) changed the feed", msg)
		}
	}
}

func itemKey(it item) string { return it.id }

// insert adds n new items at the front of the source.
func (s *source) insert(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	front := make([]item, 0, n+len(s.items))
	for i := range n {
		front = append(front, item{id: fmt.Sprintf("new%d", i), title: "new"})
	}
	s.items = append(front, s.items...)
}

func TestReload(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantSel int
		wantID  string
	}{
		{"by key", []Option{WithKey(itemKey)}, 7, "5"},
		{"by index", nil, 5, "3"},
		{"key of another type is ignored", []Option{WithKey(func(s string) string { return s })}, 5, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSource(30, 10)
			m := load(t, src, tt.opts...)
			m = keys(t, m, "down", "down", "down", "down", "down")
			src.insert(2)
			m = run(t, m, m.Reload())
			if m.Index() != tt.wantSel {
				t.Fatalf("Index() = %d, want %d", m.Index(), tt.wantSel)
			}
			if it, ok := m.Selected(); !ok || it.id != tt.wantID {
				t.Fatalf("Selected() = %v, %v; want item %s", it, ok, tt.wantID)
			}
			assertVisible(t, m)
		})
	}
}

func TestReloadKeepsRowOnScreen(t *testing.T) {
	src := newSource(30, 10)
	m := load(t, src, WithKey(itemKey))
	m = keys(t, m, "pgdown", "up")
	row := m.Index() - m.top
	src.insert(1)
	m = run(t, m, m.Reload())
	if got := m.Index() - m.top; got != row {
		t.Fatalf("selected row moved from %d to %d on screen", row, got)
	}
}

func TestReloadShowsOldItemsUntilNewOnesArrive(t *testing.T) {
	src := newSource(30, 10)
	m := load(t, src)
	before := m.View()
	cmd := m.Reload()
	if m.View() != before {
		t.Fatal("Reload changed the view before new items arrived")
	}
	if cmd == nil {
		t.Fatal("Reload returned no command")
	}
}

func TestReloadFollowsChangedChunks(t *testing.T) {
	src := newSource(25, 10)
	m := load(t, src, WithKey(itemKey))
	m = keys(t, m, "end", "end", "end")
	if !m.Done() || m.Len() != 25 {
		t.Fatalf("Done() = %v, Len() = %d", m.Done(), m.Len())
	}
	// The last chunk grows past its old end, so the feed is no longer done.
	src.insert(10)
	m = run(t, m, m.Reload())
	if m.Len() < 30 || m.Done() && m.Len() != 35 {
		t.Fatalf("Len() = %d, Done() = %v after the source grew", m.Len(), m.Done())
	}
	if it, ok := m.Selected(); !ok || it.id != "24" {
		t.Fatalf("Selected() = %v, %v; want item 24", it, ok)
	}
	m = keys(t, m, "end", "end")
	if !m.Done() || m.Len() != 35 {
		t.Fatalf("Done() = %v, Len() = %d; want true, 35", m.Done(), m.Len())
	}
}

func TestReloadRetriesFailures(t *testing.T) {
	src := newSource(10, 10)
	src.setFail("", errors.New("offline"))
	m := load(t, src)
	src.setFail("", nil)
	m = run(t, m, m.Reload())
	if m.Err() != nil || m.Len() != 10 {
		t.Fatalf("Err() = %v, Len() = %d after Reload", m.Err(), m.Len())
	}
}

func TestReset(t *testing.T) {
	query := "a"
	var ctxs []context.Context
	fetch := func(ctx context.Context, cursor string) ([]item, string, error) {
		ctxs = append(ctxs, ctx)
		next := ""
		if cursor == "" {
			next = "1"
		}
		return []item{{id: query + cursor, title: query}}, next, nil
	}
	m := New(fetch, renderItem, WithSize(40, 1), WithFocused(true))
	// The first fetch is still in flight when the query changes.
	stale := m.Init()

	query = "b"
	cmd := m.Reset()
	if m.Len() != 0 || m.Index() != 0 || !m.tail.fetching {
		t.Fatalf("after Reset: Len() = %d, Index() = %d, loading = %v", m.Len(), m.Index(), m.tail.fetching)
	}
	m = run(t, m, stale)
	if m.Len() != 0 {
		t.Fatal("a result from before Reset was kept")
	}
	if ctxs[0].Err() == nil {
		t.Fatal("Reset should cancel the fetches in flight")
	}
	m = run(t, m, cmd)
	if it, ok := m.Selected(); !ok || it.id != "b" {
		t.Fatalf("Selected() = %v, %v; want the new query's first item", it, ok)
	}
}

func TestReloadDropsStaleResults(t *testing.T) {
	src := newSource(30, 10)
	m := load(t, src)
	m, stale := m.Update(press("end"))
	m = run(t, m, m.Reload())
	before := m.Len()
	m = run(t, m, stale)
	if m.Len() != before {
		t.Fatal("a result from before Reload was kept")
	}
}

func TestSetKey(t *testing.T) {
	src := newSource(30, 10)
	m := load(t, src)
	m.SetKey(itemKey)
	m = keys(t, m, "down")
	src.insert(3)
	m = run(t, m, m.Reload())
	if it, _ := m.Selected(); it.id != "1" {
		t.Fatalf("Selected() = %v, want item 1", it)
	}
}
