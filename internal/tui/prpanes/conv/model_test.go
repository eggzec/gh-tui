package conv

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// fake is a pull request whose conversation comes in pages.
type fake struct {
	tl [][]core.TimelineItem
	th [][]core.Thread
	// cached makes the cached reads answer, as stale pages.
	cached bool
	// fetches counts the reads from the network.
	fetches int
	offline bool
	// tlErr and thErr fail the reads of the page of that index; -1 is none.
	tlErr, thErr int
	tlTrunc      bool
	// tlLoop makes the timeline's pages from the second on point to
	// themselves.
	tlLoop bool
	// threadsTotal is how many threads GitHub says there are; 0 counts the
	// pages.
	threadsTotal int
}

func newFake(tl [][]core.TimelineItem, th [][]core.Thread) *fake {
	return &fake{tl: tl, th: th, tlErr: -1, thErr: -1}
}

func pageAt(cursor string) int {
	if cursor == "" {
		return 0
	}
	n, _ := strconv.Atoi(cursor)
	return n
}

func next(i, n int) string {
	if i+1 >= n {
		return ""
	}
	return strconv.Itoa(i + 1)
}

func (f *fake) timelinePage(cursor string) core.Page[core.TimelineItem] {
	i := pageAt(cursor)
	nx := next(i, len(f.tl))
	if f.tlLoop && i >= 1 {
		nx = cursor
	}
	return core.Page[core.TimelineItem]{Items: slices.Clone(f.tl[i]), Next: nx, Truncated: f.tlTrunc && i == 0}
}

func (f *fake) threadPage(cursor string) core.ThreadPage {
	i := pageAt(cursor)
	total := f.threadsTotal
	if total == 0 {
		for _, p := range f.th {
			total += len(p)
		}
	}
	return core.ThreadPage{Threads: slices.Clone(f.th[i]), Total: total, Next: next(i, len(f.th))}
}

func (f *fake) source() Source {
	s := Source{
		Timeline: func(_ context.Context, cursor string) (core.Page[core.TimelineItem], error) {
			f.fetches++
			if f.tlErr == pageAt(cursor) {
				return core.Page[core.TimelineItem]{}, errors.New("boom")
			}
			p := f.timelinePage(cursor)
			p.Offline = f.offline
			return p, nil
		},
		Threads: func(_ context.Context, cursor string) (core.ThreadPage, error) {
			f.fetches++
			if f.thErr == pageAt(cursor) {
				return core.ThreadPage{}, errors.New("boom")
			}
			p := f.threadPage(cursor)
			p.Offline = f.offline
			return p, nil
		},
	}
	if f.cached {
		s.CachedTimeline = func(cursor string) (core.Page[core.TimelineItem], bool) {
			p := f.timelinePage(cursor)
			p.Stale = true
			return p, true
		}
		s.CachedThreads = func(cursor string) (core.ThreadPage, bool) { return f.threadPage(cursor), true }
	}
	return s
}

// conversation is three pages of the timeline and two of the threads.
func conversation() *fake {
	return newFake(
		[][]core.TimelineItem{
			{comment("1"), review("R1", core.ReviewStateCommented)},
			{comment("2"), review("R2", core.ReviewStateCommented)},
			{comment("3")},
		},
		[][]core.Thread{
			{thread("T1", "R1", 2)},
			{thread("T2", "R2", 1)},
		},
	)
}

// driver runs the commands of a model, one at a time, as the runtime does.
type driver struct {
	t testing.TB
	m Model
	q []tea.Cmd
}

func newDriver(tb testing.TB, f *fake, opts ...Option) *driver {
	tb.Helper()
	o := opts
	if len(o) == 0 {
		o = []Option{WithSize(100, 40), WithOptions(opts0()), WithTreeKeys(treeKeys())}
	}
	d := &driver{t: tb, m: New(f.source(), o...)}
	d.q = []tea.Cmd{d.m.Init()}
	return d
}

func opts0() Options { return opts(config.IconsUnicode, 100) }

func treeKeys() foldtree.KeyMap {
	return foldtree.NewKeyMap(keytest.Table(map[string][]string{
		"up":            {"up"},
		"down":          {"down"},
		"global.select": {"enter"},
	}))
}

// step runs the next command and gives its message to the model. It reports
// whether there was a command.
func (d *driver) step() bool {
	for len(d.q) > 0 {
		c := d.q[0]
		d.q = d.q[1:]
		if c == nil {
			continue
		}
		msg := c()
		if b, ok := msg.(tea.BatchMsg); ok {
			d.q = append(b, d.q...)
			continue
		}
		d.send(msg)
		return true
	}
	return false
}

