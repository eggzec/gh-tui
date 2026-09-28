package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Changed does nothing: the fake's details don't age.
func (*fakeService) Changed(core.RepoRef, int, time.Time) {}

// firstComments returns the numbers whose first comments were read.
func (f *fakeService) firstComments() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []int
	for _, q := range f.comments {
		if q == commentsQuery(q.Repo, q.Number) {
			out = append(out, q.Number)
		}
	}
	return out
}

func sorted(s []int) []int {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func TestPrefetchFirstRows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		// #135 is cached already.
		svc.cached[135] = true
		svc.commented[commentsQuery(repo, 135)] = true
		started(t, svc, 80, 30, WithPrefetch(3, 150*time.Millisecond))

		// The first three rows but the cached one. The row under the
		// cursor is among them, so it isn't read again.
		want := []int{128, 142}
		if got := sorted(svc.got()); !slices.Equal(got, want) {
			t.Errorf("read details %v, want %v", got, want)
		}
		if got := sorted(svc.firstComments()); !slices.Equal(got, want) {
			t.Errorf("read first comments of %v, want %v", got, want)
		}
	})
}

func TestPrefetchHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		h := started(t, svc, 80, 30, WithPrefetch(0, 150*time.Millisecond))
		if got := svc.got(); !slices.Equal(got, []int{142}) {
			t.Errorf("read details %v, want the row under the cursor", got)
		}
		start := time.Now()
		press(t, h, "down")
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("read after %v, want the delay", waited)
		}
		if got := svc.got(); !slices.Equal(got, []int{142, 135}) {
			t.Errorf("read details %v, want the row moved to", got)
		}
		// Coming back to a cached row reads nothing.
		press(t, h, "up")
		if got := svc.got(); len(got) != 2 {
			t.Errorf("read details %v, want no more", got)
		}
	})
}

func TestPrefetchCancelledByRepo(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30, WithPrefetch(3, time.Millisecond))
	svc.mu.Lock()
	ctxs := slices.Clone(svc.getCtxs)
	svc.mu.Unlock()
	if len(ctxs) == 0 {
		t.Fatal("nothing was read ahead")
	}
	drain(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	for _, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Error("a read ahead of the old repository wasn't cancelled")
		}
	}
}

func TestPrefetchStopsAtRateLimit(t *testing.T) {
	svc := newFakeService()
	svc.getErr = &core.RateLimitError{Reset: clock}
	h := started(t, svc, 80, 30, WithPrefetch(5, time.Millisecond))
	n := len(svc.got())
	if n == 0 || n > 3 {
		t.Errorf("read %d details before the rate limit stopped it, want 1 to 3", n)
	}
	press(t, h, "down")
	press(t, h, "down")
	if got := len(svc.got()); got != n {
		t.Errorf("read %d more details under the rate limit", got-n)
	}
	// Another repository reads ahead again.
	svc.mu.Lock()
	svc.getErr = nil
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	for i := range svc.pulls {
		svc.pulls[i].Repo = other
	}
	svc.mu.Unlock()
	drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
	if got := len(svc.got()); got == n {
		t.Error("nothing was read ahead for another repository")
	}
}

func TestPrefetchedModalOpensAtOnce(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 40, WithPrefetch(5, time.Millisecond))
	press(t, h, "down")
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	drain(t, h, seq[0])
	view := modalScreen(t, h)
	for _, want := range []string{"Retry GraphQL requests", "Cold starts read every page", "Does this survive a crash"} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading…") {
		t.Errorf("modal waits for its detail:\n%s", view)
	}
}

func TestNoPrefetchByDefault(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30)
	press(t, h, "down")
	if got := svc.got(); len(got) != 0 {
		t.Errorf("read details %v without WithPrefetch", got)
	}
}

// TestNotificationReadAheadOpensAtOnce reads a pull request ahead from a
// notification, of another repository than the one shown, and opens it
// from the notification: it shows at once, from what was read ahead.
func TestNotificationReadAheadOpensAtOnce(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 40)
	other := core.RepoRef{Owner: "charmbracelet", Name: "glow"}
	n := core.Notification{Repo: other, Subject: core.Subject{Type: core.SubjectPullRequest, Number: 142}, UpdatedAt: clock}
	o := threads.New(t.Context(), threads.WithPulls(svc), threads.WithPrefetch(1, time.Millisecond))
	drain(t, h, o.ReadAhead(func(i int) (core.Notification, bool) { return n, i == 0 }, n, false))
	if got := svc.got(); !slices.Equal(got, []int{142}) {
		t.Fatalf("read %v ahead, want #142", got)
	}

	seq, ok := sequence(h.Update(o.Open(n)())())
	if !ok || len(seq) != 2 {
		t.Fatal("the notification should open the modal, then start its loads")
	}
	drain(t, h, seq[0])
	view := modalScreen(t, h)
	for _, want := range []string{"Cold starts read every page", "Does this survive a crash", "It writes to a temporary file"} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading…") {
		t.Errorf("modal waits for what was read ahead:\n%s", view)
	}
}

