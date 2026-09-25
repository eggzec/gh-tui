package actions

import (
	"context"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// capsOf serves the caps of repo, and counts its reads.
type capsOf struct {
	caps   core.RepoCaps
	cached bool
	gets   int
}

func (c *capsOf) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	return core.Repo{Ref: ref, Caps: c.caps}, c.cached
}

func (c *capsOf) Get(_ context.Context, ref core.RepoRef) (core.Repo, error) {
	c.gets++
	c.cached = true
	return core.Repo{Ref: ref, Caps: c.caps}, nil
}

// offered returns what the enabled change keys do, as help shows them.
func offered(m *Modal) []string {
	var out []string
	h := helpKeys{m: m}
	for _, b := range h.changes() {
		if d := b.Help().Desc; b.Enabled() && d != m.keys.Filter.Help().Desc && d != m.keys.Open.Help().Desc {
			out = append(out, d)
		}
	}
	return out
}

func TestChangesNeedWriteAccess(t *testing.T) {
	src := &capsOf{caps: core.RepoCaps{Known: true, Permission: core.PermissionRead}}
	f := newFake()
	m, h := newModal(t, f, wideW, wideH, WithRepos(src))
	if src.gets != 1 || m.caps != src.caps {
		t.Fatalf("caps %+v after %d reads, want them read once", m.caps, src.gets)
	}
	if got := offered(m); len(got) != 0 {
		t.Errorf("help offers %v with read access", got)
	}
	rerun := ui.NotifyMsg{Level: toast.Info, Text: "Re-running needs write access to charmbracelet/bubbletea."}
	for _, k := range []string{"ctrl+r", "R"} {
		h.keys(k)
		if m.ask != nil || !slices.Contains(h.take(), any(rerun)) {
			t.Errorf("%s asked %+v, want the toast %q", k, m.ask, rerun.Text)
		}
	}
	for m.run.ID != runningRun {
		h.keys("down")
	}
	h.keys("x")
	cancel := ui.NotifyMsg{Level: toast.Info, Text: "Cancelling needs write access to charmbracelet/bubbletea."}
	if m.ask != nil || !slices.Contains(h.take(), any(cancel)) {
		t.Errorf("x asked %+v, want the toast %q", m.ask, cancel.Text)
	}
	if len(f.sent) != 0 {
		t.Errorf("sent %v, want nothing", f.sent)
	}

	// Once the viewer may write, they may cancel.
	h.send(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionWrite}})
	if got := offered(m); !slices.Equal(got, []string{"cancel run"}) {
		t.Errorf("help offers %v with write access, want the cancel", got)
	}
	h.keys("x", "y")
	if !slices.Equal(f.sent, []string{"cancel"}) {
		t.Errorf("sent %v, want the cancel", f.sent)
	}
}

func TestCachedCapsAreNotReadAgain(t *testing.T) {
	src := &capsOf{caps: core.RepoCaps{Known: true, Permission: core.PermissionAdmin}, cached: true}
	m, _ := newModal(t, newFake(), wideW, wideH, WithRepos(src))
	if src.gets != 0 || m.caps != src.caps {
		t.Errorf("caps %+v after %d reads, want the cached ones", m.caps, src.gets)
	}
}