func (d *driver) send(msg tea.Msg) {
	var cmd tea.Cmd
	d.m, cmd = d.m.Update(msg)
	d.q = append(d.q, cmd)
}

func (d *driver) all() {
	for range 1000 {
		if !d.step() {
			return
		}
	}
	d.t.Fatal("the commands never end")
}

// rowIDs returns the IDs of the rows of the view that shows.
func (d *driver) rowIDs() []string { return ids(d.m.views[d.m.cur].rows) }

func TestLoadAllPages(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	for !d.m.ready() {
		if !d.step() {
			t.Fatal("never ready")
		}
	}
	if st := d.m.Status(); !st.Loading {
		t.Fatalf("status after the first pages = %+v, want it loading", st)
	}
	v := d.m.views[""]
	v.tree.Reveal("c:1")
	v.tree.SetFolded("r:R1", true)
	d.all()

	got := d.rowIDs()
	for _, want := range []string{"c:1", "r:R1", "t:T1", "p:T1-b", "c:2", "r:R2", "t:T2", "c:3"} {
		if !slices.Contains(got, want) {
			t.Errorf("rows %v lack %s", got, want)
		}
	}
	if id, kind, ok := d.m.Selected(); !ok || id != "c:1" || kind != KindComment {
		t.Errorf("selected = %q %q %v, want c:1 comment", id, kind, ok)
	}
	if !d.m.views[""].tree.Folded("r:R1") {
		t.Error("the closed review opened when more pages arrived")
	}
	if st := d.m.Status(); st.Loading || st.Total != 5+2 || st.Err != nil {
		t.Errorf("status = %+v, want loaded with 7 entries", st)
	}
}

func TestFoldsOutliveReload(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.offline = true
	d := newDriver(t, f)
	d.all()
	d.m.views[""].tree.SetFolded("r:R1", true)

	// A read that has no R1 drops its row, and its fold with it, from the tree.
	full := f.tl
	f.tl = [][]core.TimelineItem{{comment("1")}}
	d.send(ui.OnlineMsg{})
	d.all()
	if slices.Contains(d.rowIDs(), "r:R1") {
		t.Fatal("the review is still there")
	}
	f.tl = full
	d.send(ui.OnlineMsg{})
	d.all()
	if !d.m.views[""].tree.Folded("r:R1") {
		t.Error("the fold of a review that went and came back is open")
	}
}

func TestCachedFirst(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.cached = true
	d := newDriver(t, f)
	d.step()
	d.step()
	if !d.m.ready() || f.fetches != 0 {
		t.Fatalf("ready = %v after %d reads, want rows from the kept pages alone", d.m.ready(), f.fetches)
	}
	if got := d.rowIDs(); !slices.Contains(got, "c:3") || !slices.Contains(got, "t:T2") {
		t.Errorf("rows from the kept pages = %v, want every page walked", got)
	}
	if st := d.m.Status(); !st.Stale {
		t.Errorf("status = %+v, want stale", st)
	}
	d.all()
	if f.fetches != 5 {
		t.Errorf("%d reads, want 5 pages", f.fetches)
	}
	if st := d.m.Status(); st.Stale || st.Loading {
		t.Errorf("status = %+v, want fresh once read", st)
	}
}

func TestForeignMsgsIgnored(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	d.all()
	before, version := d.rowIDs(), d.m.version
	late := loadMsg{model: d.m.id, seq: d.m.seq, src: timeline, first: true, chunk: chunk{have: true, items: []core.TimelineItem{comment("x")}}}
	for name, msg := range map[string]loadMsg{
		"other model": {model: d.m.id + 1, seq: d.m.seq, src: late.src, first: true, chunk: late.chunk},
		"old seq":     {model: d.m.id, seq: d.m.seq - 1, src: late.src, first: true, chunk: late.chunk},
	} {
		m, cmd := d.m.Update(msg)
		if cmd != nil || m.version != version || !slices.Equal(ids(m.views[""].rows), before) {
			t.Errorf("%s changed the model", name)
		}
	}
}

func TestOfflineAndOnline(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.offline = true
	d := newDriver(t, f)
	d.all()
	if st := d.m.Status(); !st.Offline {
		t.Fatalf("status = %+v, want offline", st)
	}
	if len(d.rowIDs()) == 0 {
		t.Fatal("the kept rows went")
	}

	f.offline = false
	reads := f.fetches
	d.send(ui.OnlineMsg{})
	d.all()
	if st := d.m.Status(); st.Offline || st.Loading {
		t.Errorf("status = %+v after GitHub answered, want it online", st)
	}
	if f.fetches != reads+5 {
		t.Errorf("%d reads after OnlineMsg, want all 5 pages again", f.fetches-reads)
	}

	_, cmd := d.m.Update(ui.OnlineMsg{})
	if cmd != nil {
		t.Error("a model that shows fresh data read again on OnlineMsg")
	}
}

