package tui

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// backModal is a fake modal that closes with esc, and with the quit
// intent, and counts the times it is discarded.
type backModal struct {
	fakeModal
	discarded int
}

func (b *backModal) Update(msg tea.Msg) tea.Cmd {
	b.msgs = append(b.msgs, msg)
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEscape {
		return ui.CloseModal(b)
	}
	return nil
}

func (b *backModal) Act(action string) (tea.Cmd, bool) {
	if action == ui.ActQuit {
		return ui.CloseModal(b), true
	}
	return nil, false
}
func (b *backModal) Discard() { b.discarded++ }

// KeyLayers lists no keys, in a context that doesn't take them all, so that
// the app's keys reach the modal as intents.
func (b *backModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{{Source: b.title, Context: "issue_modal"}}
}

// disabledBackModal has a binding on the back key that is off, as the
// pager's search clear is while no search is open.
type disabledBackModal struct {
	backModal
	keys []string
}

func (d *disabledBackModal) KeyLayers() []keyhelp.Layer {
	off := key.NewBinding(key.WithKeys(d.keys...), key.WithHelp("off", "clear"))
	off.SetEnabled(false)
	return []keyhelp.Layer{{Source: d.title, Context: "issue_modal", Bindings: []key.Binding{off}}}
}

// TestHelpListsBackBesideDisabledBinding checks that a disabled binding on
// the back key doesn't hide the app's back key from the help, since the
// disabled one doesn't answer the key.
func TestHelpListsBackBesideDisabledBinding(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	first := &backModal{}
	first.title = "first"
	second := &disabledBackModal{keys: m.keys.Back.Keys()}
	second.title = "second"
	run(m, ui.OpenModalOver(first, nil))
	run(m, ui.OpenModalOver(second, first))
	if !m.modalBack() {
		t.Fatal("the open modal has no modal to return to")
	}
	if !backKey(t, m) {
		t.Error("the help lists the back key as off, or not at all")
	}
}

// backKey returns the back key as the help of the open modal lists it, and
// whether it is on.
func backKey(t *testing.T, m *Model) bool {
	t.Helper()
	for _, l := range m.keyLayers() {
		for _, b := range l.Bindings {
			if b.Help().Desc == "back" && slices.Contains(b.Keys(), "backspace") {
				return b.Enabled()
			}
		}
	}
	t.Fatal("the help lists no back key")
	return false
}

var backspace = tea.KeyPressMsg{Code: tea.KeyBackspace}

// chain opens the modals titled titles, each in place of the one before,
// as a reference picked in a modal opens its item.
func chain(m *Model, titles ...string) []*backModal {
	mods := make([]*backModal, 0, len(titles))
	for _, title := range titles {
		mod := &backModal{}
		mod.title = title
		var prev ui.Modal
		if n := len(mods); n > 0 {
			prev = mods[n-1]
		}
		run(m, ui.OpenModalOver(mod, prev))
		mods = append(mods, mod)
	}
	return mods
}

// TestBackReturnsToModal checks that the back key returns to the modal
// that another replaced, and through a chain of them to the first, and
// that it does nothing where the chain begins.
func TestBackReturnsToModal(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mods := chain(m, "A", "B", "C")
	if !backKey(t, m) {
		t.Error("the help shows the back key off over C, which has two modals to return to")
	}
	run(m, m.key(backspace))
	if m.modal != mods[1] || mods[2].discarded != 1 {
		t.Fatalf("after one back: open %v, C discarded %d times, want B and once", m.topModal(), mods[2].discarded)
	}
	run(m, m.key(backspace))
	if m.modal != mods[0] || mods[1].discarded != 1 {
		t.Fatalf("after two: open %v, want A", m.topModal())
	}
	if backKey(t, m) {
		t.Error("the help shows the back key on over A, which has none to return to")
	}
	if !slices.ContainsFunc(mods[0].msgs, func(msg tea.Msg) bool { _, ok := msg.(ui.ReopenedMsg); return ok }) {
		t.Error("A wasn't told that it is open again")
	}
	run(m, m.key(backspace))
	if m.modal != mods[0] || mods[0].discarded != 0 {
		t.Errorf("back at the first modal changed %v, discards %d", m.topModal(), mods[0].discarded)
	}
}

