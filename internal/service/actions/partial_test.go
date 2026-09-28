package actions

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Lines of the log of job 22, in progress.
const (
	setUpLine = "2026-09-22T11:00:00.1Z setting up\n"
	testLine  = "2026-09-22T11:00:01.5Z ##[group]Run go test\n"
	okLine    = "2026-09-22T11:00:02Z ok\n"
)

// clock sets the clock of s to one that the returned function moves on.
func clock(s *Service) (tick func(d time.Duration)) {
	now := at.Add(time.Hour)
	s.now = func() time.Time { return now }
	return func(d time.Duration) { now = now.Add(d) }
}

func lineTexts(l core.Log) []string {
	texts := make([]string, len(l.Lines))
	for i, ln := range l.Lines {
		texts[i] = ln.Text
	}
	return texts
}

// logReads counts the reads of partial logs among calls.
func logReads(calls []string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, "JobLogFrom ") {
			n++
		}
	}
	return n
}

const partialCall = "JobLogFrom octo-org/hello 22 from="

func TestPartialLog(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	tick := clock(s)
	stop := s.WatchLog(repo, 2, 22)
	defer stop()
	// The storage holds a line and a half.
	f.change(func(f *fakeGitHub) { f.partial[22] = setUpLine + testLine[:10] })

	l, err := s.PartialLog(t.Context(), repo, 22)
	if err != nil {
		t.Fatalf("PartialLog: %v", err)
	}
	if got := lineTexts(l.Log); len(got) != 1 || got[0] != "setting up" || l.Gen != 1 || !l.At.Equal(s.now()) {
		t.Errorf("partial log = %q gen %d at %v, want its whole line", got, l.Gen, l.At)
	}
	checkCalls(t, f, partialCall+"0 limit=2097152")

	// The next read asks again from the start of the last line, to tell
	// that the log goes on from it, and appends.
	f.change(func(f *fakeGitHub) { f.partial[22] = setUpLine + testLine + okLine })
	tick(time.Second)
	again, err := s.PartialLog(t.Context(), repo, 22)
	if err != nil {
		t.Fatal(err)
	}
	if got := lineTexts(again.Log); len(got) != 3 || again.Gen != 1 || !again.At.After(l.At) {
		t.Errorf("partial log = %q gen %d, want 3 lines of the same gen, read later", got, again.Gen)
	}
	if again.Lines[1].Step != 2 {
		t.Errorf("line %q of step %d, want step 2", again.Lines[1].Text, again.Lines[1].Step)
	}
	checkCalls(t, f, partialCall+"0 limit=2097152")
	if c, ok := s.CachedPartialLog(repo, 22); !ok || len(c.Lines) != 3 {
		t.Errorf("cached partial log = %d lines, %v; want the 3 read", len(c.Lines), ok)
	}

	// Nothing added: the log stays as it was, checked later.
	tick(time.Second)
	same, _ := s.PartialLog(t.Context(), repo, 22)
	if len(same.Lines) != 3 || same.Gen != 1 || !same.At.Equal(s.now()) {
		t.Errorf("unchanged partial log = %d lines at %v, want as before, checked now", len(same.Lines), same.At)
	}

	// The log started over, so it is read again from its start.
	f.change(func(f *fakeGitHub) { f.partial[22] = okLine + okLine + okLine + okLine })
	restarted, _ := s.PartialLog(t.Context(), repo, 22)
	if got := lineTexts(restarted.Log); len(got) != 4 || got[0] != "ok" || restarted.Gen != 2 {
		t.Errorf("restarted partial log = %q gen %d, want its 4 lines in gen 2", got, restarted.Gen)
	}

	// Once no view watches it, nothing is kept.
	stop()
	if _, ok := s.CachedPartialLog(repo, 22); ok {
		t.Error("an unwatched partial log is kept")
	}
}

// A long log is read on from the last bytes read.
func TestPartialLogOverlaps(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	defer s.WatchLog(repo, 2, 22)()
	long := strings.Repeat(setUpLine, 20)
	f.change(func(f *fakeGitHub) { f.partial[22] = long })
	if _, err := s.PartialLog(t.Context(), repo, 22); err != nil {
		t.Fatal(err)
	}
	f.change(func(f *fakeGitHub) { f.partial[22] = long + okLine })
	f.take()
	l, err := s.PartialLog(t.Context(), repo, 22)
	if err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, partialCall+strconv.Itoa(len(long)-partialOverlap)+" limit=2097152")
	if len(l.Lines) != 21 || l.Lines[20].Text != "ok" || l.Gen != 1 {
		t.Errorf("partial log of %d lines, gen %d, want the line added", len(l.Lines), l.Gen)
	}
}

func TestPartialLogPending(t *testing.T) {
	for _, err := range []error{nil, errNotFound} {
		f := newFake()
		s := primed(t, f)
		if err != nil {
			f.failCall("JobLogFrom", err)
		}
		if _, got := s.PartialLog(t.Context(), repo, 22); !errors.Is(got, core.ErrLogPending) {
			t.Errorf("PartialLog with %v = %v, want ErrLogPending", err, got)
		}
	}
}

