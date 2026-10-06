package issues

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fullRoom is the room of a list whose labels fill the widest column.
var fullRoom = labelRoom{labelsCap(1), labelsCap(2)}

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
		got := layout(tt.width, fullRoom, 4)
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
		with, without := layout(width, fullRoom, 4), layout(width, labelRoom{}, 4)
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
	if got := layout(78, labelRoom{}, 4); got.title != 78-prefixWidth-got.right() || !got.comments || !got.author || !got.age {
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

// The labels column is as wide as the widest labels the list has loaded,
// not the widest it could be, so short labels leave the title the rest.
func TestRowsLabelsFitContent(t *testing.T) {
	bug := core.Label{Name: "bug", Color: "d73a4a"}
	uiLabel := core.Label{Name: "ui", Color: "fbca04"}
	docs := core.Label{Name: "docs", Color: "0075ca"}
	tests := []struct {
		name   string
		labels []core.Label
		// want is the row's labels, as the column shows them.
		want string
	}{
		{"one label", []core.Label{bug}, " bug "},
		{"two labels", []core.Label{bug, uiLabel}, " bug   ui "},
		{"more than two", []core.Label{bug, uiLabel, docs}, " bug   ui  +1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := sampleIssues(6)
			for i := range issues {
				issues[i].Labels = nil
			}
			issues[2].Labels = tt.labels
			h := started(t, newFakeService(issues), 120, 12)
			if h.cols.chips != 2 || h.cols.labels != len(tt.want) {
				t.Errorf("columns = %+v, want two chips in %d cells", h.cols, len(tt.want))
			}
			if full := layout(h.colsWidth, fullRoom, h.dates.Width()); h.cols.title <= full.title {
				t.Errorf("title of %d cells, want more than the %d of the widest labels", h.cols.title, full.title)
			}
			row := ansi.Strip(h.renderRow(issues[2], false, h.colsWidth))
			if !strings.Contains(row, "  "+tt.want+"  ") {
				t.Errorf("row %q, want %q in it", row, tt.want)
			}
			if w := ansi.StringWidth(row); w != h.colsWidth {
				t.Errorf("row is %d cells, want %d", w, h.colsWidth)
			}
		})
	}
}

// While the list is kept, the labels column grows to wider labels that a
// reload brings, and keeps its width when they get shorter again.
func TestRowsLabelsOnlyGrow(t *testing.T) {
	issues := sampleIssues(6)
	for i := range issues {
		issues[i].Labels = []core.Label{{Name: "bug", Color: "d73a4a"}}
	}
	svc := newFakeService(issues)
	h := started(t, svc, 120, 12)
	narrow := h.cols.labels
	wide := []core.Label{{Name: "enhancement", Color: "a2eeef"}}
	svc.set(issues[0].Number, func(it *core.Issue) { it.Labels = wide })
	run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	grown := h.cols.labels
	if grown <= narrow {
		t.Fatalf("labels column of %d cells after wider labels came, want more than %d", grown, narrow)
	}
	svc.set(issues[0].Number, func(it *core.Issue) { it.Labels = issues[1].Labels })
	run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	if h.cols.labels != grown {
		t.Errorf("labels column of %d cells after the wide labels went, want it kept at %d", h.cols.labels, grown)
	}
}

// A new theme sizes the labels column again, since the chips it draws may
// be of another width.
func TestRowsLabelsRescanOnTheme(t *testing.T) {
	issues := sampleIssues(6)
	for i := range issues {
		issues[i].Labels = []core.Label{{Name: "bug", Color: "d73a4a"}}
	}
	h := started(t, newFakeService(issues), 120, 12)
	want := h.cols.labels
	h.room = fullRoom
	h.SetTheme(h.theme)
	if h.room == fullRoom || h.cols.labels != want {
		t.Errorf("after a new theme the room is %v and the column %d cells, want %d", h.room, h.cols.labels, want)
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
