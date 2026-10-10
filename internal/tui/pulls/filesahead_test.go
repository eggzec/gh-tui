package pulls

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// readingFiles returns the option that reads the first page of the files of
// the pull request a modal shows, as the defaults do, but for what edit
// changes in the settings of the kind.
func readingFiles(edit func(*config.Layer)) Option {
	p := config.Default().Prefetch
	if edit != nil {
		edit(&p.Pulls.Files)
	}
	return WithPrefetch(p)
}

// quickFiles is readingFiles with a short rest, for the tests that show the
// Files tab, which can't run on a fake clock.
func quickFiles() Option {
	return readingFiles(func(l *config.Layer) { l.Rest = new(5 * time.Millisecond) })
}

// newHead moves the head of the first pull request, and has the modal read
// the detail again, as a sync does. It returns the messages for the app.
func newHead(tb testing.TB, h *host, svc *fakeService, head string) {
	tb.Helper()
	svc.pulls[0].HeadSHA = head
	drain(tb, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
}

// openHeld opens the modal of the first pull request on its conversation,
// with the read ahead of its files held by svc.filesGate, which the caller
// closes. The commands that follow the open run in a goroutine that ends
// when done is closed.
func openHeld(tb testing.TB, svc *fakeService) (h *host, m *detailModal, done chan struct{}) {
	tb.Helper()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	svc.filesGate = make(chan struct{})
	h = started(tb, svc, 100, 30, readingFiles(nil), WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	cmd := h.Update(keyMsg("enter"))
	done = make(chan struct{})
	go func() {
		defer close(done)
		drain(tb, h, cmd)
	}()
	// The rest ends, and the read starts and is held.
	time.Sleep(time.Second)
	synctest.Wait()
	if m = h.modal(); m == nil {
		tb.Fatal("enter opened no pull request")
	}
	return h, m, done
}

func TestFilesAheadReadsAfterRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesCache = true
		start := time.Now()
		_, m := filedWith(t, svc, 100, 30, readingFiles(nil))
		if got := time.Since(start); got != 300*time.Millisecond {
			t.Errorf("the files were read ahead after %v, want 300ms of rest", got)
		}
		if n := svc.fileReadCount(); n != 1 {
			t.Fatalf("%d pages read ahead while the conversation shows, want the first", n)
		}
		if q := svc.fileReads[0]; q.Head != "a1b2c3d" || q.Cursor != "" || q.Number != svc.pulls[0].Number {
			t.Errorf("read ahead %+v, want the first page at the head", q)
		}
		if m.files != nil {
			t.Error("reading ahead started the Files tab")
		}
	})
}

func TestFilesAheadThenTabUsesIt(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.filesCache = true
	h, m := filedWith(t, svc, 100, 30, quickFiles())
	if n := svc.fileReadCount(); n != 1 {
		t.Fatalf("%d pages read ahead, want the first", n)
	}
	// The tab shows what was read, with no request.
	press(t, h, "[")
	if m.files == nil || m.files.diff.Files() != len(svc.changed) {
		t.Fatalf("the tab shows %v, want the %d files read ahead", m.files, len(svc.changed))
	}
	if n := svc.fileReadCount(); n != 1 {
		t.Errorf("showing the Files tab read %d more pages, want none", n-1)
	}
}

// filesOpened replaces the default stats and returns how many files reads
// ahead they count as used.
func filesOpened(t *testing.T) func() int64 {
	t.Helper()
	stats := obs.NewStats()
	prev := obs.SetDefault(stats)
	t.Cleanup(func() { obs.SetDefault(prev) })
	return func() int64 {
		for _, p := range stats.Summary().Prefetch {
			if p.Kind == "pull_files" {
				return p.Opened
			}
		}
		return 0
	}
}

// TestFilesAheadThenTabCountsAsUsed checks that showing the Files tab counts
// the files read ahead of it as used, and not before. It stays serial, as it
// replaces the default stats.
func TestFilesAheadThenTabCountsAsUsed(t *testing.T) {
	opened := filesOpened(t)
	svc := newFakeService()
	svc.filesCache = true
	h, _ := filedWith(t, svc, 100, 30, quickFiles())
	if n := opened(); n != 0 {
		t.Fatalf("%d files reads counted as used before the tab showed, want none", n)
	}
	press(t, h, "[")
	if n := opened(); n != 1 {
		t.Errorf("%d files reads counted as used after showing the tab, want 1", n)
	}
}

