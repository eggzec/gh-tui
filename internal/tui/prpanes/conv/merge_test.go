package conv

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestMergeEvents(t *testing.T) {
	t.Parallel()
	request := func(who core.User, mins int, subject core.TimelineSubject) core.TimelineItem {
		it := ev(core.TimelineReviewRequested, who, mins)
		it.Subject = subject
		return it
	}
	person := func(u core.User) core.TimelineSubject { return core.TimelineSubject{User: u} }
	team := func(slug string) core.TimelineSubject { return core.TimelineSubject{Team: slug} }
	unlabeled := func(who core.User, name string, mins int) core.TimelineItem {
		it := labeled(who, name, mins)
		it.Kind = core.TimelineUnlabeled
		return it
	}
	for _, tc := range []struct {
		name  string
		items []core.TimelineItem
		hide  []Group
		want  []string
	}{
		{"same verb joins labels", []core.TimelineItem{labeled(alice, "bug", 0), labeled(alice, "api", 1)},
			nil, []string{"Alice Chen added  bug   api"}},
		{"added and removed", []core.TimelineItem{labeled(alice, "bug", 0), unlabeled(alice, "wip", 1), labeled(alice, "api", 2)},
			nil, []string{"Alice Chen added  bug   api  and removed  wip"}},
		{"a person and a team", []core.TimelineItem{request(alice, 0, person(carol)), request(alice, 1, team("eggzec/core"))},
			nil, []string{"Alice Chen requested review from Carol Diaz and core team"}},
		{"three objects", []core.TimelineItem{
			request(alice, 0, person(bert)), request(alice, 1, person(carol)), request(alice, 2, team("eggzec/core")),
		}, nil, []string{"Alice Chen requested review from Bert Moss, Carol Diaz and core team"}},
		{"eleven minutes apart", []core.TimelineItem{labeled(alice, "bug", 0), labeled(alice, "api", 11)},
			nil, []string{"Alice Chen added  bug", "Alice Chen added  api"}},
		{"ten minutes apart", []core.TimelineItem{labeled(alice, "bug", 0), labeled(alice, "api", 10)},
			nil, []string{"Alice Chen added  bug   api"}},
		{"within ten of the latest but not the first", []core.TimelineItem{
			labeled(alice, "bug", 0), labeled(alice, "api", 8), labeled(alice, "ui", 16),
		}, nil, []string{"Alice Chen added  bug   api", "Alice Chen added  ui"}},
		{"another actor between", []core.TimelineItem{labeled(alice, "bug", 0), labeled(bert, "api", 1), labeled(alice, "ui", 2)},
			nil, []string{"Alice Chen added  bug", "Bert Moss added  api", "Alice Chen added  ui"}},
		{"a comment between", []core.TimelineItem{labeled(alice, "bug", 0), comment("1"), labeled(alice, "api", 1)},
			nil, []string{"Alice Chen added  bug", "Alice Chen added  api"}},
		{"another family between", []core.TimelineItem{
			labeled(alice, "bug", 0), ev(core.TimelineClosed, alice, 1), labeled(alice, "api", 2),
		}, nil, []string{"Alice Chen added  bug", "Alice Chen closed this", "Alice Chen added  api"}},
		{"a hidden kind between merges", []core.TimelineItem{
			labeled(alice, "bug", 0), ev(core.TimelineClosed, alice, 1), labeled(alice, "api", 2),
		}, []Group{GroupState}, []string{"Alice Chen added  bug   api"}},
		{"a hidden comment between merges", []core.TimelineItem{labeled(alice, "bug", 0), comment("1"), labeled(alice, "api", 1)},
			[]Group{GroupComments}, []string{"Alice Chen added  bug   api"}},
		{"other kinds never merge", []core.TimelineItem{ev(core.TimelineClosed, alice, 0), ev(core.TimelineClosed, alice, 1)},
			nil, []string{"Alice Chen closed this", "Alice Chen closed this"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := opts(config.IconsASCII, 160)
			for _, g := range tc.hide {
				o.hide[g] = true
			}
			got := saidWith(Data{Items: tc.items}, o)
			if len(got) != len(tc.want) {
				t.Fatalf("lines = %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestCountsMerged(t *testing.T) {
	t.Parallel()
	f := newFake([][]core.TimelineItem{{
		labeled(alice, "bug", 0), labeled(alice, "api", 1), ev(core.TimelineClosed, alice, 2),
		labeled(bert, "ui", 3), labeled(alice, "wip", 30),
	}}, [][]core.Thread{nil})
	d := newDriver(t, f)
	d.all()
	if c := d.m.Choices()[GroupLabels]; c.Count != 3 {
		t.Errorf("labels = %d, want 3: the first two merge, the others stand alone", c.Count)
	}
	d.m.SetShown(allBut(GroupLabels))
	if c := d.m.Choices()[GroupLabels]; c.On || c.Count != 3 {
		t.Errorf("labels = %+v, want off and still 3", c)
	}
	if c := d.m.Choices()[GroupState]; c.Count != 1 {
		t.Errorf("state = %d, want 1", c.Count)
	}
}
