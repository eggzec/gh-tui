package issues

import (
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSyncReloads(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	tests := []struct {
		name   string
		msg    ui.SyncMsg
		reload bool
	}{
		{"this repository", ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}, true},
		{"this repository in other case", ui.SyncMsg{Key: issuesvc.SyncKey(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"})}, true},
		{"another repository", ui.SyncMsg{Key: issuesvc.SyncKey(other)}, false},
		{"pull requests of this repository", ui.SyncMsg{Key: pullsvc.SyncKey(testRepo)}, false},
		{"failed poll", ui.SyncMsg{Key: issuesvc.SyncKey(testRepo), Err: errors.New("offline")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			s := started(t, svc, 80, 20)
			press(t, s, "down", "down")
			// Poll invalidated the cache, so the reload sees the change.
			svc.set(998, func(it *core.Issue) { it.Title = "Renamed on the server" })
			before := len(svc.listCalls())
			run(t, s, s.Update(tt.msg))

			want, title := 0, titles[2]
			if tt.reload {
				want, title = 1, "Renamed on the server"
			}
			if got := len(svc.listCalls()) - before; got != want {
				t.Errorf("listed %d pages, want %d", got, want)
			}
			if it, _ := s.list.Selected(); it.Number != 998 || it.Title != title {
				t.Errorf("selected = #%d %q, want #998 %q", it.Number, it.Title, title)
			}
			if len(svc.getCalls()) != 0 {
				t.Errorf("got issues %v with none open", svc.getCalls())
			}
		})
	}
}

func TestSyncWaitsForStart(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := newSection(t, svc, 80, 20)
	run(t, s, s.Update(ui.RepoMsg{Repo: testRepo}))
	run(t, s, s.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	if got := len(svc.listCalls()); got != 0 {
		t.Errorf("listed %d pages before the section started, want 0", got)
	}
}

func TestSyncRefreshesDetail(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := opened(t, svc, 12)
	svc.set(999, func(it *core.Issue) { it.Title = "Renamed on the server" })
	lists, comments := len(svc.listCalls()), len(svc.commentQueries)
	run(t, s, s.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	if !s.inDetail {
		t.Fatal("sync closed the issue")
	}
	if got := svc.getCalls(); len(got) != 2 {
		t.Errorf("Get calls = %v, want two", got)
	}
	if s.issue.Title != "Renamed on the server" {
		t.Errorf("open issue = %q, want it renamed", s.issue.Title)
	}
	if got := len(svc.commentQueries); got <= comments {
		t.Errorf("comment pages read = %d, want more than %d", got, comments)
	}
	if got := len(svc.listCalls()); got != lists+1 {
		t.Errorf("listed %d pages, want %d, so the list is current on the way back", got, lists+1)
	}
}
