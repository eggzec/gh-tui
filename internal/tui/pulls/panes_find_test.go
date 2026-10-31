package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// changedPanes returns the modal of the first pull request, opened over a
// service whose pull requests have a head and change the files given.
func changedPanes(t *testing.T, head string, files ...core.CommitFile) (*host, *panesModal) {
	t.Helper()
	svc := newFakeService()
	for i := range svc.pulls {
		svc.pulls[i].HeadSHA = head
	}
	svc.changed = files
	h := started(t, svc, 160, 30)
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the modal")
	}
	return h, m
}

// typeQuery types the letters of q into the finder.
func typeQuery(t *testing.T, h *host, q string) {
	t.Helper()
	for _, r := range q {
		press(t, h, string(r))
	}
}

func finderTop(m *panesModal) *finderOverlay {
	o, _ := m.top().(*finderOverlay)
	return o
}

func changedSample() []core.CommitFile {
	return []core.CommitFile{
		{Path: "README.md", Additions: 2},
		{Path: "internal/config/default.yaml", Additions: 12, Deletions: 3},
		{Path: "internal/tui/pulls/panes.go", Additions: 1, Deletions: 1},
	}
}

func TestFindChoosesAChangedFile(t *testing.T) {
	t.Parallel()
	h, m := changedPanes(t, "a1b2c3d", changedSample()...)
	m.nav.SetFocus(paneConversation)
	press(t, h, "ctrl+p")
	o := finderTop(m)
	if o == nil {
		t.Fatal("the find key opened no finder")
	}
	if got := o.f.Total(); got != len(changedSample()) {
		t.Errorf("the finder lists %d files, want the %d the pull request changes", got, len(changedSample()))
	}
	typeQuery(t, h, "yaml")
	if it, ok := o.f.Selected(); !ok || it.Path != "internal/config/default.yaml" {
		t.Fatalf("typing yaml selected %v, want the yaml file", it)
	}
	press(t, h, "enter")
	if finderTop(m) != nil || len(m.overlays) != 1 {
		t.Error("choosing a file left the finder open")
	}
	if m.nav.Focus() != paneFiles || m.found != "internal/config/default.yaml" {
		t.Errorf("focus %v and found %q, want the Files pane and the file chosen", m.nav.Focus(), m.found)
	}
}

func TestFindEscClosesOnlyTheFinder(t *testing.T) {
	t.Parallel()
	h, m := changedPanes(t, "a1b2c3d", changedSample()...)
	m.nav.SetFocus(paneConversation)
	press(t, h, "ctrl+p")
	if finderTop(m) == nil {
		t.Fatal("the find key opened no finder")
	}
	press(t, h, "esc")
	if m.top() != nil || m.closed || h.modal() != m {
		t.Errorf("esc left open %v, modal closed %v, want only the finder closed", m.top() != nil, m.closed)
	}
	if m.nav.Focus() != paneConversation || m.found != "" {
		t.Errorf("focus %v and found %q after esc, want them as before", m.nav.Focus(), m.found)
	}
}

func TestFindNeedsTheHead(t *testing.T) {
	t.Parallel()
	h, m := changedPanes(t, "", changedSample()...)
	if m.canFind() {
		t.Fatal("canFind is true without a head")
	}
	press(t, h, "ctrl+p")
	if m.top() != nil {
		t.Errorf("the find key opened %T without a head", m.top())
	}
}
