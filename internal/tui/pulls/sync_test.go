package pulls

import (
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSyncReloads(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	tests := []struct {
		name   string
		msg    ui.SyncMsg
		reload bool
	}{
		{"this repository", ui.SyncMsg{Key: pulls.SyncKey(repo)}, true},
		{"this repository in other case", ui.SyncMsg{Key: pulls.SyncKey(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"})}, true},
		{"another repository", ui.SyncMsg{Key: pulls.SyncKey(other)}, false},
		{"issues of this repository", ui.SyncMsg{Key: issues.SyncKey(repo)}, false},
		{"failed poll", ui.SyncMsg{Key: pulls.SyncKey(repo), Err: errors.New("offline")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := started(t, svc, 80, 20)
			n := len(svc.listed())
			drain(t, s, s.Update(tt.msg))
			want := n
			if tt.reload {
				want++
			}
			if got := len(svc.listed()); got != want {
				t.Errorf("listed %d times, want %d", got, want)
			}
			if len(svc.got()) != 0 {
				t.Errorf("got details %v with no pull request open", svc.got())
			}
		})
	}
}

func TestSyncWaitsForStart(t *testing.T) {
	svc := newFakeService()
	s := newTest(t, svc, 80, 20)
	drain(t, s, s.Update(ui.RepoMsg{Repo: repo}))
	drain(t, s, s.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if got := len(svc.listed()); got != 0 {
		t.Errorf("listed %d times before the section started, want 0", got)
	}
}

func TestSyncRefreshesDetail(t *testing.T) {
	svc := newFakeService()
	s := started(t, svc, 80, 20)
	press(t, s, "enter")
	commentReads := func() int {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.comments)
	}
	lists, gets, comments := len(svc.listed()), len(svc.got()), commentReads()
	drain(t, s, s.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if s.modal() == nil {
		t.Fatal("sync closed the modal")
	}
	if got := len(svc.got()); got != gets+1 {
		t.Errorf("got the detail %d times, want %d", got, gets+1)
	}
	if got := commentReads(); got <= comments {
		t.Errorf("comments read %d times, want more than %d", got, comments)
	}
	if got := len(svc.listed()); got != lists+1 {
		t.Errorf("listed %d times, want %d, so the list is current on the way back", got, lists+1)
	}
}

// A modal's change reloads the list of its repository, however either
// spells it.
func TestChangedReloads(t *testing.T) {
	tests := []struct {
		name   string
		repo   core.RepoRef
		reload bool
	}{
		{"this repository", repo, true},
		{"this repository in other case", core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}, true},
		{"another repository", core.RepoRef{Owner: "cli", Name: "cli"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := started(t, svc, 80, 20)
			n := len(svc.listed())
			drain(t, s, s.Update(changedMsg{repo: tt.repo}))
			want := n
			if tt.reload {
				want++
			}
			if got := len(svc.listed()); got != want {
				t.Errorf("listed %d times, want %d", got, want)
			}
		})
	}
}
