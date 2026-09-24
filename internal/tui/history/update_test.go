package history

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// main0 and the rest are the SHAs of the commits of main, newest first.
var (
	main0 = sha("main", 0)
	main1 = sha("main", 1)
	main2 = sha("main", 2)
	main3 = sha("main", 3)
)

func commitCalls(shas ...string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[i] = "commit " + short(s)
	}
	return out
}

func TestOpensOnTheDefaultBranch(t *testing.T) {
	f := newFake()
	m, _ := newModal(t, f, 108, 30)
	if m.focus != graphPane || !m.graph.model.Focused() {
		t.Errorf("focus = %d, want the graph focused", m.focus)
	}
	if got := names(m.branches.items); !slices.Equal(got, []string{"main", "fix/tabs", "v2-exp"}) {
		t.Errorf("branches = %v, want the default one first", got)
	}
	if b, _ := m.branches.selected(); b.Name != "main" || m.graph.shown() != "main" {
		t.Errorf("cursor on %q, graph of %q; want main", b.Name, m.graph.shown())
	}
	if !m.commit.loaded || m.commit.c.SHA != main0 {
		t.Errorf("commit pane shows %q, loaded %v; want the newest commit", short(m.commit.c.SHA), m.commit.loaded)
	}
	// The branches and the first page, the newest commit, and the three
	// after it read ahead; main isn't compared with itself.
	want := append([]string{"branches", "commits main"}, commitCalls(main0, main1, main2, main3)...)
	if got := f.took(); !sameCalls(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

// sameCalls reports whether got are the calls of want, in any order after
// the first commit read: the reads ahead run at once.
func sameCalls(got, want []string) bool {
	i := slices.IndexFunc(want, func(c string) bool { return strings.HasPrefix(c, "commit ") })
	if i < 0 || len(got) != len(want) {
		return slices.Equal(got, want)
	}
	i++
	return slices.Equal(got[:i], want[:i]) && slices.Equal(slices.Sorted(slices.Values(got[i:])), slices.Sorted(slices.Values(want[i:])))
}

func names(bs []core.Branch) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Name
	}
	return out
}

func TestOpensOnTheBranchOfTheBase(t *testing.T) {
	f := newFake()
	base := ui.BaseMsg{Repo: repo, Ref: sha("v2-exp", 1), Label: "v2-exp @ x", Branch: "v2-exp"}
	m, _ := newModalAt(t, f, 108, 30, base)
	if b, _ := m.branches.selected(); b.Name != "v2-exp" || m.graph.shown() != "v2-exp" {
		t.Errorf("cursor on %q, graph of %q; want v2-exp", b.Name, m.graph.shown())
	}
	if !m.isBase("v2-exp") || m.isBase("main") {
		t.Error("the branch of the base isn't the one marked")
	}
}

func TestFocusMovesBetweenPanes(t *testing.T) {
	m, h := newModal(t, newFake(), 108, 30)
	for _, want := range []pane{commitPane, branchPane, graphPane} {
		h.keys("tab")
		if m.focus != want {
			t.Fatalf("tab focused %d, want %d", m.focus, want)
		}
	}
	h.keys("shift+tab")
	if m.focus != branchPane || m.graph.model.Focused() {
		t.Errorf("shift+tab focused %d, want the branches alone", m.focus)
	}
}

func TestEnterAndEscStepThroughThePanes(t *testing.T) {
	m, h := newModal(t, newFake(), 108, 30)
	h.keys("esc")
	if m.focus != branchPane {
		t.Fatalf("esc from the graph focused %d, want the branches", m.focus)
	}
	h.keys("enter")
	if m.focus != graphPane {
		t.Fatalf("enter on a branch focused %d, want its graph", m.focus)
	}
	h.keys("enter")
	if m.focus != commitPane || m.commit.patch {
		t.Fatalf("enter on a commit focused %d, patch %v; want its files", m.focus, m.commit.patch)
	}
	h.keys("j", "enter")
	if !m.commit.patch || !m.commit.pager.Focused() || m.commit.pager.Name() != "tea.go" {
		t.Fatalf("enter on a file shows %q, patch %v; want its patch", m.commit.pager.Name(), m.commit.patch)
	}
	// Pager keys scroll the patch rather than moving the focus.
	h.keys("j", "space")
	if m.focus != commitPane || !m.commit.patch {
		t.Fatal("pager keys left the patch")
	}
	for _, want := range []pane{commitPane, graphPane, branchPane} {
		h.keys("esc")
		if m.focus != want || m.commit.patch {
			t.Fatalf("esc focused %d, patch %v; want %d", m.focus, m.commit.patch, want)
		}
	}
	if got := h.take(); len(got) != 0 {
		t.Fatalf("stepping back sent %#v", got)
	}
	h.keys("esc")
	if got := h.take(); !slices.Equal(got, []tea.Msg{ui.CloseModalMsg{Modal: m}}) {
		t.Errorf("esc on the branches sent %#v, want the modal closed", got)
	}
	if m.ctx.Err() == nil {
		t.Error("closing left the modal's reads running")
	}
}

