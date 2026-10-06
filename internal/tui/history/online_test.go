package history

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestOnlineRetriesOnce checks that GitHub answering again reads once
// what failed for want of an answer: the branches, the history and the
// commit, which it reads again although it was asked for before. What
// GitHub refused, or what loaded, costs nothing.
func TestOnlineRetriesOnce(t *testing.T) {
	offline := fmt.Errorf("github: GET: %w", core.ErrOffline)
	refused := fmt.Errorf("github: 403 Forbidden: %w", core.ErrForbidden)
	tests := []struct {
		name string
		errs map[string]error
		want []string
	}{
		{
			"branches and commit offline",
			map[string]error{"branches": offline, "commit " + short(main0): offline},
			[]string{"branches", "commit " + short(main0)},
		},
		{"history offline", map[string]error{"commits main": offline}, []string{"commits main"}},
		{"refused", map[string]error{"branches": refused, "commit " + short(main0): refused}, nil},
		{"loaded", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			maps.Copy(f.errs, tt.errs)
			m, h := newModal(t, f, 108, 30)
			f.mu.Lock()
			clear(f.errs)
			f.mu.Unlock()
			f.took()

			h.run(func() tea.Msg { return ui.OnlineMsg{} })
			h.run(func() tea.Msg { return ui.OnlineMsg{} })
			// Once the history loads, the commit under the cursor and the
			// comparisons are read as always; only what failed counts.
			var got []string
			for _, c := range f.took() {
				if tt.errs[c] != nil {
					got = append(got, c)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("reads after two OnlineMsg = %q, want %q", got, tt.want)
			}
			if tt.want != nil && (m.branches.err != nil || m.graph.model.Err() != nil || m.commit.err != nil || !m.commit.loaded) {
				t.Errorf("once online: branches %v, history %v, commit %v, loaded %v; want them all",
					m.branches.err, m.graph.model.Err(), m.commit.err, m.commit.loaded)
			}
		})
	}
}

// TestOnlineReadsKeptAgain checks that the branches and the head of the
// history, served kept while GitHub rate limited their reads, are read
// again once the limit lifts, once, and that the history is shown again
// if its head moved meanwhile.
func TestOnlineReadsKeptAgain(t *testing.T) {
	f := newFake()
	f.limited = true
	m, h := newModal(t, f, 108, 30)
	f.mu.Lock()
	f.limited = false
	// A commit landed on main meanwhile.
	moved := slices.Concat(history("hotfix", 1), f.histories["main"])
	f.histories["main"] = moved
	f.mu.Unlock()
	f.took()

	h.run(func() tea.Msg { return ui.OnlineMsg{} })
	h.run(func() tea.Msg { return ui.OnlineMsg{} })
	var got []string
	for _, c := range f.took() {
		if strings.HasPrefix(c, "branches") || c == "commits main" {
			got = append(got, c)
		}
	}
	// The head read, then the history shown again from its first page.
	if want := []string{"branches again", "commits main", "commits main"}; !slices.Equal(got, want) {
		t.Errorf("reads after two OnlineMsg = %q, want %q", got, want)
	}
	if m.branches.kept {
		t.Error("branches still kept once read again")
	}
	if c, ok := m.graph.model.At(0); !ok || c.ID != moved[0].SHA {
		t.Errorf("first commit shown = %v, %v; want the new head %s", c.ID, ok, short(moved[0].SHA))
	}
}

