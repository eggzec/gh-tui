package actions

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/internal/core"
)

// slowLogs is a fake whose job log download takes until release is closed,
// and ends at once if its context is done, as a download does.
type slowLogs struct {
	*fakeGitHub
	downloads atomic.Int32
	release   chan struct{}
}

func (s *slowLogs) JobLog(ctx context.Context, r core.RepoRef, jobID, limit int64) (text []byte, truncated bool, err error) {
	s.downloads.Add(1)
	select {
	case <-s.release:
		return s.fakeGitHub.JobLog(context.WithoutCancel(ctx), r, jobID, limit)
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

type logResult struct {
	log core.Log
	err error
}

func readLog(ctx context.Context, s *Service) <-chan logResult {
	c := make(chan logResult, 1)
	go func() {
		l, err := s.Log(ctx, repo, 12)
		c <- logResult{l, err}
	}()
	return c
}

// A read of a log that the modal opens joins the read ahead of it that is
// still downloading, even if the read ahead is canceled meanwhile, so the
// log is downloaded once and shown whole.
func TestLogJoinsReadAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &slowLogs{fakeGitHub: newFake(), release: make(chan struct{})}
		s := New(f)

		aheadCtx, cancelAhead := context.WithCancel(t.Context())
		ahead := readLog(aheadCtx, s)
		synctest.Wait()
		modal := readLog(t.Context(), s)
		synctest.Wait()

		// The cursor reaches the row, or the window moves on.
		cancelAhead()
		synctest.Wait()
		if r := <-ahead; r.err == nil {
			t.Error("the canceled read ahead returned a log, want its context's error")
		}
		close(f.release)
		synctest.Wait()

		r := <-modal
		if r.err != nil {
			t.Fatalf("modal's Log: %v", r.err)
		}
		checkLog(t, r.log)
		if n := f.downloads.Load(); n != 1 {
			t.Errorf("the log was downloaded %d times, want once", n)
		}
		if _, ok := s.CachedLog(repo, 12); !ok {
			t.Error("the log isn't kept after the download")
		}
	})
}

// A read ahead that nobody joined and that is canceled keeps no part of the
// log, and the next read downloads it whole.
func TestLogReadAheadCanceledKeepsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &slowLogs{fakeGitHub: newFake(), release: make(chan struct{})}
		s := New(f)

		aheadCtx, cancelAhead := context.WithCancel(t.Context())
		ahead := readLog(aheadCtx, s)
		synctest.Wait()
		cancelAhead()
		synctest.Wait()
		<-ahead
		if _, ok := s.CachedLog(repo, 12); ok {
			t.Fatal("a canceled download left a log in memory")
		}

		close(f.release)
		r := <-readLog(t.Context(), s)
		if r.err != nil {
			t.Fatalf("Log after the cancel: %v", r.err)
		}
		checkLog(t, r.log)
		if n := f.downloads.Load(); n != 2 {
			t.Errorf("downloads = %d, want the canceled one and the whole one", n)
		}
	})
}