// rested runs cmd, which moved the cursor, until the delay reports that it
// rested, and returns the read that s then starts.
func rested(t *testing.T, s *Section, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	var find func(tea.Cmd) (ui.AheadMsg, bool)
	find = func(c tea.Cmd) (ui.AheadMsg, bool) {
		if c == nil {
			return ui.AheadMsg{}, false
		}
		switch msg := c().(type) {
		case ui.AheadMsg:
			return msg, true
		case tea.BatchMsg:
			for _, c := range msg {
				if m, ok := find(c); ok {
					return m, true
				}
			}
		}
		return ui.AheadMsg{}, false
	}
	msg, ok := find(cmd)
	if !ok {
		t.Fatal("moving the cursor started no delay")
	}
	read := s.Update(msg)
	if read == nil {
		t.Fatal("resting on the row read nothing")
	}
	return read
}

func TestModalPausesReadAhead(t *testing.T) {
	tests := []struct {
		name string
		// settle runs the loads of the modal, or closes it.
		settle func(t *testing.T, h *host, loads tea.Cmd)
	}{
		{"until its detail loads", func(t *testing.T, h *host, loads tea.Cmd) {
			t.Helper()
			drain(t, h, loads)
		}},
		{"until it closes", func(t *testing.T, h *host, _ tea.Cmd) {
			t.Helper()
			press(t, h, "esc")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc := newFakeService()
				h := started(t, svc, 80, 40, WithPrefetch(0, 150*time.Millisecond))
				seq, ok := sequence(h.Update(keyMsg("enter"))())
				if !ok || len(seq) != 2 {
					t.Fatal("enter should open the modal, then start its loads")
				}
				drain(t, h, seq[0])
				// The list reads #135 ahead behind the modal.
				read := rested(t, h.Section, h.Section.Update(keyMsg("down")))
				done := make(chan struct{})
				go func() {
					read()
					close(done)
				}()
				synctest.Wait()
				if slices.Contains(svc.got(), 135) {
					t.Error("read #135 ahead while the modal loads")
				}
				tt.settle(t, h, seq[1])
				<-done
				if !slices.Contains(svc.got(), 135) {
					t.Error("didn't read #135 ahead once the modal settled")
				}
			})
		})
	}
}

func TestModalResumesReadAheadWhateverTheOrder(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, h *host, svc *fakeService)
		// paused is set when the reads ahead should still wait after run.
		paused bool
	}{{
		name: "opened, not loaded yet",
		run: func(t *testing.T, h *host, _ *fakeService) {
			t.Helper()
			seq, _ := sequence(h.Update(keyMsg("enter"))())
			drain(t, h, seq[0])
		},
		paused: true,
	}, {
		name: "its detail fails",
		run: func(t *testing.T, h *host, svc *fakeService) {
			t.Helper()
			svc.getErr = errors.New("boom")
			seq, _ := sequence(h.Update(keyMsg("enter"))())
			drain(t, h, seq[0])
			drain(t, h, seq[1])
			press(t, h, "esc")
		},
	}, {
		name: "closed before it loads, another opened and closed, then both load",
		run: func(t *testing.T, h *host, _ *fakeService) {
			t.Helper()
			seq, _ := sequence(h.Update(keyMsg("enter"))())
			drain(t, h, seq[0])
			press(t, h, "esc")
			seq2, _ := sequence(h.Update(keyMsg("enter"))())
			drain(t, h, seq2[0])
			press(t, h, "esc")
			drain(t, h, seq[1])
			drain(t, h, seq2[1])
		},
	}, {
		name: "two opened, both load, neither closed",
		run: func(t *testing.T, h *host, _ *fakeService) {
			t.Helper()
			for _, c := range []tea.Cmd{h.openDetail(repo, 142, nil, false, false, nil), h.openDetail(repo, 128, nil, false, false, nil)} {
				seq, ok := sequence(c())
				if !ok {
					t.Fatal("opening a modal isn't a sequence")
				}
				for _, s := range seq {
					drain(t, h, s)
				}
			}
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc := newFakeService()
				h := started(t, svc, 80, 40, WithPrefetch(0, 150*time.Millisecond))
				tt.run(t, h, svc)
				svc.mu.Lock()
				svc.getErr = nil
				svc.mu.Unlock()
				read := rested(t, h.Section, h.Section.Update(keyMsg("down")))
				done := make(chan struct{})
				go func() {
					read()
					close(done)
				}()
				synctest.Wait()
				select {
				case <-done:
					if tt.paused {
						t.Error("read ahead while the modal loads")
					}
					if !slices.Contains(svc.got(), 135) {
						t.Errorf("read details %v, want #135 read ahead", svc.got())
					}
				default:
					if !tt.paused {
						t.Error("the reads ahead still wait")
					}
					// End the waiting read.
					h.cancelFeed()
					<-done
				}
			})
		})
	}
}
