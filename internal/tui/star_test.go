package tui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeStar is a Starrer that records what it sends. known says whether the
// repository is in memory, and starred whether the viewer starred it.
type fakeStar struct {
	mu      sync.Mutex
	known   bool
	starred bool
	err     error
	sent    []string
}

func (f *fakeStar) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return core.Repo{Ref: ref, Starred: f.starred}, f.known
}

func (f *fakeStar) op(what string, ref core.RepoRef) *optimistic.Op {
	return optimistic.New(func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sent = append(f.sent, what+" "+ref.String())
		return f.err
	})
}

func (f *fakeStar) Star(ref core.RepoRef) *optimistic.Op   { return f.op("star", ref) }
func (f *fakeStar) Unstar(ref core.RepoRef) *optimistic.Op { return f.op("unstar", ref) }

func (f *fakeStar) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}

// starApp returns the keys app on the repository screen, with the keys of
// star as set, and the starrer.
func starApp(t *testing.T, f *fakeStar, keys ...string) *Model {
	t.Helper()
	return newKeysAppOpts(t, true, func(c *config.Config) { c.Keys.Set(config.ActionStar, keys) }, WithStarrer(f))
}

// answerQuestion answers the question that the app asks with key.
func answerQuestion(t *testing.T, m *Model, key string) {
	t.Helper()
	if got := layerNames(m.keyLayers()); got != "always, confirm" {
		t.Fatalf("the app has %q, want the question", got)
	}
	tap(t, m, key)
}

// TestStarCommandAsksThenSends checks that the command with no key asks
// whether to star, or to unstar what is starred, and sends only on a yes.
func TestStarCommandAsksThenSends(t *testing.T) {
	for _, tt := range []struct {
		name     string
		starred  bool
		answer   string
		question string
		sent     []string
		toast    string
	}{
		{"star", false, "y", "Star eggzec/gh-tui?", []string{"star eggzec/gh-tui"}, "Starred eggzec/gh-tui."},
		{"unstar", true, "y", "Unstar eggzec/gh-tui?", []string{"unstar eggzec/gh-tui"}, "Unstarred eggzec/gh-tui."},
		{"no", false, "n", "Star eggzec/gh-tui?", nil, ""},
		{"esc", true, "esc", "Unstar eggzec/gh-tui?", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := &fakeStar{known: true, starred: tt.starred}
				m := starApp(t, f)
				runCommand(t, m, "star")
				if got := layerNames(m.keyLayers()); got != "always, confirm" {
					t.Fatalf(":star reached %q, want the question: toasts %s", got, toasted(m))
				}
				if view := ansi.Strip(m.View().Content); !strings.Contains(view, tt.question) {
					t.Errorf(":star asks something else than %q:\n%s", tt.question, view)
				}
				if got := f.log(); len(got) != 0 {
					t.Fatalf("sent %v before the answer", got)
				}
				answerQuestion(t, m, tt.answer)
				if got := f.log(); !slices.Equal(got, tt.sent) {
					t.Errorf("sent %v, want %v", got, tt.sent)
				}
				if tt.toast != "" && !hasToast(m, tt.toast) {
					t.Errorf("toasts %s, want %q", toasted(m), tt.toast)
				}
			})
		})
	}
}

// TestStarKeyAsksLikeTheCommand checks that a key that the config gives
// the action asks the same question as the command.
func TestStarKeyAsksLikeTheCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeStar{known: true}
		m := starApp(t, f, "ctrl+s")
		tap(t, m, "ctrl+s")
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Star eggzec/gh-tui?") {
			t.Fatalf("the key asks something else:\n%s", view)
		}
		answerQuestion(t, m, "y")
		if got := f.log(); !slices.Equal(got, []string{"star eggzec/gh-tui"}) {
			t.Errorf("sent %v", got)
		}
	})
}

// TestStarCommandSaysWhenItFails checks that an error is shown, and that a
// repository that isn't read yet is not asked about, as the question would
// not know whether to star or to unstar.
func TestStarCommandSaysWhenItFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeStar{known: true, err: core.ErrForbidden}
		m := starApp(t, f)
		runCommand(t, m, "star")
		answerQuestion(t, m, "y")
		if hasToast(m, "Starred") || toasted(m) == "" {
			t.Errorf("toasts %q, want the error", toasted(m))
		}
	})
	synctest.Test(t, func(t *testing.T) {
		f := &fakeStar{}
		m := starApp(t, f)
		runCommand(t, m, "star")
		if got := layerNames(m.keyLayers()); got == "always, confirm" {
			t.Error(":star asks about a repository that isn't read")
		}
		if !hasToast(m, "Still reading eggzec/gh-tui") {
			t.Errorf("toasts %q", toasted(m))
		}
	})
}

// TestStarAsksAgainWhenItChanged checks that a star that arrives behind the
// question makes a yes send nothing.
func TestStarAsksAgainWhenItChanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeStar{known: true}
		m := starApp(t, f)
		runCommand(t, m, "star")
		f.mu.Lock()
		f.starred = true
		f.mu.Unlock()
		answerQuestion(t, m, "y")
		if got := f.log(); len(got) != 0 {
			t.Errorf("sent %v, want nothing", got)
		}
		if !hasToast(m, ui.Meanwhile("eggzec/gh-tui")) {
			t.Errorf("toasts %q", toasted(m))
		}
	})
}

// TestStarCommandWorksWhereTheRepositoryIs checks that the command is
// refused out of the repository screen and over a modal, naming where it
// works.
func TestStarCommandWorksWhereTheRepositoryIs(t *testing.T) {
	for _, tt := range []struct {
		name  string
		repo  bool
		steps []string
		want  string
	}{
		{"on the dashboard", false, nil, "star works in Repository screen."},
		{"over history", true, []string{"repo.history"}, "Close History first to use star."},
		{"over the pull request", true, []string{"global.pane_2", "global.select"}, "Close the pull request first to use star."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := &fakeStar{known: true}
				m := newKeysAppOpts(t, tt.repo, func(*config.Config) {}, WithStarrer(f))
				c := keyContext{name: tt.name, repo: tt.repo}
				for _, s := range tt.steps {
					for _, k := range c.press(t, config.Default().Keys, s) {
						msg, _ := keyPress(k)
						driveKeys(t, m, m.key(msg))
					}
				}
				open := m.modal
				runCommand(t, m, "star")
				if !hasToast(m, tt.want) {
					t.Errorf(":star toasts %s, want %q", toasted(m), tt.want)
				}
				if m.modal != open || len(f.log()) != 0 {
					t.Error("the refusal changed the modal or sent a change")
				}
			})
		})
	}
}
