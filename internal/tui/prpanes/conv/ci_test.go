package conv

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestCILine(t *testing.T) {
	t.Parallel()
	items := []core.TimelineItem{pushed(alice, sha(1), 0), pushed(alice, sha(2), 1), comment("1")}
	for _, tc := range []struct {
		name   string
		checks map[string]core.CheckState
		hide   []Group
		want   string
	}{
		{"failing", map[string]core.CheckState{sha(2): core.CheckFailing}, nil, "checks failed on 0000002"},
		{"pending", map[string]core.CheckState{sha(2): core.CheckPending}, nil, "checks pending on 0000002"},
		{"passing", map[string]core.CheckState{sha(2): core.CheckPassing}, nil, "checks passed on 0000002"},
		{"the first commit does not count", map[string]core.CheckState{sha(1): core.CheckFailing, sha(2): core.CheckPassing}, nil, "checks passed on 0000002"},
		{"none", map[string]core.CheckState{sha(2): core.CheckNone}, nil, ""},
		{"unknown", map[string]core.CheckState{sha(1): core.CheckFailing}, nil, ""},
		{"hidden with the pushes", map[string]core.CheckState{sha(2): core.CheckFailing}, []Group{GroupPushes}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := opts(config.IconsASCII, 100)
			o.checks = tc.checks
			for _, g := range tc.hide {
				o.hide[g] = true
			}
			var got []string
			for _, r := range Build(Data{Items: items}, o) {
				if r.Kind == KindCI {
					if r.Right != "" || !r.Action {
						t.Errorf("CI row = %+v, want an action with no age", r)
					}
					got = append(got, strings.TrimSpace(ansi.Strip(r.Lines[0])))
				}
			}
			switch {
			case tc.want == "" && len(got) != 0:
				t.Errorf("CI lines = %q, want none", got)
			case tc.want != "" && (len(got) != 1 || !strings.HasSuffix(got[0], tc.want)):
				t.Errorf("CI lines = %q, want one ending %q", got, tc.want)
			}
		})
	}
}

func TestCILineNotCounted(t *testing.T) {
	t.Parallel()
	items := []core.TimelineItem{pushed(alice, sha(1), 0)}
	if c := countOf(Data{Items: items}); c[GroupPushes] != 1 {
		t.Errorf("pushes = %d, want 1", c[GroupPushes])
	}
}

func TestEnterCI(t *testing.T) {
	t.Parallel()
	d := newSourceDriver(t, pushSource(func() core.PullCommits {
		return pullCommits(core.CheckPassing, core.CheckPassing, core.CheckPassing, core.CheckFailing)
	}, nil))
	d.all()
	v := d.m.views[""]
	cmd := v.enter("ci:" + sha(4))
	if cmd == nil {
		t.Fatal("enter on the CI line sent nothing")
	}
	got, ok := cmd().(EnterMsg)
	if !ok || got.Kind != KindCI || got.SHA != sha(4) {
		t.Errorf("enter on the CI line sent %+v, want kind %q and the SHA %s", got, KindCI, sha(4))
	}
}

func TestChecksGolden(t *testing.T) {
	t.Parallel()
	d := pushesFixture()
	for _, w := range []int{80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			o := opts(config.IconsNerd, w)
			o.checks = map[string]core.CheckState{
				sha(0xc3d4e5f): core.CheckPassing,
				sha(0xd4e5f60): core.CheckFailing,
				sha(0xe5f6071): core.CheckPending,
			}
			rows, _ := build(d, o, true)
			golden.RequireEqual(t, text(rows, w, false))
		})
	}
}
