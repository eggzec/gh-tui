package pulls

import (
	"maps"
	"slices"
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// findingPrefix starts the ID of the block that shows what the checks found on
// a line of the diff.
const findingPrefix = "finding:"

// findingBlocks returns one block under each new-side line that the checks left
// annotations on, with a row for each, worst level first. The annotations are
// those of the last commit of the range, so there are none while they are read
// or when the read failed. A line that the diff does not show gets none, and
// neither does a diff taken at an older commit, whose lines are not those the
// annotations are on.
func (o *diffOverlay) findingBlocks() []diff.Block {
	m := o.m
	_, sha, _, ok := m.rangeEnds()
	if o.at != "" || !ok || sha != m.files.finds.sha || len(m.files.finds.byPath) == 0 {
		return nil
	}
	var blocks []diff.Block
	for _, path := range slices.Sorted(maps.Keys(m.files.finds.byPath)) {
		if o.mode == diffview.ThisFile && path != o.path {
			continue
		}
		byLine := m.files.finds.byPath[path].ByLine()
		for _, line := range slices.Sorted(maps.Keys(byLine)) {
			blocks = append(blocks, o.findingBlock(path, line, byLine[line]))
		}
	}
	return blocks
}

// findingBlock is the rows under a line, one for each of its annotations.
func (o *diffOverlay) findingBlock(path string, line int, notes []files.Note) diff.Block {
	m := o.m
	return diff.Block{
		ID:         findingPrefix + path + ":" + strconv.Itoa(line),
		Anchor:     diff.Pos{Path: path, Side: diff.NewSide, Line: line},
		OnlyAtLine: true,
		Passive:    true,
		Render: func(width int, _ bool) []string {
			st := m.filesStyles()
			bar := "  " + m.theme.Subtle.Render("▎ ")
			rows := make([]string, 0, len(notes))
			for _, n := range notes {
				text := termtext.OneLine(n.Message)
				if n.App != "" {
					text = termtext.OneLine(n.App) + ": " + text
				}
				rows = append(rows, ansi.Truncate(bar+st.Level(n.Level)+" "+m.theme.Subtle.Render(text), width, m.icons.Ellipsis))
			}
			return rows
		},
	}
}

// diffOverlays returns the diffs that show the checks' findings and the
// drafts: first, if given, the one the modal has open, and the zoomed Files
// pane's.
func (m *panesModal) diffOverlays(first *diffOverlay) []*diffOverlay {
	var shown []*diffOverlay
	if first != nil {
		shown = append(shown, first)
	}
	if pv := &m.files.pv; pv.on && pv.o != nil && !slices.Contains(shown, pv.o) {
		shown = append(shown, pv.o)
	}
	for _, ov := range m.overlays {
		if d, ok := ov.overlay.(*diffOverlay); ok && !slices.Contains(shown, d) {
			shown = append(shown, d)
		}
	}
	return shown
}

// showFindings puts the findings in the diffs that are open, as they changed.
func (m *panesModal) showFindings() {
	for _, d := range m.diffOverlays(nil) {
		d.setBlocks()
	}
}
