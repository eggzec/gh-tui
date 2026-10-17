package conv

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// pushSource is a timeline of four commits in one push, whose checks are read
// from commits, kept first when cached is set.
func pushSource(commits, cached func() core.PullCommits) Source {
	items := []core.TimelineItem{
		pushed(alice, sha(1), 0), pushed(alice, sha(2), 1), pushed(alice, sha(3), 2), pushed(alice, sha(4), 3),
	}
	s := Source{
		Timeline: func(context.Context, string) (core.Page[core.TimelineItem], error) {
			return core.Page[core.TimelineItem]{Items: items}, nil
		},
	}
	if commits != nil {
		s.Commits = func(context.Context) (core.PullCommits, error) { return commits(), nil }
	}
	if cached != nil {
		s.CachedCommits = func() (core.PullCommits, bool) { return cached(), true }
	}
	return s
}

func pullCommits(states ...core.CheckState) core.PullCommits {
	var p core.PullCommits
	for i, st := range states {
		// The host may write the OID in either case.
		p.Commits = append(p.Commits, core.PullCommit{OID: strings.ToUpper(sha(i + 1)), Checks: st})
	}
	return p
}

func newSourceDriver(t *testing.T, s Source) *driver {
	t.Helper()
	d := &driver{t: t, m: New(s, WithSize(100, 40), WithOptions(opts0()), WithTreeKeys(treeKeys()))}
	d.q = []tea.Cmd{d.m.Init()}
	return d
}

// commitRights returns the Right of the commit rows by OID.
func commitRights(d *driver) map[string]string {
	rights := map[string]string{}
	for _, r := range d.m.views[""].rows {
		if r.Kind == KindCommit {
			rights[strings.TrimPrefix(r.ID, "k:")] = r.Right
		}
	}
	return rights
}

func TestCommitIcons(t *testing.T) {
	t.Parallel()
	st := opts0().Styles
	pass, fail, pending := st.glyphs.Render(ui.GlyphCheckPass), st.glyphs.Render(ui.GlyphCheckFail), st.glyphs.Render(ui.GlyphCheckPending)
	want := map[string]string{sha(1): pass, sha(2): fail, sha(3): pending, sha(4): ""}
	check := func(t *testing.T, d *driver, want map[string]string) {
		t.Helper()
		got := commitRights(d)
		if len(got) != len(want) {
			t.Fatalf("commit rows = %v, want %v", got, want)
		}
		for oid, icon := range want {
			if got[oid] != icon {
				t.Errorf("commit %s Right = %q, want %q", oid[:7], got[oid], icon)
			}
		}
	}
	read := func() core.PullCommits {
		return pullCommits(core.CheckPassing, core.CheckFailing, core.CheckPending, core.CheckNone)
	}

	t.Run("read", func(t *testing.T) {
		t.Parallel()
		d := newSourceDriver(t, pushSource(read, nil))
		d.all()
		check(t, d, want)
	})
	t.Run("no commits source shows no icon", func(t *testing.T) {
		t.Parallel()
		d := newSourceDriver(t, pushSource(nil, nil))
		d.all()
		check(t, d, map[string]string{sha(1): "", sha(2): "", sha(3): "", sha(4): ""})
	})
	t.Run("the kept list shows at once and the read replaces it", func(t *testing.T) {
		t.Parallel()
		d := newSourceDriver(t, pushSource(read, func() core.PullCommits {
			return pullCommits(core.CheckPending, core.CheckPending, core.CheckPending, core.CheckPending)
		}))
		// Only the cached messages are run: the read is still queued.
		for range 3 {
			d.step()
		}
		if got := d.m.checks[sha(1)]; got != core.CheckPending {
			t.Fatalf("after the kept list the state is %v, want pending", got)
		}
		d.all()
		if got := d.m.checks[sha(1)]; got != core.CheckPassing {
			t.Errorf("after the read the state is %v, want passing", got)
		}
	})
	t.Run("a failed read leaves no error line", func(t *testing.T) {
		t.Parallel()
		s := pushSource(nil, nil)
		s.Commits = func(context.Context) (core.PullCommits, error) { return core.PullCommits{}, errors.New("boom") }
		d := newSourceDriver(t, s)
		d.all()
		check(t, d, map[string]string{sha(1): "", sha(2): "", sha(3): "", sha(4): ""})
		if err := d.m.Status().Err; err != nil {
			t.Errorf("Status.Err = %v, want none", err)
		}
	})
	t.Run("a stale message is dropped", func(t *testing.T) {
		t.Parallel()
		d := newSourceDriver(t, pushSource(read, nil))
		d.all()
		for name, msg := range map[string]commitsMsg{
			"other model":  {model: d.m.id + 1000, seq: d.m.seq, value: pullCommits(core.CheckFailing)},
			"earlier read": {model: d.m.id, seq: d.m.seq - 1, value: pullCommits(core.CheckFailing)},
		} {
			m, cmd := d.m.Update(msg)
			d.m = m
			if cmd != nil {
				t.Errorf("%s: a stale message sent a command", name)
			}
			if got := d.m.checks[sha(1)]; got != core.CheckPassing {
				t.Errorf("%s: the state is %v, want passing", name, got)
			}
		}
	})
}
