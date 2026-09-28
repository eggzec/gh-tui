package tui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestOldEnterprise(t *testing.T) {
	old := make(chan string, 1)
	m, _ := newTestApp(t, WithOldEnterprise(old))
	old <- "3.12.4"
	run(m, m.listenOldEnterprise())
	want := "GitHub Enterprise 3.12 isn't supported (" + core.MinEnterprise + "+ is); some things may not work."
	if !hasToast(m, want) {
		t.Errorf("toasts %q, want %q", toasted(m), want)
	}
}
