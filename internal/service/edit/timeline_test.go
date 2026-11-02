package edit

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func timelinePage() core.Page[core.TimelineItem] {
	return core.Page[core.TimelineItem]{Items: []core.TimelineItem{
		{ID: "a", Body: "A"},
		{ID: "b", Body: "B"},
	}}
}

func TestOnTimeline(t *testing.T) {
	at := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	in := timelinePage()
	out, changed := OnTimeline("b", "New", at)(in)
	if !changed || out.Items[1].Body != "New" || !out.Items[1].UpdatedAt.Equal(at) {
		t.Errorf("out = %+v, %v; want b set", out.Items, changed)
	}
	if in.Items[1].Body != "B" {
		t.Errorf("the input page was changed: %+v", in.Items)
	}
	if _, changed := OnTimeline("zz", "New", at)(in); changed {
		t.Error("an absent item was reported changed")
	}
}

func TestDropFromTimeline(t *testing.T) {
	in := timelinePage()
	out, changed := DropFromTimeline("a")(in)
	if !changed || len(out.Items) != 1 || out.Items[0].ID != "b" {
		t.Errorf("out = %+v, %v; want only b", out.Items, changed)
	}
	if len(in.Items) != 2 || in.Items[0].ID != "a" {
		t.Errorf("the input page was changed: %+v", in.Items)
	}
	if _, changed := DropFromTimeline("zz")(in); changed {
		t.Error("an absent item was reported changed")
	}
}