func TestErrorLine(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.tlErr = 0
	d := newDriver(t, f)
	d.all()
	if st := d.m.Status(); st.Err == nil || st.Loading {
		t.Fatalf("status = %+v, want the error", st)
	}
	view := ansi.Strip(d.m.View())
	if strings.Contains(view, "loading") || strings.TrimSpace(view) == "" {
		t.Errorf("view without data reads %q, want the error line", view)
	}
	if len(d.m.views[""].rows) != 0 {
		t.Error("rows without data")
	}

	f = conversation()
	f.thErr = 1
	d = newDriver(t, f)
	d.all()
	if st := d.m.Status(); st.Err == nil {
		t.Error("an error after the first page is not in the status")
	}
	if got := d.rowIDs(); !slices.Contains(got, "t:T1") || !slices.Contains(got, "c:3") {
		t.Errorf("rows with an error after data = %v, want the data kept", got)
	}

	// A source whose first page failed doesn't hide what the other read.
	f = conversation()
	f.thErr = 0
	d = newDriver(t, f)
	d.all()
	if st := d.m.Status(); st.Err == nil || st.Loading {
		t.Fatalf("status = %+v, want the error", st)
	}
	got := d.rowIDs()
	for _, want := range []string{"c:1", "r:R1", "c:3"} {
		if !slices.Contains(got, want) {
			t.Errorf("rows %v lack %s, read before the threads failed", got, want)
		}
	}
	if view := ansi.Strip(d.m.View()); strings.Contains(view, "load the conversation") {
		t.Errorf("view = %q, want the rows, not the error line", view)
	}
}

func TestRepeatedCursorEnds(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.tlLoop = true
	d := newDriver(t, f)
	d.all()
	if st := d.m.Status(); st.Loading {
		t.Errorf("status = %+v, want the reads ended at the repeated cursor", st)
	}
}

func TestMoreNotShown(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.threadsTotal = 4
	d := newDriver(t, f)
	d.all()
	rows := d.m.views[""].rows
	last := rows[len(rows)-1]
	if got := ansi.Strip(last.Lines[0]); got != "2 more not shown" || !last.Skip {
		t.Errorf("last row = %q skip %v, want the count of threads not read", got, last.Skip)
	}
	f = conversation()
	f.tlTrunc = true
	d = newDriver(t, f)
	d.all()
	rows = d.m.views[""].rows
	if got := ansi.Strip(rows[len(rows)-1].Lines[0]); got != "more not shown" {
		t.Errorf("last row of a truncated timeline = %q", got)
	}
}

func TestEnter(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	d.all()
	d.m.Focus()
	enter := func(id string) EnterMsg {
		t.Helper()
		d.m.views[""].tree.Reveal(id)
		m, cmd := d.m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		d.m = m
		if cmd == nil {
			t.Fatalf("enter on %s sent nothing", id)
		}
		m, cmd = d.m.Update(cmd())
		d.m = m
		if cmd == nil {
			t.Fatalf("the tree's message for %s sent nothing on", id)
		}
		msg := cmd()
		got, ok := msg.(EnterMsg)
		if !ok {
			t.Fatalf("enter on %s sent %T", id, msg)
		}
		return got
	}
	if got := enter("c:1"); got.ID != "c:1" || got.Kind != KindComment || got.Item == nil || got.Item.ID != "1" {
		t.Errorf("enter on a comment = %+v", got)
	}
	if got := enter("p:T1-b"); got.Kind != KindReply || got.Thread == nil || got.Comment == nil || got.Comment.ID != "T1-b" {
		t.Errorf("enter on a reply = %+v", got)
	}
	// A message of another tree is not the model's to answer.
	if _, cmd := d.m.Update(foldtree.EnterMsg{Tree: d.m.views[""].tree.ID() + 1000, ID: "c:1"}); cmd != nil {
		t.Error("an enter of a foreign tree was answered")
	}
}

func TestCacheNotRebuiltOnView(t *testing.T) {
	t.Parallel()
	builds := 0
	d := newDriver(t, conversation(),
		WithSize(100, 40), WithOptions(opts0()), WithTreeKeys(treeKeys()), withBuilt(func() { builds++ }))
	d.m.Focus()
	d.all()
	base := builds
	if base == 0 {
		t.Fatal("rows were never built")
	}
	for range 5 {
		_ = d.m.View()
	}
	for _, k := range []tea.Key{{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyUp}, {Code: tea.KeyEnter}} {
		d.send(tea.KeyPressMsg(k))
	}
	d.m.SetSize(100, 30)
	if builds != base {
		t.Fatalf("%d builds after views, keys and a new height, want none", builds-base)
	}
	d.m.SetSize(120, 30)
	if builds != base+1 {
		t.Errorf("%d builds after a new width, want 1", builds-base)
	}
	d.m.SetSize(120, 30)
	if builds != base+1 {
		t.Errorf("%d builds after the same size again, want 1", builds-base)
	}
}