// TestBackKeepsScroll checks that a pull request returned to is the one
// that was left, read down to where it was.
func TestBackKeepsScroll(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2} })
		a := m.modal
		if a == nil {
			t.Fatal("no pull request opened")
		}
		// The conversation is the tab that reads down.
		driveKeys(t, m, m.key(press("]")))
		driveKeys(t, m, m.key(press("]")))
		top := a.View()
		driveKeys(t, m, m.key(press("G")))
		bottom := a.View()
		if bottom == top {
			t.Fatal("the pull request didn't scroll; the test needs a longer body")
		}
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 3, Back: a} })
		if m.modal == nil || m.modal == a {
			t.Fatal("the second pull request didn't open in place of the first")
		}
		b := m.modal
		driveKeys(t, m, m.key(backspace))
		if m.modal != a {
			t.Fatalf("the back key didn't return to the first pull request")
		}
		if got := a.View(); got != bottom {
			t.Errorf("the first pull request shows another place than it was left at:\n%s", got)
		}
		// The way back is used up, and the one left is gone for good.
		driveKeys(t, m, m.key(backspace))
		if m.modal != a {
			t.Error("the back key left the first modal with nothing to go back to")
		}
		if d, ok := b.(ui.Discarder); !ok {
			t.Error("the pull request modal can't be discarded")
		} else {
			d.Discard()
		}
	})
}

// TestBackCap checks that the history keeps the newest modals, and drops
// the oldest, discarded, beyond the bound.
func TestBackCap(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	titles := make([]string, maxBackModals+3)
	for i := range titles {
		titles[i] = string(rune('a' + i))
	}
	mods := chain(m, titles...)
	// The top and the maxBackModals before it are kept: the first two of
	// the thirteen are dropped, and the top isn't a place to return to.
	for i, mod := range mods {
		want := 0
		if i < 2 {
			want = 1
		}
		if mod.discarded != want {
			t.Errorf("modal %d discarded %d times, want %d", i, mod.discarded, want)
		}
	}
	n := 0
	for range maxBackModals {
		run(m, m.key(backspace))
		n++
	}
	if m.modal != mods[2] {
		t.Errorf("after %d backs the open modal is %v, want the third", n, m.topModal())
	}
	run(m, m.key(backspace))
	if m.modal != mods[2] {
		t.Error("the back key went past the oldest modal kept")
	}
}

// TestBackNeedsTheOpenModal checks that a modal opened in place of one that
// isn't open keeps nothing, and that opening a modal again keeps nothing.
func TestBackNeedsTheOpenModal(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	a, stale := &backModal{}, &backModal{}
	run(m, ui.OpenModal(a))
	b := &backModal{}
	run(m, ui.OpenModalOver(b, stale))
	if len(m.back) != 0 || backKey(t, m) {
		t.Errorf("a modal opened over one that wasn't open kept %d places", len(m.back))
	}
	run(m, ui.OpenModalOver(b, b))
	run(m, ui.Reopen(b))
	if len(m.back) != 0 {
		t.Errorf("opening the open modal again kept %d places", len(m.back))
	}
}

// TestCloseDropsBack checks that closing the modal, with esc or q, ends the
// chain: every modal it kept is discarded.
func TestCloseDropsBack(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"esc", "q"} {
		t.Run(k, func(t *testing.T) {
			m, _ := newTestApp(t)
			mods := chain(m, "A", "B", "C")
			run(m, m.key(press(k)))
			if m.modal != nil {
				t.Fatalf("%s left %v open", k, m.topModal())
			}
			if mods[0].discarded != 1 || mods[1].discarded != 1 {
				t.Errorf("discarded A %d, B %d times, want once each", mods[0].discarded, mods[1].discarded)
			}
			if len(m.back) != 0 {
				t.Errorf("%d places are kept", len(m.back))
			}
		})
	}
}

// TestBackSurvivesPreview checks that a modal opened from the one on view,
// and closed to return to it, keeps the way back.
func TestBackSurvivesPreview(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mods := chain(m, "A", "B")
	preview := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(preview))
	run(m, ui.Reopen(mods[1]))
	if !backKey(t, m) || mods[0].discarded != 0 {
		t.Fatalf("after the preview the back key is on: %v, A discarded %d times", backKey(t, m), mods[0].discarded)
	}
	run(m, m.key(backspace))
	if m.modal != mods[0] {
		t.Errorf("the back key after a preview opened %v, want A", m.topModal())
	}
}

// TestBackAfterChainEnds checks that the back key through the screens works
// after a chain of modals ended, and that a chain's places don't get in its
// way.
func TestBackAfterChainEnds(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	run(m, m.showScreen(notifScreen, 0))
	mods := chain(m, "A", "B")
	run(m, m.key(press("esc")))
	if m.modal != nil || mods[0].discarded != 1 {
		t.Fatalf("the chain didn't end: open %v", m.topModal())
	}
	if m.screen != notifScreen {
		t.Fatalf("on screen %d, want the notifications", m.screen)
	}
	run(m, m.key(backspace))
	if m.screen != repoScreen {
		t.Errorf("the back key shows screen %d, want the repository", m.screen)
	}
}