// TestFilesAheadNewHeadThenTabCountsAsUsed checks that the files read ahead
// of a new head count as used when the tab, which was seen before, shows
// again and reads them. It stays serial, as it replaces the default stats.
func TestFilesAheadNewHeadThenTabCountsAsUsed(t *testing.T) {
	opened := filesOpened(t)
	svc := newFakeService()
	svc.filesCache = true
	h, _ := filedWith(t, svc, 100, 30, quickFiles())
	press(t, h, "[")
	press(t, h, "]")
	newHead(t, h, svc, "b2c3d4e")
	if n := opened(); n != 1 {
		t.Fatalf("%d files reads counted as used before the tab showed again, want the first", n)
	}
	press(t, h, "[")
	if n := opened(); n != 2 {
		t.Errorf("%d files reads counted as used after the tab showed again, want 2", n)
	}
}

func TestFilesAheadWaitsForRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesCache = true
		h, _ := filedWith(t, svc, 100, 30, readingFiles(nil))
		reads := svc.fileReadCount()
		// A new head starts the rest again, and moving to another tab and
		// back before it ends starts it once more.
		svc.pulls[0].HeadSHA = "b2c3d4e"
		cmd := h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)})
		start := time.Now()
		drain(t, h, cmd)
		if got := time.Since(start); got < 300*time.Millisecond {
			t.Errorf("the new head's files were read after %v, want the rest first", got)
		}
		if svc.fileReadCount() != reads+1 || svc.fileReads[reads].Head != "b2c3d4e" {
			t.Errorf("reads %+v, want the new head's first page", svc.fileReads)
		}
	})
}

func TestFilesAheadOff(t *testing.T) {
	quick := func(l *config.Layer) { l.Rest = new(5 * time.Millisecond) }
	tests := []struct {
		name string
		opt  Option
	}{
		{"kind off", readingFiles(func(l *config.Layer) { quick(l); l.Enabled = new(false) })},
		{"all off", func() Option {
			p := config.Default().Prefetch
			p.Enabled = false
			p.Pulls.Files.Rest = new(5 * time.Millisecond)
			p.Pulls.Files.Enabled = nil
			return WithPrefetch(p)
		}()},
		{"no settings", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newFakeService()
			svc.filesCache = true
			var opts []Option
			if tt.opt != nil {
				opts = append(opts, tt.opt)
			}
			h, _ := filedWith(t, svc, 100, 30, opts...)
			press(t, h, "]")
			press(t, h, "[")
			if n := svc.fileReadCount(); n != 0 {
				t.Errorf("%d pages read while another tab shows, want none", n)
			}
			// Only the tab's own read, once it shows.
			press(t, h, "[")
			if n := svc.fileReadCount(); n != 1 {
				t.Errorf("%d pages read once the Files tab showed, want its own", n)
			}
		})
	}
}

func TestFilesAheadNotWhileFilesShows(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.filesCache = true
	h, m := filedWith(t, svc, 100, 30, quickFiles())
	press(t, h, "[")
	if !m.onFiles() {
		t.Fatal("the modal isn't on the Files tab")
	}
	reads := svc.fileReadCount()
	// The tab reads the files of the new head itself.
	newHead(t, h, svc, "b2c3d4e")
	time.Sleep(20 * time.Millisecond)
	press(t, h, "j")
	if got := svc.fileReadCount(); got != reads+1 {
		t.Errorf("%d reads of the new head's files while Files shows, want the tab's own one", got-reads)
	}
}

func TestFilesAheadNotTwice(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.filesCache = true
	h, _ := filedWith(t, svc, 100, 30, quickFiles())
	for range 3 {
		press(t, h, "]")
		press(t, h, "[")
		press(t, h, "[")
		press(t, h, "]")
		press(t, h, "]")
		press(t, h, "[")
	}
	if n := svc.fileReadCount(); n != 1 {
		t.Errorf("%d pages read, want the one page once", n)
	}
}

func TestFilesAheadNotInFlightTwice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesCache = true
		h, _, done := openHeld(t, svc)
		// The first read is held. Other things that ask for a window,
		// such as GitHub answering again, don't send another.
		for range 3 {
			drain(t, h, h.Update(ui.OnlineMsg{}))
		}
		if n := svc.fileReadCount(); n != 1 {
			t.Errorf("%d reads of the page while one is in flight, want 1", n)
		}
		close(svc.filesGate)
		<-done
	})
}

