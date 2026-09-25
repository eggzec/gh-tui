package actions

import (
	"errors"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPoll(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	poll := s.Poll(repo, 2)

	res, err := poll(t.Context())
	if err != nil || res.Changed {
		t.Errorf("poll = %+v, %v; want no change", res, err)
	}
	checkCalls(t, f, "GetRun octo-org/hello 2 if-none-match", "ListJobs octo-org/hello 2 attempt=1 cursor= per_page=100 if-none-match")

	// A step moves on: the jobs change, the run doesn't.
	f.change(func(f *fakeGitHub) {
		j := &f.jobs[2][1]
		j.Steps = append([]core.Step(nil), j.Steps...)
		j.Steps[1].Status, j.Steps[1].Conclusion, j.Steps[1].CompletedAt = completed, core.ConclusionSuccess, at.Add(time.Hour+2*time.Second)
	})
	if res, err := poll(t.Context()); err != nil || !res.Changed {
		t.Errorf("poll = %+v, %v; want a change", res, err)
	}
	f.take()
	if p, ok := s.CachedJobs(JobsQuery{Repo: repo, RunID: 2, Attempt: 1}); !ok || p.Items[1].Steps[1].Status != completed {
		t.Errorf("cached jobs = %+v, want the step completed", p)
	}

	// The run completes: the change is the last one reported.
	f.change(func(f *fakeGitHub) {
		f.runs[0].Status, f.runs[0].Conclusion = completed, core.ConclusionSuccess
		f.jobs[2][1] = doneJob(22, 2, core.ConclusionSuccess)
	})
	if res, err := poll(t.Context()); err != nil || !res.Changed {
		t.Errorf("poll = %+v, %v; want a change", res, err)
	}
	p, _ := s.CachedRuns(RunsQuery{Repo: repo})
	if !p.Items[0].Done() {
		t.Errorf("cached page shows run 2 %+v, want it completed", p.Items[0])
	}
	f.take()
	if res, err := poll(t.Context()); err != nil || res.Changed {
		t.Errorf("poll after completion = %+v, %v; want nothing", res, err)
	}
	checkCalls(t, f)
	// The jobs of the attempt that completed are kept for good.
	if _, err := s.Jobs(t.Context(), JobsQuery{Repo: repo, RunID: 2, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f)
}

func TestPollFirstReportsWhatWasntRead(t *testing.T) {
	f := newFake()
	s := New(f)
	if res, err := s.Poll(repo, 2)(t.Context()); err != nil || !res.Changed {
		t.Errorf("poll = %+v, %v; want a change, as nothing was read", res, err)
	}
	if _, ok := s.CachedRun(repo, 2); !ok {
		t.Error("the poll didn't cache the run")
	}
}

func TestPollFails(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	f.fail(errNotFound)
	if _, err := s.Poll(repo, 2)(t.Context()); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("poll = %v, want ErrNotFound", err)
	}
	if RunSyncKey(repo, 2) != "actions:octo-org/hello/run/2" {
		t.Errorf("RunSyncKey = %q", RunSyncKey(repo, 2))
	}
}

func TestPollChecks(t *testing.T) {
	f := newFake()
	pending := core.Checks{SHA: "abc", State: core.ChecksPending, Runs: []core.Check{
		{ID: 12, Name: "test", Status: core.RunInProgress},
		{ID: 13, Name: "lint", Status: completed, Conclusion: core.ConclusionSuccess},
	}}
	f.change(func(f *fakeGitHub) { f.checks["#5"] = pending })
	s := New(f)
	q := ChecksQuery{Repo: repo, Number: 5}
	if _, err := s.Checks(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	f.take()
	poll := s.PollChecks(q)

	if res, err := poll(t.Context()); err != nil || res.Changed {
		t.Errorf("poll = %+v, %v; want no change", res, err)
	}
	checkCalls(t, f, "PullChecks octo-org/hello #5")

	// The test fails: a change, cached, and the last poll that asks.
	f.change(func(f *fakeGitHub) {
		c := pending
		c.State = core.ChecksFailure
		c.Runs = []core.Check{{ID: 12, Name: "test", Status: completed, Conclusion: core.ConclusionFailure}, pending.Runs[1]}
		f.checks["#5"] = c
	})
	if res, err := poll(t.Context()); err != nil || !res.Changed {
		t.Errorf("poll = %+v, %v; want a change", res, err)
	}
	if c, ok := s.CachedChecks(q); !ok || c.Runs[0].Conclusion != core.ConclusionFailure {
		t.Errorf("cached checks = %+v, want the test failed", c)
	}
	f.take()
	if res, err := poll(t.Context()); err != nil || res.Changed {
		t.Errorf("poll once all are done = %+v, %v; want nothing", res, err)
	}
	checkCalls(t, f)
	if ChecksSyncKey(q) == ChecksSyncKey(ChecksQuery{Repo: repo, Number: 6}) || ChecksSyncKey(q) == SyncKey(repo) {
		t.Error("the sync keys of checks aren't their own")
	}
}

func TestPollChecksIsBounded(t *testing.T) {
	f := newFake()
	f.change(func(f *fakeGitHub) {
		f.checks["@abc"] = core.Checks{SHA: "abc", Statuses: []core.StatusContext{{Context: "ci/ext", State: "pending"}}}
	})
	s := New(f)
	poll := s.PollChecks(ChecksQuery{Repo: repo, SHA: "abc"})
	for range MaxChecksPolls + 5 {
		if _, err := poll(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.take()); n != MaxChecksPolls {
		t.Errorf("polled %d times, want at most %d", n, MaxChecksPolls)
	}
}

func TestPollChecksError(t *testing.T) {
	f := newFake()
	f.failCall("PullChecks", errors.New("boom"))
	if _, err := New(f).PollChecks(ChecksQuery{Repo: repo, Number: 5})(t.Context()); err == nil {
		t.Error("a failed read isn't an error")
	}
}
