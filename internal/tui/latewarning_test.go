package tui

import "testing"

func TestLateWarning(t *testing.T) {
	t.Parallel()
	late := make(chan string, 1)
	m, _ := newTestApp(t, WithLateWarning(late))
	const want = "The token is octocat's: profile work applies from the next start."
	late <- want
	run(m, m.listenLateWarning())
	if !hasToast(m, want) {
		t.Errorf("toasts %q, want %q", toasted(m), want)
	}
}