// TestBackTypesInPrompt checks that the back key types into the prompt of
// the comment on an issue, which was opened from another modal, instead of
// returning.
func TestBackTypesInPrompt(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		a := m.modal
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2, Back: a} })
		b := m.modal
		if b == nil || b == a {
			t.Fatal("the second issue didn't open in place of the first")
		}
		driveKeys(t, m, m.key(press("c")))
		for _, k := range []string{"a", "b", "x"} {
			driveKeys(t, m, m.key(press(k)))
		}
		driveKeys(t, m, m.key(backspace))
		if m.modal != b {
			t.Fatalf("the back key in the prompt left the modal")
		}
		if got := onScreen(m); !strings.Contains(got, "ab") || strings.Contains(got, "abx") {
			t.Errorf("the prompt doesn't show %q after the back key:\n%s", "ab", got)
		}
	})
}

// pauses counts the reads it holds, and those it lets go on.
type pauses struct{ held, resumed int }

func (p *pauses) Pause() func() {
	p.held++
	return func() { p.resumed++ }
}

// TestChainDropDiscardsRealModals checks that the end of a chain discards
// the modals it kept, which are real ones: the reads held back for the first
// go on, as they do when it closes. The quit key reaches the modal as an
// intent, and esc as its own key.
func TestChainDropDiscardsRealModals(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"esc", "q"} {
		for _, kind := range []string{"pull", "issue"} {
			t.Run(kind+" "+k, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					m := newKeysApp(t, true)
					p := &pauses{}
					open := func(n int, back ui.Modal, pause ui.Pauser) {
						var msg tea.Msg = ui.OpenIssueMsg{Repo: testRepo, Number: n, Back: back, Pause: pause}
						if kind == "pull" {
							msg = ui.OpenPullMsg{Repo: testRepo, Number: n, Back: back, Pause: pause}
						}
						driveKeys(t, m, func() tea.Msg { return msg })
					}
					open(1, nil, p)
					a := m.modal
					open(2, a, nil)
					if m.modal == nil || m.modal == a {
						t.Fatal("the second modal didn't open in place of the first")
					}
					held, resumed := p.held, p.resumed
					driveKeys(t, m, m.key(press(k)))
					if m.modal != nil {
						t.Fatalf("%s left a modal open", k)
					}
					if p.resumed <= resumed || p.held != held {
						t.Errorf("the first modal wasn't discarded: held %d, resumed %d before, %d after", p.held, resumed, p.resumed)
					}
				})
			})
		}
	}
}

// TestBackStepsOutOfTheChecks checks that the back key steps out of a log
// on the Checks tab of a pull request to the list of checks, and only then
// returns to the modal that the pull request replaced, if it replaced one.
func TestBackStepsOutOfTheChecks(t *testing.T) {
	t.Parallel()
	for _, chained := range []bool{true, false} {
		name := "alone"
		if chained {
			name = "in a chain"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := newKeysApp(t, true)
				tap := func(action string) {
					t.Helper()
					msg, ok := keyPress(m.cfg.Keys.Of(action)[0])
					if !ok {
						t.Fatalf("can't press %s", action)
					}
					driveKeys(t, m, m.key(msg))
				}
				open := func(back ui.Modal) {
					driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1, Back: back} })
				}
				open(nil)
				first := m.modal
				if chained {
					open(first)
				}
				pull := m.modal
				tap("pulls.checks")
				tap(config.ActionSelect)
				if got := layerNames(m.keyLayers()); got != "global, pull_modal, pull_check_log" {
					t.Fatalf("after opening a log the keys reach %q", got)
				}
				tap(config.ActionBack)
				if got := layerNames(m.keyLayers()); m.modal != pull || got != "global, pull_modal, pull_check_list" {
					t.Fatalf("back in the log left the keys at %q, want the list of checks", got)
				}
				tap(config.ActionBack)
				switch {
				case chained && m.modal != first:
					t.Errorf("back on the list left %v open, want the first modal", m.topModal())
				case !chained && (m.modal != pull || layerNames(m.keyLayers()) != "global, pull_modal, pull_check_list"):
					t.Errorf("back on the list with nothing to return to changed %q", layerNames(m.keyLayers()))
				}
			})
		})
	}
}
