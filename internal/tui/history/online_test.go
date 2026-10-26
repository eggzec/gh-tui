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
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
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

// twoPages is a fake with two pages of branches, the second still to read.
func twoPages() *fake {
	f := newFake()
	// The first page is long enough that nothing reads the second yet.
	first := make([]core.Branch, 1, 61)
	first[0] = core.Branch{Name: "main"}
	for i := range 60 {
		first = append(first, core.Branch{Name: fmt.Sprintf("feat/%d", i)})
	}
	f.branchPages = map[string]core.Page[core.Branch]{
		"":  {Items: first, Next: "2"},
		"2": {Items: []core.Branch{{Name: "fix/a"}, {Name: "fix/b"}}},
	}
	return f
}

// TestFilterWaitsForEveryPage checks that f opens the form only once the
// last page of branches is read.
func TestFilterWaitsForEveryPage(t *testing.T) {
	m, h := newModal(t, twoPages(), 108, 30)
	h.take()
	var held []tea.Msg
	h.skip = func(msg tea.Msg) bool {
		b, ok := msg.(branchesMsg)
		return ok && b.cursor == "2"
	}
	h.keys("backspace", "f")
	held = h.take()
	h.skip = nil
	if openedFilter(held) || !m.branches.wanted {
		t.Fatalf("the form opened before the last page; wanted %v", m.branches.wanted)
	}
	h.run(m.loadBranches("2", true))
	if !openedFilter(h.take()) || m.branches.wanted {
		t.Error("the form didn't open once the last page landed")
	}
}

// TestFilterOpensOnAReadError checks that a failed page opens the form with
// the branches loaded.
func TestFilterOpensOnAReadError(t *testing.T) {
	f := twoPages()
	m, h := newModal(t, f, 108, 30)
	h.take()
	h.skip = func(msg tea.Msg) bool {
		b, ok := msg.(branchesMsg)
		return ok && b.cursor == "2"
	}
	h.keys("backspace", "f")
	h.skip = nil
	h.take()
	f.mu.Lock()
	f.errs["branches"] = fmt.Errorf("github: GET: %w", core.ErrOffline)
	f.mu.Unlock()
	h.run(m.loadBranches("2", true))
	if !openedFilter(h.take()) {
		t.Fatal("the form didn't open after the read failed")
	}
	fl, _ := m.Filter()
	if n := len(fl.Spec.Fields[0].Options); n != 61 {
		t.Errorf("the form lists %d branches, want the 61 loaded", n)
	}
}

// TestFilterSpec checks the one choice of the form.
func TestFilterSpec(t *testing.T) {
	m, h := newModal(t, newFake(), 108, 30)
	h.keys("backspace", "j")
	fl, _ := m.Filter()
	if len(fl.Spec.Fields) != 1 {
		t.Fatalf("the form has %d fields, want 1", len(fl.Spec.Fields))
	}
	fd := fl.Spec.Fields[0]
	if fd.Key != "branch" || fd.Kind != filterform.Choice || fd.Label != "Branch" {
		t.Errorf("field %q %q %v, want a Choice branch", fd.Key, fd.Label, fd.Kind)
	}
	for i, br := range m.branches.items {
		want := ""
		if br.Name == m.branches.defaultBranch {
			want = "default"
		}
		if o := fd.Options[i]; o.Value != br.Name || o.Detail != want {
			t.Errorf("option %d is %+v, want %q with detail %q", i, o, br.Name, want)
		}
	}
	if b, _ := m.branches.selected(); fl.Query != "branch:"+b.Name {
		t.Errorf("query %q, want the branch under the cursor %q", fl.Query, b.Name)
	}
}

// TestApplyFilterFollowsAMissingBranch checks that a branch no page loaded
// yet is followed rather than selected.
func TestApplyFilterFollowsAMissingBranch(t *testing.T) {
	m, h := newModal(t, newFake(), 108, 30)
	h.run(m.ApplyFilter(filterform.AppliedMsg{Values: map[string]filterform.Value{fieldBranch: filterform.TextValue("fix/c")}}))
	if m.branches.follow != "fix/c" || m.graph.shown() != "fix/c" {
		t.Errorf("following %q, graph of %q; want fix/c", m.branches.follow, m.graph.shown())
	}
}
