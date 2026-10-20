package notifications

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// stateInbox is threads of every kind of subject state, with the states.
func stateInbox() (items []core.Notification, states map[core.SubjectRef]core.SubjectState) {
	const repo = "o/a"
	states = map[core.SubjectRef]core.SubjectState{}
	add := func(id string, typ core.SubjectType, st core.SubjectState, known bool) {
		n := thread(id, repo, typ, "t"+id, "mention", true, time.Minute)
		items = append(items, n)
		if known {
			states[subjectRef(n)] = st
		}
	}
	open, closed := core.SubjectState{State: core.StateOpen}, core.SubjectState{State: core.StateClosed}
	add("1", core.SubjectPullRequest, open, true)
	add("2", core.SubjectPullRequest, core.SubjectState{State: core.StateOpen, Draft: true}, true)
	add("3", core.SubjectPullRequest, core.SubjectState{State: core.StateMerged}, true)
	add("4", core.SubjectPullRequest, closed, true)
	add("5", core.SubjectIssue, open, true)
	add("6", core.SubjectIssue, core.SubjectState{State: core.StateClosed, Reason: "not_planned"}, true)
	add("7", core.SubjectDiscussion, open, true)
	add("8", core.SubjectDiscussion, core.SubjectState{State: core.StateOpen, Answered: true}, true)
	add("9", core.SubjectDiscussion, closed, true)
	add("10", core.SubjectRelease, core.SubjectState{}, false)
	add("11", core.SubjectCheckSuite, core.SubjectState{}, false)
	add("12", core.SubjectIssue, core.SubjectState{}, false)
	return items, states
}

func TestFilterKeepsByState(t *testing.T) {
	t.Parallel()
	items, states := stateInbox()
	tests := []struct {
		query string
		want  []string
	}{
		{"", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}},
		{"state:open", []string{"1", "5", "7"}},
		{"state:draft", []string{"2"}},
		{"state:merged", []string{"3"}},
		{"state:closed", []string{"4", "6", "9"}},
		{"state:answered", []string{"8"}},
		{"state:open,draft", []string{"1", "2", "5", "7"}},
		{"state:open state:merged", []string{"1", "3", "5", "7"}},
		{"state:open type:issue", []string{"5"}},
		{"state:Closed", []string{"4", "6", "9"}},
	}
	for _, tt := range tests {
		f := parseFilter(tt.query)
		var got []string
		for _, n := range f.apply(items) {
			st, ok := states[subjectRef(n)]
			if f.keepsState(n.Subject.Type, st, ok) {
				got = append(got, n.ID)
			}
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("filter %q keeps %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestFilterStateRoundTrip(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	for _, q := range []string{"is:unread state:open state:draft", "is:unread type:pr state:merged"} {
		press(t, s, showAll+q)
		f, _ := s.Filter()
		if got := filterform.New(f.Spec, filterform.WithQuery(f.Query)).Query(); got != q {
			t.Errorf("the form of %q writes %q", q, got)
		}
	}
	f, _ := s.Filter()
	field := f.Spec.Fields[len(f.Spec.Fields)-1]
	if field.Key != "state" || field.Kind != filterform.Multi || !field.Each || field.Qualifier != "state" || len(field.Options) != 5 {
		t.Errorf("the last field is %+v, want the state picked among five, one qualifier each", field)
	}
}

func TestFilterStateChips(t *testing.T) {
	t.Parallel()
	f := parseFilter("is:unread state:open state:Draft type:pr")
	if got, want := f.chips(", "), "state open, state draft, pr"; got != want {
		t.Errorf("chips = %q, want %q", got, want)
	}
}

// stateRows returns the IDs of the threads the list holds, in order.
func stateRows(s *Section) []string {
	var ids []string
	for i := range s.feed.Len() {
		if n, ok := s.feed.LoadedItem(i); ok {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

func stateSection(tb testing.TB, st States) *Section {
	tb.Helper()
	items, _ := stateInbox()
	s := New(tb.Context(), newFake(items...), config.Default().Keys, WithNow(func() time.Time { return now }), WithStates(st))
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	s.SetSize(100, 30)
	s.Focus()
	run(tb, s, s.Init())
	return s
}

func TestSectionFiltersByState(t *testing.T) {
	t.Parallel()
	_, states := stateInbox()
	s := stateSection(t, &fakeStates{cached: states})
	if got := len(stateRows(s)); got != 12 {
		t.Fatalf("%d rows without a state filter, want all 12", got)
	}
	run(t, s, s.setFilter("is:unread state:open"))
	if got, want := stateRows(s), []string{"1", "5", "7"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	run(t, s, s.setFilter("is:unread state:merged state:answered"))
	if got, want := stateRows(s), []string{"3", "8"}; !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestSectionRegroupsByStateWhenTheyArrive(t *testing.T) {
	t.Parallel()
	_, states := stateInbox()
	// The read of the states fails, so every row is in the group of none.
	s := stateSection(t, &fakeStates{err: errors.New("boom")})
	run(t, s, s.setGroups(levels("state")))
	if view := ansi.Strip(s.View()); !strings.Contains(view, "none") {
		t.Fatalf("no group of none before the states arrived:\n%s", view)
	}
	run(t, s, s.Update(statesMsg{states: states}))
	view := ansi.Strip(s.View())
	if !strings.Contains(view, "Open") {
		t.Errorf("no group of open rows after the states arrived:\n%s", view)
	}
}

func TestSectionHidesUnknownStatesUntilTheyArrive(t *testing.T) {
	t.Parallel()
	_, states := stateInbox()
	// The read of the states fails, so no row has one.
	s := stateSection(t, &fakeStates{err: errors.New("boom")})
	run(t, s, s.setFilter("is:unread state:open"))
	if got := stateRows(s); len(got) != 0 {
		t.Fatalf("rows = %v, want none while no state is known", got)
	}

	// Late states show the open rows, and the others stay hidden.
	run(t, s, s.Update(statesMsg{states: states}))
	if got, want := stateRows(s), []string{"1", "5", "7"}; !slices.Equal(got, want) {
		t.Errorf("rows after the states arrived = %v, want %v", got, want)
	}

	// A state that changes takes its row out.
	pull := core.Notification{Repo: core.RepoRef{Owner: "o", Name: "a"}, Subject: core.Subject{Type: core.SubjectPullRequest, Number: 1}}
	run(t, s, s.Update(statesMsg{states: map[core.SubjectRef]core.SubjectState{subjectRef(pull): {State: core.StateMerged}}}))
	if got, want := stateRows(s), []string{"5", "7"}; !slices.Equal(got, want) {
		t.Errorf("rows after a state changed = %v, want %v", got, want)
	}
}
