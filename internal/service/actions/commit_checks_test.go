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
