package history

import (
	"fmt"
	"maps"
	"slices"
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
