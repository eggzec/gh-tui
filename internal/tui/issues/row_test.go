package issues

import (
	"image/color"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestLayoutDropsColumns(t *testing.T) {
	tests := []struct {
		width int
		want  columns
	}{
		{118, columns{chips: 2, comments: true, author: true, age: true}},
		{78, columns{chips: 1, comments: true, author: true, age: true}},
		{66, columns{chips: 1, comments: true, age: true}},
		{58, columns{chips: 1, age: true}},
		{40, columns{age: true}},
		{20, columns{}},
	}
	for _, tt := range tests {
		got := layout(tt.width, true, 4)
		if got.title < minTitle && got != (columns{title: got.title, ageWidth: got.ageWidth}) {
			t.Errorf("layout(%d) leaves the title %d cells", tt.width, got.title)
		}
		got.title, got.labels, got.ageWidth = 0, 0, 0
		if got != tt.want {
			t.Errorf("layout(%d) = %+v, want %+v", tt.width, got, tt.want)
		}
	}
}

// Without labels in the list, no row keeps room for them, and the other
// columns and the title take it.
func TestLayoutWithoutLabels(t *testing.T) {
	for _, width := range []int{118, 78, 66, 58, 40, 20} {
		with, without := layout(width, true, 4), layout(width, false, 4)
		if without.chips != 0 || without.labels != 0 {
			t.Errorf("layout(%d, false) = %+v, want no labels", width, without)
		}
		if with.comments && !without.comments || with.author && !without.author || with.age && !without.age {
			t.Errorf("layout(%d, false) = %+v, want at least the columns of %+v", width, without, with)
		}
		if with == without && with.chips > 0 {
			t.Errorf("layout(%d, false) keeps the labels", width)
		}
	}
	if got := layout(78, false, 4); got.title != 78-prefixWidth-got.right() || !got.comments || !got.author || !got.age {
		t.Errorf("layout(78, false) = %+v, want every other column", got)
	}
}

func TestChipColors(t *testing.T) {
	for _, hex := range []string{"", "zzzzzz", "12345", "#12"} {
		if _, _, ok := chipColors(hex, true); ok {
			t.Errorf("chipColors(%q) accepted an invalid color", hex)
		}
	}
	lum := func(c color.Color) uint32 {
		r, g, b, _ := c.RGBA()
		return (r + g + b) / 3 >> 8
	}
	// A loud yellow and a near black both come out legible and quiet.
	for _, hex := range []string{"fbca04", "#000", "d73a4a", "ffffff"} {
		fg, bg, ok := chipColors(hex, true)
		if !ok {
			t.Fatalf("chipColors(%q) rejected a valid color", hex)
		}
		if lum(fg) < 150 || lum(bg) > 90 {
			t.Errorf("dark chip of %s: fg %d, bg %d; want a light fg on a dim bg", hex, lum(fg), lum(bg))
		}
		fg, bg, _ = chipColors(hex, false)
		if lum(fg) > 110 || lum(bg) < 200 {
			t.Errorf("light chip of %s: fg %d, bg %d; want a dark fg on a pale bg", hex, lum(fg), lum(bg))
		}
	}
}

// A list of issues none of which has labels keeps no column for them.
func TestRowsWithoutLabels(t *testing.T) {
	issues := sampleIssues(6)
	for i := range issues {
		issues[i].Labels = nil
	}
	h := started(t, newFakeService(issues), 120, 12)
	if h.cols.chips != 0 {
		t.Errorf("columns = %+v, want no labels", h.cols)
	}
	h = started(t, newFakeService(sampleIssues(6)), 120, 12)
	if h.cols.chips == 0 {
		t.Errorf("columns = %+v, want labels", h.cols)
	}
}

// A reload that brings labels to the same number of issues makes room
// for them, which the list then keeps.
func TestRowsGainLabelsOnSync(t *testing.T) {
	issues := sampleIssues(6)
	for i := range issues {
		issues[i].Labels = nil
	}
	svc := newFakeService(issues)
	h := started(t, svc, 120, 12)
	if h.cols.chips != 0 {
		t.Fatalf("columns = %+v, want no labels", h.cols)
	}
	labels := sampleIssues(1)[0].Labels
	for _, it := range issues {
		svc.set(it.Number, func(it *core.Issue) { it.Labels = labels })
	}
	run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	if h.cols.chips == 0 {
		t.Errorf("columns = %+v, want labels", h.cols)
	}
	if view := h.View(); !strings.Contains(view, labels[0].Name) {
		t.Errorf("view has no label %q:\n%s", labels[0].Name, view)
	}
	// Issues that drop out of the list, as they do while it scrolls, or
	// lose their labels don't move the rows: the list keeps the room.
	for _, it := range issues {
		svc.set(it.Number, func(it *core.Issue) { it.Labels = nil })
	}
	run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	if h.cols.chips == 0 {
		t.Errorf("columns = %+v after the labels went, want the room kept", h.cols)
	}
}

// Another repository with as many issues, none with labels, keeps no room
// for them, and switching back makes it again.
func TestRowsLabelsFollowRepo(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	plain := sampleIssues(6)
	for i := range plain {
		plain[i].Repo, plain[i].Labels = other, nil
	}
	h := started(t, newFakeService(append(sampleIssues(6), plain...)), 120, 12)
	if h.cols.chips == 0 {
		t.Fatalf("columns = %+v, want labels", h.cols)
	}
	n := h.list.Len()
	run(t, h, h.Update(ui.RepoMsg{Repo: other}))
	if h.list.Len() != n {
		t.Fatalf("list has %d issues in %s, want %d as in %s", h.list.Len(), other, n, testRepo)
	}
	if h.cols.chips != 0 {
		t.Errorf("columns = %+v in %s, want no labels", h.cols, other)
	}
	run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
	if h.cols.chips == 0 {
		t.Errorf("columns = %+v back in %s, want labels", h.cols, testRepo)
	}
}