func TestFilesAheadWaitsWhileLimited(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesCache = true
		svc.filesErr = &core.RateLimitError{Reset: clock}
		h, _ := filedWith(t, svc, 100, 30, readingFiles(nil))
		if n := svc.fileReadCount(); n != 1 {
			t.Fatalf("%d reads, want the one the limit refused", n)
		}
		// Neither a push nor another tab reads while the limit holds.
		newHead(t, h, svc, "b2c3d4e")
		newHead(t, h, svc, "c3d4e5f")
		if n := svc.fileReadCount(); n != 1 {
			t.Errorf("%d reads under the limit, want none after the first", n-1)
		}
		svc.mu.Lock()
		svc.filesErr = nil
		svc.mu.Unlock()
		// A limit that holds goes on holding; one that lifted resumes.
		drain(t, h, h.Update(ui.OnlineMsg{Limited: true}))
		if n := svc.fileReadCount(); n != 1 {
			t.Errorf("%d reads while GitHub still limited", n-1)
		}
		drain(t, h, h.Update(ui.OnlineMsg{}))
		if n := svc.fileReadCount(); n != 2 || svc.fileReads[1].Head != "c3d4e5f" {
			t.Errorf("reads %+v, want the head's page once the limit lifted", svc.fileReads)
		}
	})
}

func TestFilesAheadErrorsAreSilent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesErr = errors.New("boom")
		_, m := filedWith(t, svc, 100, 30, readingFiles(nil))
		if n := svc.fileReadCount(); n != 1 {
			t.Fatalf("%d reads, want the one that failed", n)
		}
		if m.files != nil || m.failed != nil {
			t.Errorf("the failed read ahead left files %v, failure %v", m.files, m.failed)
		}
	})
}

func TestFilesAheadDroppedOnClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesCache = true
		h, m := filedWith(t, svc, 100, 30, readingFiles(nil))
		reads := svc.fileReadCount()
		// A push starts a read, which the modal closing before the rest is
		// over drops.
		svc.pulls[0].HeadSHA = "b2c3d4e"
		cmd := h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)})
		press(t, h, "esc")
		if !m.closed {
			t.Fatal("esc left the modal open")
		}
		drain(t, h, cmd)
		if cmd := m.Update(ui.AheadMsg{}); cmd != nil {
			t.Error("the closed modal went on")
		}
		if got := svc.fileReadCount(); got != reads {
			t.Errorf("%d pages read after the modal closed", got-reads)
		}
	})
}

func TestFilesAheadInFlightCancelledOnClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		h, m, done := openHeld(t, svc)
		if len(svc.fileCtxs) != 1 {
			t.Fatalf("%d reads in flight, want 1", len(svc.fileCtxs))
		}
		drain(t, h, h.Update(keyMsg("esc")))
		<-done
		if svc.fileCtxs[0].Err() == nil || !m.closed {
			t.Error("the read ahead went on after the modal closed")
		}
	})
}

func TestFilesAheadHeadChangeDropsRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		h, _, done := openHeld(t, svc)
		if len(svc.fileCtxs) != 1 {
			t.Fatalf("%d reads in flight, want 1", len(svc.fileCtxs))
		}
		svc.pulls[0].HeadSHA = "b2c3d4e"
		gate := svc.filesGate
		cmd := h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)})
		moved := make(chan struct{})
		go func() {
			defer close(moved)
			drain(t, h, cmd)
		}()
		time.Sleep(time.Second)
		synctest.Wait()
		if svc.fileCtxs[0].Err() == nil {
			t.Error("the read of the old head went on after a push")
		}
		close(gate)
		<-moved
		<-done
		if last := svc.fileReads[len(svc.fileReads)-1].Head; svc.fileReads[0].Head != "a1b2c3d" || last != "b2c3d4e" {
			t.Errorf("read heads %v, want the old and then the new", svc.fileReads)
		}
	})
}

// TestFilesAheadReadsAfterHidden loses the message that ends the rest, and
// all that follows, as when another modal replaces this one then.
func TestFilesAheadReadsAfterHidden(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.filesCache = true
		svc.pulls[0].HeadSHA = "a1b2c3d"
		svc.changed = sampleFiles()
		h := started(t, svc, 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)), readingFiles(nil))
		hidden := false
		lose := func(msg tea.Msg) bool {
			if _, ok := msg.(ui.AheadMsg); ok {
				hidden = true
			}
			return hidden
		}
		drainLosing(t, h, h.Update(keyMsg("enter")), lose)
		if !hidden {
			t.Fatal("the read ahead never rested, so the modal was not hidden during it")
		}
		m := h.modal()
		if n := svc.fileReadCount(); n != 0 {
			t.Fatalf("%d pages read ahead while the modal was hidden, want none", n)
		}
		// Shown again, the modal waits out the rest anew and then reads.
		drain(t, h, m.Update(ui.ReopenedMsg{Modal: m}))
		if n := svc.fileReadCount(); n != 1 {
			t.Fatalf("%d pages read ahead after the modal showed again, want the first", n)
		}
	})
}
