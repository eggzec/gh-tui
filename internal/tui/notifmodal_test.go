package tui

import (
	"testing"
	"testing/synctest"
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