// TestOnlineKeepsBranchOfLaterPage checks that reading the branches again
// from the first page, once the limit lifts, brings the cursor back to the
// branch it was on, which a later page lists.
func TestOnlineKeepsBranchOfLaterPage(t *testing.T) {
	f := newFake()
	first := make([]core.Branch, 1, 12)
	first[0] = core.Branch{Name: "main"}
	for i := range 11 {
		first = append(first, core.Branch{Name: fmt.Sprintf("feat/%d", i)})
	}
	f.branchPages = map[string]core.Page[core.Branch]{
		"":  {Items: first, Next: "2"},
		"2": {Items: []core.Branch{{Name: "fix/a"}, {Name: "fix/b"}, {Name: "fix/c"}}},
	}
	f.limited = true
	m, h := newModal(t, f, 108, 30)
	h.keys("shift+tab", "end", "end")
	if b, _ := m.branches.selected(); b.Name != "fix/c" {
		t.Fatalf("cursor on %q, want fix/c", b.Name)
	}
	f.mu.Lock()
	f.limited = false
	f.mu.Unlock()

	h.run(func() tea.Msg { return ui.OnlineMsg{} })
	if b, _ := m.branches.selected(); b.Name != "fix/c" || m.branches.follow != "" {
		t.Errorf("cursor on %q, following %q; want back on fix/c", b.Name, m.branches.follow)
	}
	if n := len(m.branches.items); n != 15 {
		t.Errorf("%d branches listed, want both pages", n)
	}
}

// TestOnlineFollowEndsAtMaxBranchPages checks that, once the branch the
// cursor was on is in no page any more, reading the branches again from
// the first page stops following it after maxBranchPages pages, without
// reading every page of a repository with more.
func TestOnlineFollowEndsAtMaxBranchPages(t *testing.T) {
	f := newFake()
	first := make([]core.Branch, 1, 12)
	first[0] = core.Branch{Name: "main"}
	for i := range 11 {
		first = append(first, core.Branch{Name: fmt.Sprintf("feat/%d", i)})
	}
	const pages = maxBranchPages + 5
	f.branchPages = map[string]core.Page[core.Branch]{"": {Items: first, Next: "2"}}
	for n := 2; n <= pages; n++ {
		p := core.Page[core.Branch]{Items: []core.Branch{{Name: fmt.Sprintf("fix/%d", n)}}}
		if n < pages {
			p.Next = strconv.Itoa(n + 1)
		}
		f.branchPages[strconv.Itoa(n)] = p
	}
	f.limited = true
	m, h := newModal(t, f, 108, 30)
	h.keys("shift+tab", "j", "j", "j")
	if b, _ := m.branches.selected(); b.Name != "feat/2" {
		t.Fatalf("cursor on %q, want feat/2", b.Name)
	}
	f.mu.Lock()
	f.limited = false
	// The branch is deleted meanwhile.
	f.branchPages[""] = core.Page[core.Branch]{Items: slices.Delete(slices.Clone(first), 3, 4), Next: "2"}
	f.mu.Unlock()
	f.took()

	h.run(func() tea.Msg { return ui.OnlineMsg{} })
	var reads int
	for _, c := range f.took() {
		if strings.HasPrefix(c, "branches") {
			reads++
		}
	}
	if reads != maxBranchPages {
		t.Errorf("read %d pages of branches, want %d of %d", reads, maxBranchPages, pages)
	}
	if b := &m.branches; b.follow != "" || b.loading || b.next != "" {
		t.Errorf("following %q, loading %v, next %q; want the follow ended", b.follow, b.loading, b.next)
	}
}

// TestOnlineHeadRefusedIsNotReadAgain checks that the head of a history
// served kept is read again at each wake while GitHub doesn't answer it,
// but not once GitHub refuses it, such as for a branch deleted meanwhile.
func TestOnlineHeadRefusedIsNotReadAgain(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"offline", fmt.Errorf("github: GET: %w", core.ErrOffline), 2},
		{"not found", fmt.Errorf("github: 404 Not Found: %w", core.ErrNotFound), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.limited = true
			_, h := newModal(t, f, 108, 30)
			f.mu.Lock()
			f.limited = false
			f.errs["commits main"] = tt.err
			f.mu.Unlock()
			f.took()

			h.run(func() tea.Msg { return ui.OnlineMsg{} })
			h.run(func() tea.Msg { return ui.OnlineMsg{} })
			var reads int
			for _, c := range f.took() {
				if c == "commits main" {
					reads++
				}
			}
			if reads != tt.want {
				t.Errorf("head read %d times after two OnlineMsg, want %d", reads, tt.want)
			}
		})
	}
}

