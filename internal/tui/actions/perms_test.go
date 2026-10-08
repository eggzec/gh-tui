package actions

import (
	"context"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
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

// offered returns what the enabled change keys do.
func offered(m *Modal) []string {
	changes := []string{"cancel run", "rerun failed", "rerun all", "rerun job"}
	return slices.DeleteFunc(uitest.Enabled(m.KeyLayers()), func(d string) bool { return !slices.Contains(changes, d) })
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
	for _, k := range []string{"R", "E"} {
		h.keys(k)
		if m.ask != nil || !slices.Contains(h.take(), any(rerun)) {
			t.Errorf("%s asked %+v, want the toast %q", k, m.ask, rerun.Text)
		}
	}
	for m.run.ID != runningRun {
		h.keys("down")
	}
	h.keys("X")
	cancel := ui.NotifyMsg{Level: toast.Info, Text: "Cancelling needs write access to charmbracelet/bubbletea."}
	if m.ask != nil || !slices.Contains(h.take(), any(cancel)) {
		t.Errorf("X asked %+v, want the toast %q", m.ask, cancel.Text)
	}
	if len(f.sent) != 0 {
		t.Errorf("sent %v, want nothing", f.sent)
	}

	// Once the viewer may write, they may cancel.
	h.send(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionWrite}})
	if got := offered(m); !slices.Equal(got, []string{"cancel run"}) {
		t.Errorf("help offers %v with write access, want the cancel", got)
	}
	h.keys("X", "y")
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

func TestChangesNeedRepo(t *testing.T) {
	src := &capsOf{caps: core.RepoCaps{Known: true, Permission: core.PermissionAdmin}, cached: true}
	v := ui.NewVoice(config.Default().Keys, "")
	v.Token = uitest.Token(&uitest.Checker{A: uitest.Classic("public_repo")})
	f := newFake()
	m, h := newModal(t, f, wideW, wideH, WithRepos(src), WithVoice(v))
	if got := offered(m); len(got) != 0 {
		t.Errorf("help offers %v without repo", got)
	}
	rerun := ui.NotifyMsg{Level: toast.Info, Text: "Re-running needs the repo scope · :auth to grant it"}
	h.keys("E")
	if m.ask != nil || !slices.Contains(h.take(), any(rerun)) {
		t.Errorf("E asked %+v, want the toast %q", m.ask, rerun.Text)
	}
	for m.run.ID != runningRun {
		h.keys("down")
	}
	h.keys("X")
	cancel := ui.NotifyMsg{Level: toast.Info, Text: "Cancelling needs the repo scope · :auth to grant it"}
	if m.ask != nil || !slices.Contains(h.take(), any(cancel)) {
		t.Errorf("X asked %+v, want the toast %q", m.ask, cancel.Text)
	}
	if len(f.sent) != 0 {
		t.Errorf("sent %v, want nothing", f.sent)
	}
}
