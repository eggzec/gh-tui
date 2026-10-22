package pulls

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func devRef(kind core.NumberKind, n int, title string) core.Reference {
	return core.Reference{Target: core.Target{Repo: repo, Number: n, Kind: kind}, Title: title, State: core.StateOpen}
}

func devLinks() core.PullLinks {
	return core.PullLinks{
		Closes: []core.LinkedIssue{
			{Issue: devRef(core.KindIssue, 12, "Crash on empty input")},
			{Issue: devRef(core.KindIssue, 13, "Slow start")},
		},
		ClosesTotal: 2,
		Linked:      []core.Reference{devRef(core.KindPull, 30, "Follow-up cleanup")},
	}
}

// openedDev opens the first pull request of a section that reads links from
// f, and gives its Development pane the focus. The cursor is on the Closes
// header.
func openedDev(t *testing.T, f *fakeDev) *host {
	t.Helper()
	h := started(t, newFakeService(), 120, 30, WithDevelopment(f))
	press(t, h, "enter")
	press(t, h, "5")
	if m := h.modal(); m == nil || m.focus != paneDevelopment {
		t.Fatalf("Development pane not focused: %+v", m)
	}
	return h
}

// opened returns the messages of the kind T among msgs.
func opened[T any](msgs []tea.Msg) []T {
	var out []T
	for _, m := range msgs {
		if v, ok := m.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

func TestDevelopmentEnterOpensIssue(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks()})
	m := h.modal()
	press(t, h, "down")
	got := opened[ui.OpenIssueMsg](press(t, h, "enter"))
	if len(got) != 1 {
		t.Fatalf("OpenIssueMsg sent %d times, want 1", len(got))
	}
	if g := got[0]; g.Number != 12 || g.Repo != repo || g.Back != ui.Modal(m) {
		t.Fatalf("OpenIssueMsg = %+v, want issue 12 of %v returning to the modal", g, repo)
	}
}

func TestDevelopmentEnterOpensPull(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks()})
	m := h.modal()
	// Past the Closes header and its two issues, to the Linked PRs header and
	// its pull request.
	for range 4 {
		press(t, h, "down")
	}
	got := opened[ui.OpenPullMsg](press(t, h, "enter"))
	if len(got) != 1 {
		t.Fatalf("OpenPullMsg sent %d times, want 1", len(got))
	}
	if g := got[0]; g.Number != 30 || g.Back != ui.Modal(m) {
		t.Fatalf("OpenPullMsg = %+v, want pull request 30 returning to the modal", g)
	}
}

func TestDevelopmentProblemToasts(t *testing.T) {
	t.Parallel()
	links := devLinks()
	links.Closes[0].Issue.Problem = "no access"
	h := openedDev(t, &fakeDev{links: links})
	press(t, h, "down")
	msgs := press(t, h, "enter")
	if n := len(opened[ui.OpenIssueMsg](msgs)); n != 0 {
		t.Fatalf("an issue that can't be read was opened %d times", n)
	}
	if n := len(opened[ui.NotifyMsg](msgs)); n != 1 {
		t.Fatalf("a toast was shown %d times, want 1", n)
	}
}

func TestDevelopmentShowsRowsUnzoomed(t *testing.T) {
	t.Parallel()
	h := started(t, newFakeService(), 160, 50, WithDevelopment(&fakeDev{links: devLinks()}))
	press(t, h, "enter")
	press(t, h, "5")
	if m := h.modal(); m == nil || m.zoom.Zoomed() {
		t.Fatalf("the modal is zoomed: %+v", m)
	}
	s := modalScreen(t, h)
	for _, want := range []string{"Crash on empty input", "Follow-up cleanup"} {
		if !strings.Contains(s, want) {
			t.Fatalf("the unzoomed pane doesn't show %q:\n%s", want, s)
		}
	}
}

func TestDevelopmentHeaderFolds(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks()})
	// Zoomed, the pane is tall enough to show its rows.
	press(t, h, "z")
	// The preview beside the tree lists the issues of a header too, so only
	// the tree's side of the screen counts.
	tree := func() string {
		var b strings.Builder
		for line := range strings.Lines(modalScreen(t, h)) {
			left, _, _ := strings.Cut(line, "││")
			b.WriteString(left)
			b.WriteByte('\n')
		}
		return b.String()
	}
	if s := tree(); !strings.Contains(s, "Crash on empty input") {
		t.Fatalf("the issue isn't shown before the fold:\n%s", s)
	}
	msgs := press(t, h, "enter")
	if n := len(opened[ui.OpenIssueMsg](msgs)) + len(opened[ui.OpenPullMsg](msgs)); n != 0 {
		t.Fatal("enter on a header opened something")
	}
	if s := tree(); strings.Contains(s, "Crash on empty input") {
		t.Fatalf("the issue is still shown after the fold:\n%s", s)
	}
}

func TestDevelopmentBackKeepsState(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks()})
	m := h.modal()
	press(t, h, "z")
	press(t, h, "down")
	press(t, h, "down")
	zoom, focus := m.zoom.Zoomed(), m.focus
	it, _ := m.dev.model.Selected()
	at := it.Ref.Target
	got := opened[ui.OpenIssueMsg](press(t, h, "enter"))
	if len(got) != 1 || got[0].Back != ui.Modal(m) {
		t.Fatalf("the issue didn't open over the modal: %+v", got)
	}
	// The modal is kept as it was, for the back key to return to.
	if m.zoom.Zoomed() != zoom || m.focus != focus {
		t.Fatalf("zoom, focus = %v, %v; want %v, %v", m.zoom.Zoomed(), m.focus, zoom, focus)
	}
	if now, _ := m.dev.model.Selected(); now.Ref.Target != at {
		t.Fatalf("cursor moved from %+v to %+v", at, now.Ref.Target)
	}
}

func TestDevelopmentKeysFallThrough(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks()})
	m := h.modal()
	press(t, h, "tab")
	if m.focus == paneDevelopment {
		t.Fatal("tab didn't leave the pane")
	}
	press(t, h, "5")
	press(t, h, "z")
	if !m.zoom.Zoomed() {
		t.Fatal("z didn't zoom")
	}
	press(t, h, "2")
	if m.focus != paneConversation {
		t.Fatalf("2 focused %v", m.focus)
	}
}

func TestDevelopmentLoadsLinks(t *testing.T) {
	t.Parallel()
	f := &fakeDev{links: devLinks()}
	h := openedDev(t, f)
	if n := len(f.reads); n != 1 {
		t.Fatalf("links read %d times, want 1", n)
	}
	if s := modalScreen(t, h); !strings.Contains(s, "2 closes") {
		t.Fatalf("the pane's frame doesn't count what it closes:\n%s", s)
	}
}