func TestViewSize(t *testing.T) {
	t.Parallel()
	for _, sz := range [][2]int{{80, 24}, {120, 40}, {160, 50}, {19, 5}} {
		t.Run(fmt.Sprintf("%dx%d", sz[0], sz[1]), func(t *testing.T) {
			t.Parallel()
			d := newDriver(t, conversation(), WithSize(sz[0], sz[1]), WithOptions(opts0()), WithTreeKeys(treeKeys()))
			check := func(when string) {
				t.Helper()
				lines := strings.Split(d.m.View(), "\n")
				if len(lines) != sz[1] {
					t.Errorf("%s: %d lines, want %d", when, len(lines), sz[1])
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w != sz[0] {
						t.Errorf("%s: line %d is %d cells, want %d", when, i, w, sz[0])
					}
				}
			}
			check("loading")
			d.all()
			check("loaded")
		})
	}
}

// shown loads f at the width and returns what the model draws, without colors
// or trailing spaces.
func shown(t *testing.T, f *fake, width int, viewer string) string {
	t.Helper()
	o := opts(config.IconsNerd, width)
	d := newDriver(t, f, WithSize(width, 14), WithOptions(o), WithTreeKeys(treeKeys()), WithViewer(viewer))
	d.all()
	lines := strings.Split(ansi.Strip(d.m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func TestPendingReply(t *testing.T) {
	t.Parallel()
	// Each subtest reads a fake of its own: a fake counts its reads.
	f := func() *fake {
		posted := thread("T1", "R1", 2)
		posted.Comments[1].Pending = true
		return newFake(
			[][]core.TimelineItem{{review("R1", core.ReviewStateChangesRequested)}},
			[][]core.Thread{{posted}},
		)
	}
	for _, w := range []int{80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			golden.RequireEqual(t, shown(t, f(), w, "dee"))
		})
	}
}

func TestPendingReviewGroup(t *testing.T) {
	t.Parallel()
	f := func() *fake {
		draft := thread("T1", "", 2)
		draft.Pending = true
		draft.Comments[0].Pending, draft.Comments[1].Pending = true, true
		return newFake(
			[][]core.TimelineItem{{comment("1")}},
			[][]core.Thread{{draft}},
		)
	}
	for _, w := range []int{80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			golden.RequireEqual(t, shown(t, f(), w, "dee"))
		})
	}
	t.Run("no viewer", func(t *testing.T) {
		t.Parallel()
		if got := shown(t, f(), 80, ""); !strings.Contains(got, "your pending review") {
			t.Errorf("without a viewer the group reads:\n%s", got)
		}
	})
}

// bigFake is a conversation of 300 items and 100 threads.
func bigFake() *fake {
	var items []core.TimelineItem
	for i := range 300 {
		if i%3 == 2 {
			items = append(items, review(fmt.Sprint("R", i), core.ReviewStateCommented))
		} else {
			items = append(items, comment(strconv.Itoa(i)))
		}
	}
	ths := make([]core.Thread, 0, 100)
	for i := range 100 {
		ths = append(ths, thread(fmt.Sprint("T", i), fmt.Sprint("R", i*3+2), 3))
	}
	return newFake([][]core.TimelineItem{items}, [][]core.Thread{ths})
}

func BenchmarkView(b *testing.B) {
	builds := 0
	d := newDriver(b, bigFake(),
		WithSize(160, 50), WithOptions(opts(config.IconsNerd, 160)), WithTreeKeys(treeKeys()), withBuilt(func() { builds++ }))
	d.all()
	d.m.Focus()
	before := builds
	b.ReportAllocs()
	for b.Loop() {
		_ = d.m.View()
	}
	if builds != before {
		b.Fatalf("View built rows %d times", builds-before)
	}
}

func BenchmarkUpdate(b *testing.B) {
	d := newDriver(b, bigFake(),
		WithSize(160, 50), WithOptions(opts(config.IconsNerd, 160)), WithTreeKeys(treeKeys()))
	d.all()
	d.m.Focus()
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	up := tea.KeyPressMsg{Code: tea.KeyUp}
	b.ReportAllocs()
	for b.Loop() {
		d.m, _ = d.m.Update(down)
		d.m, _ = d.m.Update(up)
	}
}
