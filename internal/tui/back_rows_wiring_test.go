package tui

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestQuitCommandStaysOpenWhenAGuardKeeps checks that :q asks the guards and
// leaves the modal open, and does not quit, while one of them keeps it.
func TestQuitCommandStaysOpenWhenAGuardKeeps(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mod := &savingModal{}
	mod.title = "Comment"
	prio, ok := m.priorityOf(mod)
	if !ok {
		t.Fatal("the saving modal has no rank")
	}
	asked := 0
	m.apply(m.modals.Add(mod, prio, modals.WithGuard(func(modals.Reason) modals.Closing {
		asked++
		return modals.Refuse("Finish the comment first.")
	})))
	if m.modals.Top() != ui.Modal(mod) {
		t.Fatalf("the modal didn't open: %v", m.modals.Top())
	}
	cmd := m.runLine("q", nil)
	if asked == 0 {
		t.Error("the guard was not asked")
	}
	if quits(cmd) {
		t.Error(":q quit over a guard that keeps its modal")
	}
	if m.modals.Top() != ui.Modal(mod) {
		t.Errorf("the modal was closed: top is %v", m.modals.Top())
	}
}

// countDrafts counts the saves and deletes of the drafts of merge messages.
type countDrafts struct{ saves, deletes atomic.Int32 }

func (*countDrafts) Load(string) (string, error) { return "", nil }
func (d *countDrafts) Save(string, string) error { d.saves.Add(1); return nil }
func (d *countDrafts) Delete(string) error       { d.deletes.Add(1); return nil }

// TestCtrlCSavesARealSaver checks, with the real merge dialog, that ctrl+c
// saves the dialog's message while it is open, and quits.
func TestCtrlCSavesARealSaver(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		d := &countDrafts{}
		m := newKeysAppPulls(t, true, func(*config.Config) {}, keyInbox{}, []pulls.Option{pulls.WithDrafts(d, "test")})
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1} })
		driveKeys(t, m, m.key(press("M")))
		dialog := m.modals.Top()
		if _, ok := dialog.(*pulls.MergeDialog); !ok {
			t.Fatalf("M opened %T, want the merge dialog", dialog)
		}
		cmd := m.key(ctrlC)
		if n := d.saves.Load() + d.deletes.Load(); n != 1 {
			t.Errorf("the open dialog's draft was kept %d times, want 1", n)
		}
		if !quits(cmd) {
			t.Error("ctrl+c didn't quit")
		}
	})
}

// logActions is keyActions with a run that is still going, and counts the
// follows of its log that are open.
type logActions struct {
	keyActions
	open, runs atomic.Int32
}

func running() core.Run {
	r := keyRun
	r.Status, r.Conclusion = core.RunInProgress, ""
	return r
}

func (a *logActions) Runs(context.Context, actionssvc.RunsQuery) (core.Page[core.Run], error) {
	a.runs.Add(1)
	return core.Page[core.Run]{Items: []core.Run{running()}}, nil
}
func (*logActions) CachedRun(core.RepoRef, int64) (core.Run, bool) { return running(), true }
func (*logActions) Run(context.Context, core.RepoRef, int64) (core.Run, error) {
	return running(), nil
}
func runningJob() core.Job {
	j := keyJob
	j.Status, j.Conclusion = core.RunInProgress, ""
	return j
}

func (*logActions) CachedAllJobs(actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	return core.Page[core.Job]{Items: []core.Job{runningJob()}}, true
}
func (*logActions) AllJobs(context.Context, actionssvc.JobsQuery) (core.Page[core.Job], error) {
	return core.Page[core.Job]{Items: []core.Job{runningJob()}}, nil
}
func (a *logActions) WatchLog(core.RepoRef, int64, int64) func() {
	a.open.Add(1)
	return func() { a.open.Add(-1) }
}

// TestJobLogQuitReturnsToTheActions checks, with the real Actions modal, that
// q in a job's log ends its watch and shows the Actions modal again.
func TestJobLogQuitReturnsToTheActions(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		svc := &logActions{}
		m := newKeysAppOpts(t, true, func(*config.Config) {}, WithActions(actions.Opener(svc, config.Default().Keys)))
		driveTicking(m, func() tea.Msg {
			return ui.OpenActionsMsg{Repo: testRepo, At: ui.ActionsAt{Run: keyRun.ID, Job: keyJob.ID, Jobs: true}}
		})
		host := m.modals.Top()
		driveTicking(m, m.key(press("enter")))
		log := m.modals.Top()
		if log == nil || log == host {
			t.Fatalf("enter didn't open the log over the actions: %v", log)
		}
		if svc.open.Load() == 0 {
			t.Fatal("the open log isn't watched")
		}
		driveTicking(m, m.key(press("q")))
		if m.modals.Top() != host {
			t.Errorf("q left %v, want the Actions modal", m.modals.Top())
		}
		if n := svc.open.Load(); n != 0 {
			t.Errorf("%d watches still open after q", n)
		}
	})
}