// rereadUnderFilter opens the filter over two pages of branches, served
// kept while GitHub rate limited their read, and lifts the limit, which
// reads them again from the first page. The second page is still on its
// way when it returns.
func rereadUnderFilter(t *testing.T) (*Modal, *host, *fake) {
	t.Helper()
	f := newFake()
	first := make([]core.Branch, 1, 12)
	first[0] = core.Branch{Name: "main"}
	for i := range 11 {
		first = append(first, core.Branch{Name: fmt.Sprintf("feat/%d", i)})
	}
	f.branchPages = map[string]core.Page[core.Branch]{
		"":  {Items: first, Next: "2"},
		"2": {Items: []core.Branch{{Name: "fix/a"}, {Name: "fix/b"}, {Name: "fix/c"}}},
	}
	f.limited = true
	m, h := newModal(t, f, 108, 30)
	h.keys("shift+tab", "/")
	if n := m.branches.filter.Len(); n != 15 {
		t.Fatalf("the filter lists %d branches, want both pages", n)
	}
	f.mu.Lock()
	f.limited = false
	f.mu.Unlock()

	h.skip = func(msg tea.Msg) bool {
		b, ok := msg.(branchesMsg)
		return ok && b.cursor == "2"
	}
	h.run(func() tea.Msg { return ui.OnlineMsg{} })
	h.skip = nil
	return m, h, f
}

// TestOnlineFilterKeepsLaterPages checks that the filter, open while the
// branches are read again from the first page once the limit lifts, keeps
// listing the branches of the later pages until the read brings them
// again, rather than shrinking to the first page meanwhile.
func TestOnlineFilterKeepsLaterPages(t *testing.T) {
	m, h, f := rereadUnderFilter(t)
	if n := m.branches.filter.Len(); n != 15 {
		t.Errorf("the filter lists %d branches while the second page is read again, want 15", n)
	}
	// The second page arrives, with a branch deleted meanwhile.
	f.mu.Lock()
	f.branchPages["2"] = core.Page[core.Branch]{Items: []core.Branch{{Name: "fix/a"}, {Name: "fix/b"}}}
	f.mu.Unlock()
	h.run(m.loadBranches("2", true))
	if n := m.branches.filter.Len(); n != 14 {
		t.Errorf("the filter lists %d branches once both pages are read again, want 14", n)
	}
}

// TestOnlineFilterShowsWhatLoadedOnError checks that the filter lists the
// branches loaded once a later page fails to be read again, rather than
// what it listed before the read.
func TestOnlineFilterShowsWhatLoadedOnError(t *testing.T) {
	m, h, f := rereadUnderFilter(t)
	f.mu.Lock()
	f.errs["branches"] = fmt.Errorf("github: GET: %w", core.ErrOffline)
	f.mu.Unlock()
	h.run(m.loadBranches("2", true))
	if n := m.branches.filter.Len(); n != 12 {
		t.Errorf("the filter lists %d branches once the second page failed, want the 12 loaded", n)
	}
}

// TestOnlineFilterChoosesBranchOfLaterPage checks that a branch chosen in
// the filter while the page that lists it is read again gets the cursor
// once that page arrives.
func TestOnlineFilterChoosesBranchOfLaterPage(t *testing.T) {
	m, h, _ := rereadUnderFilter(t)
	h.keys("f", "i", "x", "/", "c", "enter")
	if m.branches.filter != nil || m.graph.shown() != "fix/c" {
		t.Fatalf("filter open %v, graph of %q; want fix/c shown", m.branches.filter != nil, m.graph.shown())
	}
	h.run(m.loadBranches("2", true))
	if b, _ := m.branches.selected(); b.Name != "fix/c" || m.branches.follow != "" {
		t.Errorf("cursor on %q, following %q; want on fix/c", b.Name, m.branches.follow)
	}
}