func TestPrefetchedCommitShowsAtOnce(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, 108, 30)
	f.took()
	// The commit after the cursor was read ahead, so moving there shows
	// its detail before the cursor rests.
	h.skip = isRest
	h.keys("j")
	if !m.commit.loaded || m.commit.c.SHA != main1 {
		t.Fatalf("commit pane shows %q, loaded %v; want the next commit at once", short(m.commit.c.SHA), m.commit.loaded)
	}
	if got := f.took(); len(got) != 0 {
		t.Errorf("calls = %q, want none", got)
	}
	// Resting there reads one more ahead, the fourth after it.
	h.skip = nil
	h.run(m.Update(restMsg{id: m.id, seq: m.seq}))
	if got, want := f.took(), commitCalls(sha("main", 4)); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestAroundIsConfigured(t *testing.T) {
	f := newFake()
	cfg := testConfig()
	cfg.Prefetch.Around = 0
	_, _ = newModal(t, f, 108, 30, WithConfig(cfg))
	if got, want := f.took(), append([]string{"branches", "commits main"}, commitCalls(main0)...); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want only the commit under the cursor, %q", got, want)
	}
}

func TestUseAsBase(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want ui.BaseMsg
	}{
		{"commit", []string{"j"}, ui.BaseMsg{Repo: repo, Ref: main1, Label: "main @ " + short(main1), Branch: "main"}},
		{"commit pane", []string{"j", "enter"}, ui.BaseMsg{Repo: repo, Ref: main1, Label: "main @ " + short(main1), Branch: "main"}},
		{"branch", []string{"esc", "j"}, ui.BaseMsg{Repo: repo, Ref: "fix/tabs", Label: "fix/tabs", Branch: "fix/tabs"}},
		{"commit of a branch", []string{"esc", "j", "j", "enter", "j"}, ui.BaseMsg{
			Repo: repo, Ref: sha("v2-exp", 1), Label: "v2-exp @ " + short(sha("v2-exp", 1)), Branch: "v2-exp",
		}},
		// The head of the default branch is where the files start.
		{"default branch", []string{"esc"}, ui.BaseMsg{Repo: repo}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, h := newModal(t, newFake(), 108, 30)
			h.keys(tt.keys...)
			h.take()
			h.keys("space")
			want := []tea.Msg{ui.CloseModalMsg{Modal: m}, tt.want, ui.ShowMsg{Title: ui.FilesTitle}}
			if got := h.take(); !slices.Equal(got, want) {
				t.Errorf("space sent %#v, want %#v", got, want)
			}
		})
	}
}

func TestResetBase(t *testing.T) {
	_, h := newModal(t, newFake(), 108, 30)
	h.keys("H")
	if len(h.take()) != 0 {
		t.Error("H sent something while the files show the head already")
	}
	m, h := newModalAt(t, newFake(), 108, 30, ui.BaseMsg{Repo: repo, Ref: "v2-exp", Label: "v2-exp", Branch: "v2-exp"})
	h.keys("H")
	want := []tea.Msg{ui.CloseModalMsg{Modal: m}, ui.BaseMsg{Repo: repo}, ui.ShowMsg{Title: ui.FilesTitle}}
	if got := h.take(); !slices.Equal(got, want) {
		t.Errorf("H sent %#v, want %#v", got, want)
	}
}

