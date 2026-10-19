package notifications

import (
	"errors"
	"maps"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var (
	repoA = core.RepoRef{Owner: "o", Name: "a"}

	subjectPull    = core.Notification{ID: "1", Repo: repoA, Subject: core.Subject{Type: core.SubjectPullRequest, Number: 7}}
	subjectIssue   = core.Notification{ID: "2", Repo: repoA, Subject: core.Subject{Type: core.SubjectIssue, Number: 8}}
	subjectRelease = core.Notification{ID: "3", Repo: repoA, Subject: core.Subject{Type: core.SubjectRelease, ReleaseID: 9}}
	// A discussion that GitHub gave no number is not asked for.
	subjectNoNumber = core.Notification{ID: "4", Repo: repoA, Subject: core.Subject{Type: core.SubjectDiscussion}}

	statesPage = page{Items: []core.Notification{subjectPull, subjectIssue, subjectRelease, subjectNoNumber, subjectPull}}

	refPull  = core.SubjectRef{Repo: repoA, Number: 7, Type: core.SubjectPullRequest}
	refIssue = core.SubjectRef{Repo: repoA, Number: 8, Type: core.SubjectIssue}

	stateMerged = core.SubjectState{State: core.StateMerged}
	stateClosed = core.SubjectState{State: core.StateClosed, Reason: core.ReasonNotPlanned}
)

// statesAPI serves statesPage, and the states of its pull request and issue.
func statesAPI(asked *[][]core.SubjectRef) *fakeAPI {
	return &fakeAPI{
		list: func(core.NotificationFilter, int, string, github.Conditional) (page, github.Response, error) {
			return statesPage, github.Response{}, nil
		},
		states: func(subjects []core.SubjectRef) (map[core.SubjectRef]core.SubjectState, error) {
			*asked = append(*asked, subjects)
			return map[core.SubjectRef]core.SubjectState{refPull: stateMerged, refIssue: stateClosed}, nil
		},
	}
}

func TestStatesAskedOncePerNetworkLoad(t *testing.T) {
	var asked [][]core.SubjectRef
	api := statesAPI(&asked)
	s := New(api)
	if got := s.CachedStates(statesPage.Items); len(got) != 0 {
		t.Fatalf("CachedStates before any read = %v, want none", got)
	}
	p, err := s.List(t.Context(), ListQuery{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := api.stateCalls.Load(); got != 0 {
		t.Fatalf("List asked the states %d times, want it to leave that to States", got)
	}

	want := map[core.SubjectRef]core.SubjectState{refPull: stateMerged, refIssue: stateClosed}
	got, err := s.States(t.Context(), p.Items)
	if err != nil || !maps.Equal(got, want) {
		t.Fatalf("States = %v, %v; want %v", got, err, want)
	}
	if len(asked) != 1 || len(asked[0]) != 2 {
		t.Fatalf("asked = %v, want one batch of the pull request and the issue", asked)
	}

	// The page is cached, so the second read asks nothing.
	if got, err = s.States(t.Context(), p.Items); err != nil || !maps.Equal(got, want) {
		t.Fatalf("second States = %v, %v; want %v", got, err, want)
	}
	if got := s.CachedStates(p.Items); !maps.Equal(got, want) {
		t.Errorf("CachedStates = %v, want %v", got, want)
	}
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("List again: %v", err)
	}
	if _, err := s.States(t.Context(), p.Items); err != nil || len(asked) != 1 {
		t.Fatalf("States after a cached page asked %d times, want 1 in all (%v)", len(asked), err)
	}

	// A page that goes to GitHub again has its states asked again.
	s.Invalidate()
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("List revalidated: %v", err)
	}
	if _, err := s.States(t.Context(), p.Items); err != nil || len(asked) != 2 {
		t.Fatalf("States after a revalidated page asked %d times, want 2 (%v)", len(asked), err)
	}
}

func TestStatesCachedAcrossSessions(t *testing.T) {
	store := openStore(t)
	var asked [][]core.SubjectRef
	first := New(statesAPI(&asked), WithStore(store))
	if _, err := first.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := first.States(t.Context(), statesPage.Items); err != nil {
		t.Fatalf("States: %v", err)
	}

	asked = nil
	api := statesAPI(&asked)
	second := New(api, WithStore(store))
	// Nothing is in memory until the store is read.
	if got := second.CachedStates(statesPage.Items); len(got) != 0 {
		t.Fatalf("CachedStates of a new session = %v, want none before States reads the store", got)
	}
	got, err := second.States(t.Context(), statesPage.Items)
	want := map[core.SubjectRef]core.SubjectState{refPull: stateMerged, refIssue: stateClosed}
	if err != nil || !maps.Equal(got, want) {
		t.Fatalf("States of a new session = %v, %v; want the kept %v", got, err, want)
	}
	if len(asked) != 0 {
		t.Errorf("asked = %v, want no request for a page that was not read from GitHub", asked)
	}
	if got := second.CachedStates(statesPage.Items); !maps.Equal(got, want) {
		t.Errorf("CachedStates after States = %v, want %v", got, want)
	}
}

func TestStatesFailureKeepsWhatIsHeld(t *testing.T) {
	var asked [][]core.SubjectRef
	api := statesAPI(&asked)
	s := New(api)
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	boom := errors.New("boom")
	api.states = func([]core.SubjectRef) (map[core.SubjectRef]core.SubjectState, error) { return nil, boom }
	got, err := s.States(t.Context(), statesPage.Items)
	if !errors.Is(err, boom) || len(got) != 0 {
		t.Fatalf("States = %v, %v; want the error and nothing", got, err)
	}
	if got := s.CachedStates(statesPage.Items); len(got) != 0 {
		t.Errorf("CachedStates after a failure = %v, want none", got)
	}
}

func TestStatesRefusedAsksNothing(t *testing.T) {
	var asked [][]core.SubjectRef
	api := statesAPI(&asked)
	s := New(api, WithAccess(tokenWith(noScope, true)))
	got, err := s.States(t.Context(), statesPage.Items)
	if err != nil || len(got) != 0 || len(asked) != 0 {
		t.Fatalf("States = %v, %v after %d requests; want none while the token may not read", got, err, len(asked))
	}
}
