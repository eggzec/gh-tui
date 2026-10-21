package tui

import (
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

const notifName = "the notifications"

func TestNotificationsOpenAsAModal(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		if got := m.modalName(m.modal); got != notifName {
			t.Fatalf("I opened %q, want %q", got, notifName)
		}
		for _, key := range []string{"esc", "backspace", "q"} {
			m := keysAfter(t, true, "global.notifications")
			tap(t, m, key)
			if m.modal != nil {
				t.Errorf("%s left %s open", key, m.modalName(m.modal))
			}
		}
		// I does nothing while the modal is open, as the help says.
		host := m.modal
		tap(t, m, "I")
		if m.modal != host {
			t.Errorf("I left %s, want the notifications still open", m.modalName(m.modal))
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
		if open.modalName(open.modal) != notifName {
			t.Fatalf("I opened %q, want %q", open.modalName(open.modal), notifName)
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
	for _, key := range []string{"esc", "backspace"} {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, "global.notifications")
			host := m.modal
			tap(t, m, "enter")
			if m.modal == host || m.modal == nil {
				t.Fatalf("enter left %v, want a thread modal over the notifications", m.modal)
			}
			tap(t, m, key)
			if m.modal != host {
				t.Errorf("%s left %v, want the notifications", key, m.modalName(m.modal))
			}
		})
	}
}

func TestReadAllInTheNotificationsModalAsks(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		host := m.modal
		runCommand(t, m, "read all")
		if m.modal == nil || m.modal == host {
			t.Fatalf("read all left %v, want a confirm over the notifications", m.modal)
		}
		tap(t, m, "esc")
		if m.modal != host {
			t.Errorf("esc left %v, want the notifications", m.modal)
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
				under := m.modal
				if under == nil {
					t.Fatal("the modal isn't open")
				}
				views := layerNames(m.keyLayers())
				depth := len(m.back)
				for _, key := range []string{"esc", "backspace"} {
					tap(t, m, "I")
					if got := m.modalName(m.modal); got != notifName {
						t.Fatalf("I over %s opened %q, want %q", mod.title, got, notifName)
					}
					tap(t, m, key)
					if m.modal != under || layerNames(m.keyLayers()) != views || len(m.back) != depth {
						t.Errorf("%s left %v with keys of %q and %d back, want the modal under with %q and %d", key, m.modalName(m.modal), layerNames(m.keyLayers()), len(m.back), views, depth)
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
		host := m.modal
		for _, key := range []string{"f", "s", "g"} {
			tap(t, m, key)
			if m.modal == host {
				t.Fatalf("%s opened nothing over the notifications", key)
			}
			if key != "f" {
				for _, back := range []string{"esc"} {
					tap(t, m, back)
					if m.modal != host {
						t.Fatalf("%s after %s left %v, want the notifications", back, key, m.modal)
					}
					tap(t, m, key)
				}
				tap(t, m, "esc")
				if m.modal != host {
					t.Fatalf("esc after %s left %v, want the notifications", key, m.modal)
				}
				continue
			}
			tap(t, m, "backspace")
			if m.modal != host {
				t.Fatalf("backspace after %s left %v, want the notifications", key, m.modal)
			}
		}
		tap(t, m, "Z")
		if !m.maximized {
			t.Error("Z didn't maximize the notifications")
		}
		tap(t, m, "Z")
		if m.maximized {
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
			open := m.modal
			tap(t, m, "I")
			if m.modal != open {
				t.Errorf("I over the token prompt left %s", m.modalName(m.modal))
			}
		}
	})
}
