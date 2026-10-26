package tui

import (
	"sync/atomic"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

const notifName = "the notifications"

func TestNotificationsOpenAsAModal(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		if got := m.modalName(m.modals.Top()); got != notifName {
			t.Fatalf("I opened %q, want %q", got, notifName)
		}
		for _, key := range []string{"esc", "backspace", "q"} {
			m := keysAfter(t, true, "global.notifications")
			tap(t, m, key)
			if m.modals.Top() != nil {
				t.Errorf("%s left %s open", key, m.modalName(m.modals.Top()))
			}
		}
		// I does nothing while the modal is open, as the help says.
		host := m.modals.Top()
		tap(t, m, "I")
		if m.modals.Top() != host {
			t.Errorf("I left %s, want the notifications still open", m.modalName(m.modals.Top()))
		}
	})
}

// TestSyncReachesTheNotificationsOnce checks that the modal, which shows the
// shared notifications section, doesn't make a broadcast reach it twice.
func TestSyncReachesTheNotificationsOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var reads atomic.Int32
		inbox := keyInbox{reads: &reads}
		sync := ui.SyncMsg{Key: notifications.SyncKey}
		edit := func(*config.Config) {}

		closed := newKeysAppInbox(t, true, edit, inbox)
		reads.Store(0)
		driveKeys(t, closed, closed.broadcast(sync))
		want := reads.Load()
		if want == 0 {
			t.Fatal("a sync didn't make the notifications read their list")
		}

		open := newKeysAppInbox(t, true, edit, inbox)
		c := keyContext{name: "root rules", repo: true}
		for _, name := range c.press(t, open.cfg.Keys, "global.notifications") {
			tap(t, open, name)
		}
		if open.modalName(open.modals.Top()) != notifName {
			t.Fatalf("I opened %q, want %q", open.modalName(open.modals.Top()), notifName)
		}
		reads.Store(0)
		driveKeys(t, open, open.broadcast(sync))
		if got := reads.Load(); got != want {
			t.Errorf("one sync made %d reads with the modal open, want %d", got, want)
		}
	})
}

func TestThreadOpensStackOverTheNotifications(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"esc", "backspace", "q"} {
		t.Run(key, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, "global.notifications")
				host := m.modals.Top()
				tap(t, m, "enter")
				if m.modals.Top() == host || m.modals.Top() == nil {
					t.Fatalf("enter left %T, want a thread modal over the notifications", m.modals.Top())
				}
				tap(t, m, key)
				if m.modals.Top() != host {
					t.Errorf("%s left %T, want the notifications", key, m.modals.Top())
				}
			})
		})
	}
}

// TestNotificationsDroppedWithTheirChain checks that a thread opened from the
// notifications, which another modal then replaces, takes the notifications
// with it when that one closes, so they are discarded and stop what they
// read ahead.
func TestNotificationsDroppedWithTheirChain(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		tap(t, m, "enter")
		thread := m.modals.Top()
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2} })
		if m.modals.Top() == nil || m.modals.Top() == thread {
			t.Fatalf("the second issue left %T, want it in place of the thread", m.modals.Top())
		}
		tap(t, m, "esc")
		if m.notifHost() == nil {
			t.Fatalf("esc left %T, want the notifications", m.modals.Top())
		}
		tap(t, m, "esc")
		if m.modals.Top() != nil {
			t.Fatalf("esc left %T, want no modal", m.modals.Top())
		}
		if m.modals.CanBack() {
			t.Error("the closed chain kept a place for the back key")
		}
	})
}

func TestReadAllInTheNotificationsModalAsks(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		host := m.modals.Top()
		runCommand(t, m, "read all")
		if m.modals.Top() == nil || m.modals.Top() == host {
			t.Fatalf("read all left %v, want a confirm over the notifications", m.modals.Top())
		}
		tap(t, m, "esc")
		if m.modals.Top() != host {
			t.Errorf("esc left %v, want the notifications", m.modals.Top())
		}
	})
}

func TestNotificationsOverAModalReturnToIt(t *testing.T) {
	t.Parallel()
	for _, mod := range modalsOver {
		if mod.name == "token prompt" {
			continue
		}
		t.Run(mod.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, mod.steps...)
				under := m.modals.Top()
				if under == nil {
					t.Fatal("the modal isn't open")
				}
				views := layerNames(m.keyLayers())
				depth := len(m.back)
				for _, key := range []string{"esc", "backspace"} {
					tap(t, m, "I")
					if got := m.modalName(m.modals.Top()); got != notifName {
						t.Fatalf("I over %s opened %q, want %q", mod.title, got, notifName)
					}
					tap(t, m, key)
					if m.modals.Top() != under || layerNames(m.keyLayers()) != views || len(m.back) != depth {
						t.Errorf("%s left %v with keys of %q and %d back, want the modal under with %q and %d", key, m.modalName(m.modals.Top()), layerNames(m.keyLayers()), len(m.back), views, depth)
					}
				}
			})
		})
	}
}

func TestNotificationsModalKeepsItsOverlays(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		host := m.modals.Top()
		for _, key := range []string{"f", "s", "g"} {
			tap(t, m, key)
			if m.modals.Top() == host {
				t.Fatalf("%s opened nothing over the notifications", key)
			}
			if key != "f" {
				for _, back := range []string{"esc"} {
					tap(t, m, back)
					if m.modals.Top() != host {
						t.Fatalf("%s after %s left %v, want the notifications", back, key, m.modals.Top())
					}
					tap(t, m, key)
				}
				tap(t, m, "esc")
				if m.modals.Top() != host {
					t.Fatalf("esc after %s left %v, want the notifications", key, m.modals.Top())
				}
				continue
			}
			tap(t, m, "backspace")
			if m.modals.Top() != host {
				t.Fatalf("backspace after %s left %v, want the notifications", key, m.modals.Top())
			}
		}
		tap(t, m, "Z")
		if !m.modals.Maximized() {
			t.Error("Z didn't maximize the notifications")
		}
		tap(t, m, "Z")
		if m.modals.Maximized() {
			t.Error("Z again didn't restore the notifications")
		}
	})
}

func TestTokenPromptRefusesTheNotifications(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		for _, mod := range modalsOver {
			if mod.name != "token prompt" {
				continue
			}
			m := keysAfter(t, true, mod.steps...)
			open := m.modals.Top()
			tap(t, m, "I")
			if m.notifHost() == nil {
				t.Fatalf("I over the token prompt left %s, want the notifications over it", m.modalName(m.modals.Top()))
			}
			tap(t, m, "esc")
			if m.modals.Top() != open {
				t.Errorf("esc left %s, want the token prompt", m.modalName(m.modals.Top()))
			}
		}
	})
}
