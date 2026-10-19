package actions

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func commitRun(status core.RunStatus, conclusion core.Conclusion) core.CommitChecks {
	return core.CommitChecks{SHA: "abc", Runs: []core.CommitCheck{{ID: 1, Name: "build", Status: status, Conclusion: conclusion}}}
}

func TestCommitChecks(t *testing.T) {
	f := newFake()
	f.commits["abc"] = commitRun(core.RunCompleted, core.ConclusionSuccess)
	s := New(f)
	q := CommitChecksQuery{Repo: repo, SHA: "ABC"}
	if _, ok := s.CachedCommitChecks(q); ok || s.FreshCommitChecks(q) {
		t.Fatal("checks are cached before a read")
	}
	got, err := s.CommitChecks(t.Context(), q)
	if err != nil || len(got.Runs) != 1 || got.Runs[0].Name != "build" {
		t.Fatalf("CommitChecks = %+v, %v", got, err)
	}
	if c, ok := s.CachedCommitChecks(q); !ok || len(c.Runs) != 1 || !s.FreshCommitChecks(q) {
		t.Errorf("CachedCommitChecks = %+v, %v; want the checks, fresh", c, ok)
	}
	if _, err := s.CommitChecks(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "CommitChecksAll octo-org/hello @abc notes=false")

	// Annotations are another read.
	if _, ok := s.CachedCommitChecks(CommitChecksQuery{Repo: repo, SHA: "abc", Notes: true}); ok {
		t.Error("checks without notes serve a read with them")
	}
	if _, err := s.CommitChecks(t.Context(), CommitChecksQuery{Repo: repo, SHA: "abc", Notes: true}); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "CommitChecksAll octo-org/hello @abc notes=true")

	// A change of the repository makes them stale.
	s.Invalidate(repo)
	if s.FreshCommitChecks(q) {
		t.Error("checks are fresh after Invalidate")
	}
	if _, err := s.CommitChecks(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "CommitChecksAll octo-org/hello @abc notes=false")
}

// A read with Again asks GitHub although the cache holds the checks fresh, and
// leaves the new ones where the reads after it find them.
func TestCommitChecksAgain(t *testing.T) {
	f := newFake()
	f.commits["abc"] = commitRun(core.RunInProgress, "")
	s := New(f)
	q := CommitChecksQuery{Repo: repo, SHA: "abc", Notes: true}
	if _, err := s.CommitChecks(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "CommitChecksAll octo-org/hello @abc notes=true")
	f.commits["abc"] = commitRun(core.RunCompleted, core.ConclusionFailure)
	if got, err := s.CommitChecks(t.Context(), q); err != nil || got.Runs[0].Status != core.RunInProgress {
		t.Fatalf("a read without Again = %+v, %v; want the cached checks", got, err)
	}
	q.Again = true
	got, err := s.CommitChecks(t.Context(), q)
	if err != nil || got.Runs[0].Status != core.RunCompleted {
		t.Fatalf("a read with Again = %+v, %v; want the new checks", got, err)
	}
	checkCalls(t, f, "CommitChecksAll octo-org/hello @abc notes=true")
	q.Again = false
	if got, _ := s.CachedCommitChecks(q); got.Runs[0].Status != core.RunCompleted {
		t.Errorf("cached = %+v, want the checks of the read with Again", got)
	}
}

// Checks that completed while the repository was invalidated are not kept
// for long: the change may be the rerun that made them pending again.
func TestCommitChecksInvalidatedDuringRead(t *testing.T) {
	f := newFake()
	f.commits["abc"] = commitRun(core.RunInProgress, "")
	s := New(f)
	q := CommitChecksQuery{Repo: repo, SHA: "abc"}
	// The cache keeps an entry of the commit, made stale by a change, which
	// the next read replaces while the repository changes again.
	if _, err := s.CommitChecks(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	s.Invalidate(repo)
	started, release := make(chan struct{}), make(chan struct{})
	f.change(func(f *fakeGitHub) {
		f.commits["abc"] = commitRun(core.RunCompleted, core.ConclusionSuccess)
		f.commitHold = func() {
			close(started)
			<-release
		}
	})
	read := make(chan error, 1)
	go func() {
		_, err := s.CommitChecks(t.Context(), q)
		read <- err
	}()
	<-started
	s.Invalidate(repo)
	close(release)
	if err := <-read; err != nil {
		t.Fatal(err)
	}
	if s.FreshCommitChecks(q) {
		t.Error("checks read across an Invalidate are fresh")
	}
}

func TestCommitChecksFails(t *testing.T) {
	f := newFake()
	s := New(f)
	if _, err := s.CommitChecks(t.Context(), CommitChecksQuery{Repo: repo}); err == nil {
		t.Error("a query without a commit is read")
	}
	f.change(func(f *fakeGitHub) { f.errs["CommitChecksAll"] = errNotFound })
	_, err := s.CommitChecks(t.Context(), CommitChecksQuery{Repo: repo, SHA: "abc"})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("CommitChecks = %v, want ErrNotFound", err)
	}
	if _, ok := s.CachedCommitChecks(CommitChecksQuery{Repo: repo, SHA: "abc"}); ok {
		t.Error("a failed read is cached")
	}
}

// While a check is pending the entry stays fresh for the short live TTL;
// once all completed, for the long one.
func TestCommitChecksTTL(t *testing.T) {
	const live, done = 10 * time.Second, 5 * time.Minute
	q := CommitChecksQuery{Repo: repo, SHA: "abc"}

	t.Run("pending", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newFake()
			f.commits["abc"] = commitRun(core.RunInProgress, "")
			s := New(f, WithLiveTTL(live), WithTTL(done))
			if _, err := s.CommitChecks(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			time.Sleep(live - time.Second)
			if !s.FreshCommitChecks(q) {
				t.Error("pending checks are stale before the live TTL")
			}
			f.take()
			time.Sleep(2 * time.Second)
			if s.FreshCommitChecks(q) {
				t.Error("pending checks are fresh after the live TTL")
			}
			// They are read again, and finish.
			f.change(func(f *fakeGitHub) { f.commits["abc"] = commitRun(core.RunCompleted, core.ConclusionSuccess) })
			got, err := s.CommitChecks(t.Context(), q)
			if err != nil || got.Pending() {
				t.Fatalf("CommitChecks = %+v, %v; want them completed", got, err)
			}
			checkCalls(t, f, "CommitChecksAll octo-org/hello @abc notes=false")
		})
	})

	t.Run("completed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			f := newFake()
			f.commits["abc"] = commitRun(core.RunCompleted, core.ConclusionFailure)
			s := New(f, WithLiveTTL(live), WithTTL(done))
			if _, err := s.CommitChecks(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			f.take()
			time.Sleep(done - time.Second)
			if !s.FreshCommitChecks(q) {
				t.Error("completed checks are stale before the long TTL")
			}
			if _, err := s.CommitChecks(t.Context(), q); err != nil {
				t.Fatal(err)
			}
			checkCalls(t, f)
			time.Sleep(2 * time.Second)
			if s.FreshCommitChecks(q) {
				t.Error("completed checks are fresh after the long TTL")
			}
		})
	})
}
