package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

func layoutRects(l panes.Layout) map[panes.Pane]panes.Rect {
	m := map[panes.Pane]panes.Rect{}
	for _, b := range l.Boxes {
		if b.Role == panes.Preview {
			continue
		}
		m[b.Pane] = b.Rect
	}
	return m
}

func TestWideLayout(t *testing.T) {
	t.Parallel()
	type rects = map[panes.Pane]panes.Rect
	tests := []struct {
		name    string
		w, h    int
		focus   panes.Pane
		zoom    bool
		devRows int
		want    rects
	}{
		{"80x24 is zoomed on the focus", 80, 24, paneConversation, false, 0,
			rects{paneConversation: {X: 0, Y: 0, W: 80, H: 24}}},
		{"80x24 ignores the zoom flag, there is nothing else to show", 80, 24, paneFiles, true, 0,
			rects{paneFiles: {X: 0, Y: 0, W: 80, H: 24}}},
		{"100x30 zoomed Merge is a list beside a preview", 100, 30, paneMerge, false, 0,
			rects{paneMerge: {X: 0, Y: 0, W: 46, H: 30}}},
		{"119 columns is still zoomed", 119, 30, paneDescription, false, 0,
			rects{paneDescription: {X: 0, Y: 0, W: 119, H: 30}}},
		{"120x40 wide", 120, 40, paneDescription, false, 4, rects{
			paneDescription: {X: 0, Y: 0, W: 84, H: 13}, paneConversation: {X: 0, Y: 13, W: 84, H: 27},
			paneMerge: {X: 84, Y: 0, W: 36, H: 11}, paneInvolved: {X: 84, Y: 11, W: 36, H: 4},
			paneDevelopment: {X: 84, Y: 15, W: 36, H: 6}, paneFiles: {X: 84, Y: 21, W: 36, H: 19},
		}},
		{"160x50 wide", 160, 50, paneFiles, false, 4, rects{
			paneDescription: {X: 0, Y: 0, W: 124, H: 16}, paneConversation: {X: 0, Y: 16, W: 124, H: 34},
			paneMerge: {X: 124, Y: 0, W: 36, H: 11}, paneInvolved: {X: 124, Y: 11, W: 36, H: 4},
			paneDevelopment: {X: 124, Y: 15, W: 36, H: 6}, paneFiles: {X: 124, Y: 21, W: 36, H: 29},
		}},
		{"120x20 shrinks the tallest first, Merge winning ties", 120, 20, paneDescription, false, 4, rects{
			paneDescription: {X: 0, Y: 0, W: 84, H: 6}, paneConversation: {X: 0, Y: 6, W: 84, H: 14},
			paneMerge: {X: 84, Y: 0, W: 36, H: 5}, paneInvolved: {X: 84, Y: 5, W: 36, H: 4},
			paneDevelopment: {X: 84, Y: 9, W: 36, H: 6}, paneFiles: {X: 84, Y: 15, W: 36, H: 5},
		}},
		{"no default rows for Development keeps its minimum height", 120, 40, paneDescription, false, 0, rects{
			paneMerge: {X: 84, Y: 0, W: 36, H: 11}, paneInvolved: {X: 84, Y: 11, W: 36, H: 4},
			paneDevelopment: {X: 84, Y: 15, W: 36, H: 3}, paneFiles: {X: 84, Y: 18, W: 36, H: 22},
		}},
		{"unzoomed wide with a zoom flag fills with the focus", 160, 50, paneDevelopment, true, 0,
			rects{paneDevelopment: {X: 0, Y: 0, W: 46, H: 50}}},
		{"a focus that is no pane counts as Description", 80, 24, 0, false, 0,
			rects{paneDescription: {X: 0, Y: 0, W: 80, H: 24}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := layoutRects(prLayout(tt.w, tt.h, tt.focus, tt.zoom, tt.devRows))
			for p, want := range tt.want {
				if got[p] != want {
					t.Errorf("%s = %+v, want %+v", paneTitle(p), got[p], want)
				}
			}
		})
	}
}

func TestPaneNames(t *testing.T) {
	t.Parallel()
	if got := paneTitle(paneConversation); got != "2 Conversation" {
		t.Errorf("title = %q", got)
	}
	if paneName(0) != "" || paneName(7) != "" || paneName(paneFiles) != "Files" {
		t.Error("an out-of-range pane must be nameless")
	}
}
