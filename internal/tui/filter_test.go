package tui

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// filterSection is a fake section with a filter and chips, which opens the
// filter on its own filter and sort keys, as a list does.
type filterSection struct {
	*fakeSection
	query   string
	ready   bool
	applied []filterform.AppliedMsg
	chips   string
	// sorts adds a sort to the filter.
	sorts bool
	// filter and sort are the keys that open the filter and the sort.
	filter, sort key.Binding
}

// Update opens the filter or the sort on their keys, where it has them,
// and records the rest.
func (s *filterSection) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case s.ready && key.Matches(k, s.filter):
			return ui.OpenFilter(filterform.FiltersTab)
		case s.ready && s.sorts && key.Matches(k, s.sort):
			return ui.OpenFilter(filterform.SortTab)
		}
	}
	return s.fakeSection.Update(msg)
}

func (s *filterSection) Filter() (ui.Filter, bool) {
	spec := filterform.Spec{Fields: []filterform.Field{{
		Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is",
		Options: []filterform.Item{{Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}},
		Default: filterform.TextValue("open"),
	}}}
	if s.sorts {
		spec.Sort = &filterform.SortField{
			Options: []filterform.SortOption{ui.SortByTime("Updated", "updated"), ui.SortByCount("Comments", "comments")},
			Default: filterform.Sort{By: "updated", Desc: true},
		}
	}
	return ui.Filter{Spec: spec, Query: s.query, Subject: testRepo.String()}, s.ready
}

func (s *filterSection) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	s.applied = append(s.applied, msg)
	s.chips = msg.Query
	return nil
}

func (s *filterSection) Chips() string { return s.chips }

// newFilterApp returns an app whose pull requests filter, focused on them.
func newFilterApp(t *testing.T) (*Model, *filterSection, []*fakeSection) {
	t.Helper()
	return newFilterAppWith(t, config.Default(), 120, 40)
}

// newFilterAppWith returns an app of cfg and size whose pull requests
// filter, focused on them.
func newFilterAppWith(t *testing.T, cfg config.Config, width, height int) (*Model, *filterSection, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}}
	list := ui.In(cfg.Keys, "pulls")
	pulls := &filterSection{
		fakeSection: fakes[1], query: "is:closed", ready: true,
		filter: list.Binding("filter", "filter"), sort: list.Binding("sort", "sort"),
	}
	m := New(t.Context(), cfg, Layout{Files: fakes[0], Pulls: pulls, Issues: fakes[2]}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	run(m, m.showScreen(repoScreen, 1))
	return m, pulls, fakes
}

func filterModal(t *testing.T, m *Model) *ui.FilterModal {
	t.Helper()
	mod, ok := m.topModal().(*ui.FilterModal)
	if !ok {
		t.Fatalf("modal = %T, want the filter modal", m.topModal())
	}
	return mod
}

func TestFilterKeyOpensTheModal(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	run(m, m.key(press("f")))
	mod := filterModal(t, m)
	if got, want := mod.Title(), "Filter · Pull requests · eggzec/gh-tui"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if got := mod.Query(); got != "is:closed" {
		t.Errorf("query = %q, want the filters in force", got)
	}
	if pulls.got(isKey("f")) {
		t.Error("the section got the filter key too")
	}
	// The form is far shorter than the screen, so the frame fits it.
	if w, h := m.frameSize(); w > 104 || h >= 32 {
		t.Errorf("frame = %dx%d, want it fitted to the form", w, h)
	}
}

func TestFilterModalApplies(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	run(m, m.key(press("f")))
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyRight}))
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if m.topModal() != nil {
		t.Error("the modal stayed open after apply")
	}
	if len(pulls.applied) != 1 || pulls.applied[0].Query != "is:open" || pulls.applied[0].Values["state"].Text() != "open" {
		t.Fatalf("applied = %+v, want the state chosen", pulls.applied)
	}
	if got := m.panes[1].label; got != "[2] Pull requests · is:open" {
		t.Errorf("pane label = %q, want the chips after the title", got)
	}
	if !strings.Contains(m.panes[1].top, "Pull requests · is:open") {
		t.Errorf("frame top = %q, want it redrawn with the chips", m.panes[1].top)
	}
}

func TestFilterModalCancels(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	run(m, m.key(press("f")))
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if m.topModal() != nil || len(pulls.applied) != 0 {
		t.Errorf("modal %v, applied %v; want it closed with nothing applied", m.topModal(), pulls.applied)
	}
}

func TestFilterKeyWithoutAFilterGoesToTheSection(t *testing.T) {
	m, pulls, fakes := newFilterApp(t)
	pulls.ready = false
	run(m, m.key(press("f")))
	if m.topModal() != nil || !pulls.got(isKey("f")) {
		t.Error("the filter key opened a modal with nothing to filter, or didn't reach the section")
	}
	run(m, m.showScreen(repoScreen, 0))
	run(m, m.key(press("f")))
	if m.topModal() != nil || !fakes[0].got(isKey("f")) {
		t.Error("the filter key didn't reach a section without a filter")
	}
}

// Tab cycles the panes of the repository screen, while ] and [ reach the
// pane, which uses them for its own tabs.
func TestTabCyclesPanesAndBracketsGoToThePane(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	run(m, m.key(press("]")))
	run(m, m.key(press("[")))
	if m.focus != 1 || !pulls.got(isKey("]")) || !pulls.got(isKey("[")) {
		t.Errorf("focus %d; want ] and [ to reach the pane that has the focus, and leave it there", m.focus)
	}
	run(m, m.key(press("tab")))
	if m.focus != 2 {
		t.Errorf("focus %d; want tab to move to the next pane", m.focus)
	}
	run(m, m.key(press("shift+tab")))
	run(m, m.key(press("shift+tab")))
	if m.focus != 0 {
		t.Errorf("focus %d; want shift+tab to move to the previous pane", m.focus)
	}
}

