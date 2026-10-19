package conv

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPeek(t *testing.T) {
	t.Parallel()
	review1 := Narrow{
		Item:   func(it core.TimelineItem) bool { return it.ID == "R1" },
		Thread: func(th core.Thread) bool { return th.ReviewID == "R1" },
	}
	comments := Narrow{Item: func(it core.TimelineItem) bool { return it.Kind == core.TimelineComment }}

	t.Run("draws the narrowed view", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		d.all()
		first, ok := d.m.Peek("p", review1, 60)
		if !ok || first == "" {
			t.Fatalf("Peek = %q, %v; want the review drawn", first, ok)
		}
		other, ok := d.m.Peek("q", comments, 60)
		if !ok || other == first {
			t.Errorf("Peek of the comments = %q, %v; want something other than the review's", other, ok)
		}
		if again, _ := d.m.Peek("p", review1, 60); again != first {
			t.Errorf("Peek again drew %q, want %q", again, first)
		}
	})

	t.Run("leaves the shown view and size", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		d.all()
		shown, cur, w, h := d.m.View(), d.m.cur, d.m.width, d.m.height
		if _, ok := d.m.Peek("p", review1, 40); !ok {
			t.Fatal("Peek drew nothing")
		}
		if got := d.m.View(); got != shown {
			t.Errorf("the shown view changed:\n%s\nwant\n%s", got, shown)
		}
		if d.m.cur != cur || d.m.width != w || d.m.height != h {
			t.Errorf("view %q at %dx%d, want %q at %dx%d", d.m.cur, d.m.width, d.m.height, cur, w, h)
		}
	})

	t.Run("nothing to show", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		d.all()
		none := Narrow{Item: func(core.TimelineItem) bool { return false }, Thread: func(core.Thread) bool { return false }}
		if v, ok := d.m.Peek("p", none, 60); ok {
			t.Errorf("Peek of nothing = %q, true", v)
		}
		if v, ok := d.m.Peek("", review1, 60); ok {
			t.Errorf("Peek of the full view's key = %q, true", v)
		}
		if v, ok := d.m.Peek("p", review1, 1); ok {
			t.Errorf("Peek too narrow = %q, true", v)
		}
	})

	t.Run("not read yet", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		if v, ok := d.m.Peek("p", review1, 60); ok {
			t.Errorf("Peek before the reads = %q, true", v)
		}
	})
}