// TestSearchPullIssueReturn checks, with the real modals, that esc on an issue
// opened from a pull request that the search opened returns to the search,
// and backspace to the pull request.
func TestSearchPullIssueReturn(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		key  string
		want func(search, pull ui.Modal) ui.Modal
	}{
		{"esc", func(search, _ ui.Modal) ui.Modal { return search }},
		{"backspace", func(_, pull ui.Modal) ui.Modal { return pull }},
	} {
		t.Run(tt.key, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := newKeysApp(t, true)
				driveKeys(t, m, m.key(press("S")))
				search := m.modals.Top()
				if search == nil {
					t.Fatal("S opened nothing")
				}
				driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1, Over: search} })
				pull := m.modals.Top()
				if pull == nil || pull == search {
					t.Fatalf("the pull request didn't open over the search: %v", pull)
				}
				driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2, Over: pull} })
				issue := m.modals.Top()
				if issue == nil || issue == pull || issue == search {
					t.Fatalf("the issue didn't open in place of the pull request: %v", issue)
				}
				driveKeys(t, m, m.key(press(tt.key)))
				if want := tt.want(search, pull); m.modals.Top() != want {
					t.Errorf("%s left %v, want %v", tt.key, m.modals.Top(), want)
				}
			})
		})
	}
}

// recordDrafts records the drafts saved, by key.
type recordDrafts struct {
	mu    sync.Mutex
	saved map[string][]string
}

func (*recordDrafts) Load(string) (string, error) { return "", nil }
func (d *recordDrafts) Save(key, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.saved == nil {
		d.saved = map[string][]string{}
	}
	d.saved[key] = append(d.saved[key], text)
	return nil
}
func (*recordDrafts) Delete(string) error { return nil }

// last is the text last saved under any key, and whether there was one.
func (d *recordDrafts) last() (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, v := range d.saved {
		return v[len(v)-1], true
	}
	return "", false
}

// openPullComment opens the real pull request modal with a comment being
// written in it, and returns the app.
func openPullComment(t *testing.T, d *recordDrafts, text string) *Model {
	t.Helper()
	m := newKeysAppPulls(t, true, func(*config.Config) {}, keyInbox{}, []pulls.Option{pulls.WithDrafts(d, "test")})
	driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1} })
	// The description quoted is the comment's first text.
	driveKeys(t, m, m.key(press("1")))
	driveKeys(t, m, m.key(press("c")))
	for _, r := range text {
		driveKeys(t, m, m.key(press(string(r))))
	}
	return m
}

// TestPullCommentDraftSavedWhenTheAppClosesTheModal checks, with the real pull
// request modal, that the text of a comment being written is saved when
// ctrl+c quits, and that nothing is saved with no composer open. A modal that
// another one replaces stays held, so only quitting saves it.
func TestPullCommentDraftSavedWhenTheAppClosesTheModal(t *testing.T) {
	t.Parallel()
	t.Run("ctrl+c", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := &recordDrafts{}
			m := openPullComment(t, d, "hi")
			cmd := m.key(ctrlC)
			if got, ok := d.last(); !ok || !strings.HasSuffix(got, "hi") {
				t.Errorf("saved %q (%v), want the comment typed", got, ok)
			}
			if !quits(cmd) {
				t.Error("ctrl+c didn't quit")
			}
		})
	})
	t.Run("no composer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			d := &recordDrafts{}
			m := newKeysAppPulls(t, true, func(*config.Config) {}, keyInbox{}, []pulls.Option{pulls.WithDrafts(d, "test")})
			driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1} })
			m.key(ctrlC)
			if got, ok := d.last(); ok {
				t.Errorf("saved %q with no composer open", got)
			}
		})
	})
}

// TestFilterEscTellsTheModalUnderItReopened checks that closing a modal
// that opened over another tells the one under it that it is open again, so
// it can resume.
func TestFilterEscTellsTheModalUnderItReopened(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		svc := &logActions{}
		m := newKeysAppOpts(t, true, func(*config.Config) {}, WithActions(actions.Opener(svc, config.Default().Keys)))
		driveTicking(m, m.key(press("A")))
		host := m.modals.Top()
		if host == nil {
			t.Fatal("A opened nothing")
		}
		driveTicking(m, m.key(press("f")))
		filter := m.modals.Top()
		if filter == nil || filter == host {
			t.Fatalf("f opened %v, want the filter over the actions", filter)
		}
		// The modal under the filter is told when the filter closes, and
		// follows the run it shows again.
		before := svc.runs.Load()
		driveTicking(m, m.key(press("esc")))
		if m.modals.Top() != host {
			t.Fatalf("esc left %v, want the Actions modal", m.modals.Top())
		}
		if svc.runs.Load() <= before {
			t.Error("the Actions modal didn't read its runs again after the filter closed")
		}
	})
}