func TestPaneTitleChips(t *testing.T) {
	for _, width := range []int{40, 60, 100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m, pulls, _ := newFilterApp(t)
			pulls.chips = "@me · bug · -is:draft"
			m.Update(tea.WindowSizeMsg{Width: width, Height: 12})
			m.updateBadges()
			golden.RequireEqual(t, m.View().Content)
		})
	}
}

func TestSortKeyOpensTheSortTab(t *testing.T) {
	tests := []struct {
		name       string
		sorts      bool
		keys       []string
		wantTab    int
		wantNoTabs bool
	}{
		{name: "f opens the filters", sorts: true, keys: []string{"f"}, wantTab: 0},
		{name: "s opens the sort", sorts: true, keys: []string{"s"}, wantTab: 1},
		{name: "] switches to the sort", sorts: true, keys: []string{"f", "]"}, wantTab: 1},
		{name: "[ switches back", sorts: true, keys: []string{"s", "["}, wantTab: 0},
		{name: "f opens a list without a sort, with no tabs", keys: []string{"f"}, wantTab: -1, wantNoTabs: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, pulls, _ := newFilterApp(t)
			pulls.sorts = tt.sorts
			for _, k := range tt.keys {
				run(m, m.key(press(k)))
			}
			names, active := filterModal(t, m).Tabs()
			if active != tt.wantTab || (names == nil) != tt.wantNoTabs {
				t.Errorf("Tabs = %q, %d; want tab %d", names, active, tt.wantTab)
			}
			if pulls.got(isKey("s")) || pulls.got(isKey("f")) {
				t.Error("the section got the key too")
			}
		})
	}
}

// Applying from either tab applies the same query.
func TestFilterModalAppliesFromEitherTab(t *testing.T) {
	for _, k := range []string{"f", "s"} {
		m, pulls, _ := newFilterApp(t)
		pulls.sorts, pulls.query = true, "is:closed sort:comments-asc"
		run(m, m.key(press(k)))
		run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEnter}))
		if len(pulls.applied) != 1 || pulls.applied[0].Query != "is:closed sort:comments-asc" {
			t.Errorf("%s then enter applied %+v, want the query as it was", k, pulls.applied)
		}
	}
}

func TestSortKeyWithoutASortGoesToTheSection(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	run(m, m.key(press("s")))
	if m.topModal() != nil || !pulls.got(isKey("s")) {
		t.Error("s opened a modal for a list that can't be sorted, or didn't reach the section")
	}
}

func TestSortKeyCanBeRebound(t *testing.T) {
	cfg := config.Default()
	cfg.Keys.Set("pulls.sort", []string{"O"})
	m, pulls, _ := newFilterAppWith(t, cfg, 120, 40)
	pulls.sorts = true
	run(m, m.key(press("s")))
	if m.topModal() != nil {
		t.Fatal("s still opens the sort")
	}
	run(m, m.key(press("O")))
	if _, active := filterModal(t, m).Tabs(); active != 1 {
		t.Errorf("O opened tab %d, want the sort", active)
	}
}

// The tabs show in the top edge of the frame, at 80 columns too, where
// they shorten a long title.
func TestFilterModalFrame(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		width int
	}{{"f", "f", 80}, {"s", "s", 80}, {"s at 70", "s", 70}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, pulls, _ := newFilterAppWith(t, config.Default(), tt.width, 24)
			pulls.sorts = true
			run(m, m.key(press(tt.key)))
			golden.RequireEqual(t, m.View().Content)
		})
	}
}

// TestFilterCommands checks that filter and sort open the filter modal of
// the focused list on their tab, as their keys do, and say so where there
// is none to open.
func TestFilterCommands(t *testing.T) {
	tests := []struct {
		name string
		line string
		// sorts gives the list a sort, and ready a filter.
		sorts, ready bool
		// tab is the tab the modal opens on, if it opens, and toast the
		// text of the toast.
		tab   string
		toast string
	}{
		{name: "filter", line: "filter", ready: true, tab: "Filters"},
		{name: "sort", line: "sort", ready: true, sorts: true, tab: "Sort"},
		{name: "no sort", line: "sort", ready: true, toast: "Nothing here to sort."},
		{name: "no filter", line: "filter", toast: "Nothing here to filter."},
		{name: "an argument", line: "filter is:open", ready: true, toast: "The filter command takes no argument."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, pulls, _ := newFilterApp(t)
			pulls.ready, pulls.sorts = tt.ready, tt.sorts
			m.toast.SetDuration(0)
			m.toast.SetErrorDuration(0)
			runCommand(t, m, tt.line)
			if tt.tab == "" {
				if m.topModal() != nil {
					t.Errorf("modal %q opened", m.topModal().Title())
				}
			} else {
				tab := "Filters"
				if names, active := filterModal(t, m).Tabs(); active >= 0 {
					tab = names[active]
				}
				if tab != tt.tab {
					t.Errorf("tab %q, want %q", tab, tt.tab)
				}
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			if pulls.got(isKey("f")) || pulls.got(isKey("s")) {
				t.Error("a key reached the section")
			}
		})
	}
}
