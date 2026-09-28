package issues

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestKeptPageIsShownThenRevalidated(t *testing.T) {
	svc := newFakeService(sampleIssues(5))
	svc.stale = true
	h := newSection(t, svc, 80, 12)
	run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
	var views []string
	drive(t, h, h.Init(), func(tea.Msg) bool {
		views = append(views, ansi.Strip(h.View()))
		return false
	})
	shown := false
	for _, v := range views {
		shown = shown || strings.Contains(v, "Kept")
	}
	if !shown {
		t.Error("the kept page was never shown")
	}
	if v := ansi.Strip(h.View()); strings.Contains(v, "Kept") || !strings.Contains(v, "#999") {
		t.Errorf("view after revalidating:\n%s\nwant GitHub's page", v)
	}
	if got := len(svc.listCalls()); got != 2 {
		t.Errorf("lists = %d, want 2: the kept page and its revalidation", got)
	}
}

// TestOfflineShowsKeptRows checks that rows served offline show without
// a toast: the status bar says the app is offline.
func TestOfflineShowsKeptRows(t *testing.T) {
	svc := newFakeService(sampleIssues(5))
	svc.offline = true
	h := newSection(t, svc, 80, 12)
	run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
	msgs := append(run(t, h, h.Init()), press(t, h, "r")...)
	if n, ok := has[ui.NotifyMsg](msgs); ok {
		t.Errorf("toast %q, want none", n.Text)
	}
	if h.list.Len() == 0 || h.list.Err() != nil {
		t.Errorf("rows = %d, error %v; want the rows served offline", h.list.Len(), h.list.Err())
	}
}