// A partial log larger than the limit keeps its end, down to 3/4 of the
// limit, so that the lines after it don't move it again at once.
func TestPartialLogLimit(t *testing.T) {
	f := newFake()
	s := New(f, WithLogLimit(int64(4*len(okLine))))
	defer s.WatchLog(repo, 2, 22)()
	read := func(lines int) core.PartialLog {
		t.Helper()
		f.change(func(f *fakeGitHub) { f.partial[22] = strings.Repeat(okLine, lines) })
		l, err := s.PartialLog(t.Context(), repo, 22)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	if l := read(4); len(l.Lines) != 4 || l.Truncated || l.Gen != 1 {
		t.Fatalf("partial log of %d lines, truncated %v, gen %d; want the 4 within the limit", len(l.Lines), l.Truncated, l.Gen)
	}
	if l := read(5); len(l.Lines) != 3 || !l.Truncated || l.Gen != 2 {
		t.Errorf("partial log of %d lines, truncated %v, gen %d; want its last 3, truncated, in gen 2", len(l.Lines), l.Truncated, l.Gen)
	}
	if l := read(6); len(l.Lines) != 4 || l.Gen != 2 {
		t.Errorf("partial log of %d lines, gen %d; want the line appended in gen 2", len(l.Lines), l.Gen)
	}
}

func TestPollReadsWatchedLogs(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	tick := clock(s)
	poll := s.Poll(repo, 2)
	f.change(func(f *fakeGitHub) { f.partial[22], f.partial[21] = setUpLine, setUpLine })

	// Unwatched, no log is read.
	if res, err := poll(t.Context()); err != nil || res.Changed {
		t.Errorf("poll = %+v, %v; want no change", res, err)
	}
	checkCalls(t, f, "GetRun octo-org/hello 2 if-none-match", "ListJobs octo-org/hello 2 attempt=1 cursor= per_page=100 if-none-match")

	// Watched, the log of the job in progress is read, and not that of the
	// job that completed; a log that grew is a change.
	stop := s.WatchLog(repo, 2, 22)
	defer s.WatchLog(repo, 2, 21)()
	if res, err := poll(t.Context()); err != nil || !res.Changed {
		t.Errorf("poll = %+v, %v; want a change", res, err)
	}
	if calls := f.take(); len(calls) != 3 || calls[2] != partialCall+"0 limit=2097152" {
		t.Errorf("calls = %q, want the log of job 22 read after the jobs", calls)
	}
	if l, ok := s.CachedPartialLog(repo, 22); !ok || len(l.Lines) != 1 {
		t.Errorf("cached partial log = %+v, %v; want the line read", l, ok)
	}
	// A log that didn't grow is no change within the minute, and the runs
	// stay as they were.
	tick(10 * time.Second)
	if res, err := poll(t.Context()); err != nil || res.Changed {
		t.Errorf("poll = %+v, %v; want no change", res, err)
	}
	if calls := f.take(); logReads(calls) != 1 {
		t.Errorf("calls = %q, want the log read", calls)
	}
	// Once the minute it was checked at moves on, the views show it.
	tick(time.Minute)
	if res, err := poll(t.Context()); err != nil || !res.Changed {
		t.Errorf("poll a minute on = %+v, %v; want a change", res, err)
	}
	if l, _ := s.CachedPartialLog(repo, 22); !l.At.Equal(s.now()) {
		t.Errorf("partial log as of %v, want as of now", l.At)
	}
	f.take()
	// A log that fails to read waits for a later poll.
	tick(time.Minute)
	f.failCall("JobLogFrom", errDial)
	if res, err := poll(t.Context()); err != nil || res.Changed {
		t.Errorf("poll with the storage down = %+v, %v; want no change and no error", res, err)
	}
	f.failCall("JobLogFrom", nil)
	f.take()

	stop()
	tick(time.Minute)
	if _, err := poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "GetRun octo-org/hello 2 if-none-match", "ListJobs octo-org/hello 2 attempt=1 cursor= per_page=100 if-none-match")
}

// A log that GitHub doesn't publish yet, or that doesn't grow, is read
// less and less often, 10, 20, 40 and then 60 seconds apart, and again at
// every poll once it grew.
func TestPollBacksOff(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	tick := clock(s)
	poll := s.Poll(repo, 2)
	defer s.WatchLog(repo, 2, 22)()
	reads := func(polls int) int {
		t.Helper()
		f.take()
		n := 0
		for range polls {
			if _, err := poll(t.Context()); err != nil {
				t.Fatal(err)
			}
			n += logReads(f.take())
			tick(10 * time.Second)
		}
		return n
	}
	// Pending: at 0, 10, 30, 70 and 130 seconds.
	if n := reads(14); n != 5 {
		t.Errorf("read a pending log %d times in 14 polls, want 5", n)
	}
	f.change(func(f *fakeGitHub) { f.partial[22] = setUpLine })
	// Due at 190 seconds, and then read as it grows at every poll.
	reads(6)
	for i := range 3 {
		f.change(func(f *fakeGitHub) { f.partial[22] += okLine })
		if n := reads(1); n != 1 {
			t.Errorf("poll %d read a growing log %d times, want once", i, n)
		}
	}
}

// A poll right after a view read the log leaves it.
func TestPollLeavesAFreshLog(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	clock(s)
	defer s.WatchLog(repo, 2, 22)()
	f.change(func(f *fakeGitHub) { f.partial[22] = setUpLine })
	if _, err := s.PartialLog(t.Context(), repo, 22); err != nil {
		t.Fatal(err)
	}
	f.take()
	if _, err := s.Poll(repo, 2)(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "GetRun octo-org/hello 2 if-none-match", "ListJobs octo-org/hello 2 attempt=1 cursor= per_page=100 if-none-match")
}

// A queued job has no log to read yet.
func TestPollLeavesAQueuedJob(t *testing.T) {
	f := newFake()
	f.jobs[2][1].Status = core.RunQueued
	s := primed(t, f)
	clock(s)
	defer s.WatchLog(repo, 2, 22)()
	if _, err := s.Poll(repo, 2)(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls := f.take(); logReads(calls) != 0 {
		t.Errorf("calls = %q, want no log read", calls)
	}
}