func TestOpenInBrowser(t *testing.T) {
	commitURL := "https://github.com/charmbracelet/bubbletea/commit/" + main1
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"branch", []string{"esc", "j"}, "https://github.com/charmbracelet/bubbletea/tree/fix/tabs"},
		{"commit", []string{"j"}, commitURL},
		// GitHub anchors the diff of a file by the SHA-256 of its path.
		{"file", []string{"j", "enter", "j"}, commitURL + "#diff-485d4740c371755eea1953e67aea510eb174767a0ba9b7e6d6a7e1d2f15e775e"},
		{"patch", []string{"j", "enter", "j", "enter"}, commitURL + "#diff-485d4740c371755eea1953e67aea510eb174767a0ba9b7e6d6a7e1d2f15e775e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, h := newModal(t, newFake(), 108, 30)
			h.keys(tt.keys...)
			h.take()
			h.keys("o")
			if got := h.take(); !slices.Equal(got, []tea.Msg{ui.OpenMsg{URL: tt.want}}) {
				t.Errorf("o sent %#v, want %q", got, tt.want)
			}
		})
	}
}

func TestShowBranch(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, 108, 30)
	h.keys("esc", "j", "j")
	f.took()
	h.keys("enter")
	if m.graph.shown() != "v2-exp" || m.focus != graphPane || m.graph.model.Len() != 5 {
		t.Fatalf("graph of %q with %d commits, focus %d; want v2-exp focused", m.graph.shown(), m.graph.model.Len(), m.focus)
	}
	if m.commit.c.SHA != sha("v2-exp", 0) {
		t.Errorf("commit pane shows %q, want the head of v2-exp", short(m.commit.c.SHA))
	}
	want := append([]string{"commits v2-exp"}, commitCalls(sha("v2-exp", 0), sha("v2-exp", 1), sha("v2-exp", 2), sha("v2-exp", 3))...)
	if got := f.took(); !sameCalls(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
	// Choosing the branch shown again reads nothing.
	h.keys("esc", "enter")
	if got := f.took(); len(got) != 0 {
		t.Errorf("calls = %q, want none", got)
	}
}

func TestCompareOnlyTheBranchUnderTheCursor(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, 108, 30)
	f.took()
	h.keys("esc", "j")
	if got := f.took(); !slices.Equal(got, []string{"compare fix/tabs"}) {
		t.Errorf("calls = %q, want fix/tabs compared", got)
	}
	if !strings.Contains(screen(m), "fix/tabs       ↑2↓5") {
		t.Errorf("the branch pane lacks how far fix/tabs is:\n%s", screen(m))
	}
	// A branch is compared once.
	h.keys("j", "k")
	if got := f.took(); !slices.Equal(got, []string{"compare v2-exp"}) {
		t.Errorf("calls = %q, want v2-exp compared alone", got)
	}
}

func TestFilterBranches(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, 108, 30)
	h.keys("esc", "/", "v", "2")
	if m.branches.filter == nil || !strings.Contains(screen(m), "v2-exp") || strings.Contains(screen(m), "fix/tabs") {
		t.Fatalf("the filter shows:\n%s", screen(m))
	}
	// The filter takes every key, esc and tab too.
	h.keys("tab")
	if m.focus != branchPane {
		t.Fatal("tab left the filter")
	}
	f.took()
	h.keys("enter")
	if m.branches.filter != nil || m.graph.shown() != "v2-exp" || m.focus != graphPane {
		t.Errorf("filter open %v, graph of %q; want v2-exp shown", m.branches.filter != nil, m.graph.shown())
	}
	if b, _ := m.branches.selected(); b.Name != "v2-exp" {
		t.Errorf("cursor on %q, want v2-exp", b.Name)
	}
	h.keys("esc", "/", "esc")
	if m.branches.filter != nil || m.focus != branchPane {
		t.Error("esc didn't close the filter alone")
	}
}

func TestStaleFirstPageIsReadAgain(t *testing.T) {
	f := newFake()
	f.stale = true
	m, _ := newModal(t, f, 108, 30)
	got := f.took()
	if n := count(got, "commits main"); n != 2 {
		t.Errorf("calls = %q, want the first page read again", got)
	}
	if m.graph.model.Len() != commitPage {
		t.Errorf("graph has %d commits, want the first page", m.graph.model.Len())
	}
}

func TestHeadMovedResetsTheGraph(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, 108, 30)
	f.mu.Lock()
	moved := append(history("pushed", 1), f.histories["main"]...)
	f.histories["main"] = moved
	f.details[moved[0].SHA] = detail(moved[0])
	f.mu.Unlock()
	// The revalidator found main moved.
	h.run(m.Update(ui.SyncMsg{Key: historysvc.SyncKey(core.RepoRef{Owner: "CharmBracelet", Name: "BubbleTea"})}))
	if c, _ := m.graph.model.At(0); c.ID != moved[0].SHA || m.commit.c.SHA != moved[0].SHA {
		t.Errorf("graph starts at %q, want the new head", short(c.ID))
	}
	f.took()
	h.run(m.Update(ui.SyncMsg{Key: "history:someone/else"}))
	if got := f.took(); len(got) != 0 {
		t.Errorf("a sync of another repository read %q", got)
	}
}

func count(calls []string, call string) int {
	n := 0
	for _, c := range calls {
		if c == call {
			n++
		}
	}
	return n
}

func TestErrorsAndRetry(t *testing.T) {
	f := newFake()
	f.errs["branches"] = errBoom
	f.errs["commit "+short(main0)] = errBoom
	m, h := newModal(t, f, 108, 30)
	if s := paneText(m, branchPane); !strings.Contains(s, "✗ Couldn't load: boom · r to retry") {
		t.Errorf("branch pane lacks the error:\n%s", s)
	}
	if s := paneText(m, commitPane); !strings.Contains(s, "✗ Couldn't load the changes: boom · r to retry") {
		t.Errorf("commit pane lacks the error:\n%s", s)
	}
	delete(f.errs, "branches")
	delete(f.errs, "commit "+short(main0))
	h.keys("tab", "r")
	if !m.commit.loaded {
		t.Error("r didn't read the commit again")
	}
	h.keys("tab", "r")
	if len(m.branches.items) != 3 {
		t.Error("r didn't read the branches again")
	}
}

func TestMoreFiles(t *testing.T) {
	f := newFake()
	d := f.details[main0]
	d.Files = manyFiles(0, 300)
	d.FilesNext = "1"
	f.details[main0] = d
	f.moreFiles[main0] = [][]core.CommitFile{manyFiles(300, 5)}
	m, h := newModal(t, f, 108, 30)
	f.took()
	h.keys("enter", "G")
	if got := f.took(); !slices.Equal(got, []string{"files " + short(main0) + "@1"}) {
		t.Errorf("calls = %q, want the next page of files", got)
	}
	if n := len(m.commit.files); n != 305 {
		t.Errorf("%d files, want 305", n)
	}
}

func manyFiles(from, n int) []core.CommitFile {
	out := make([]core.CommitFile, n)
	for i := range n {
		out[i] = core.CommitFile{Path: "pkg/file" + strings.Repeat("x", i%3) + string(rune('a'+(from+i)%26)) + ".go", Status: core.FileAdded, Additions: 1}
	}
	return out
}

func TestPatchNotices(t *testing.T) {
	tests := []struct {
		name string
		file core.CommitFile
		want string
	}{
		{"too large", core.CommitFile{Path: "big.go", Status: core.FileModified, Additions: 9000, PatchTruncated: true}, "This diff is too large to show here. Press o to see it on GitHub."},
		{"binary", core.CommitFile{Path: "logo.png", Status: core.FileAdded}, "No diff to show: the file is binary, or empty."},
		{"renamed", core.CommitFile{Path: "new.go", PreviousPath: "old.go", Status: core.FileRenamed}, "Renamed from old.go, with no changes."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			d := f.details[main0]
			d.Files = []core.CommitFile{tt.file}
			f.details[main0] = d
			m, h := newModal(t, f, 108, 30)
			h.keys("enter", "enter")
			if s := paneText(m, commitPane); !strings.Contains(s, tt.want) {
				t.Errorf("commit pane lacks %q:\n%s", tt.want, s)
			}
		})
	}
}

func TestIgnoresOtherModals(t *testing.T) {
	f := newFake()
	m, _ := newModal(t, f, 108, 30)
	other, _ := newModal(t, newFake(), 108, 30)
	f.took()
	before := screen(m)
	for _, msg := range []tea.Msg{
		branchesMsg{id: other.id},
		detailMsg{id: other.id, sha: main0, err: errBoom},
		restMsg{id: other.id, seq: m.seq},
		headMsg{id: other.id, gen: m.graph.gen, first: "x"},
	} {
		_ = m.Update(msg)
	}
	if screen(m) != before || len(f.took()) != 0 {
		t.Error("the messages of another modal reached this one")
	}
}
