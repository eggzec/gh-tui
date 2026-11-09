package search

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// The page splits below the query into Results and Preview where it has
// room: as wide as the dashboard needs for its panes, and tall enough for
// both to show rows. A smaller page shows one of them at a time.
const (
	splitMinWidth  = 100
	splitMinHeight = 30
	// listWidth is the width of the results' frame beside the preview.
	listWidth = 46
)

// split reports whether the results and the preview show side by side.
func (s *Section) split() bool {
	return s.width >= splitMinWidth && s.height >= splitMinHeight
}

// listOuter is the width of the results' frame.
func (s *Section) listOuter() int {
	if s.split() {
		return s.listSplit()
	}
	return s.width
}

// listSplit is the width of the results' frame beside the preview: its
// default, moved by the user's divider, each pane keeping panes.MinWidth.
func (s *Section) listSplit() int {
	if s.dividers == nil {
		return listWidth
	}
	return s.dividers.List.Of(listWidth, panes.MinWidth, s.width-panes.MinWidth)
}

// SetDividers gives the page the dividers the user moved, which it shares
// with the other views. With none the line stays where it starts.
func (s *Section) SetDividers(d *ui.Dividers) {
	s.dividers = d
	s.layout()
}

// moveDivider moves the line between the results and the preview by dx
// steps, or puts every divider back.
func (s *Section) moveDivider(dx int, reset bool) {
	if reset {
		s.dividers.Reset()
	} else {
		s.dividers.Move(&s.dividers.List, dx, listWidth, panes.MinWidth, s.width-panes.MinWidth)
	}
	s.layout()
}

// canDivide reports whether a line between the panes shows now.
func (s *Section) canDivide() bool { return s.dividers != nil && s.split() }

// panes draws the results and the preview below the query: side by side
// where the page is large enough, else the one that has the focus.
func (s *Section) panes() []string {
	rw, rh := s.resultsSize()
	results := func() []string {
		w := rw + framePad
		c := panes.Content{
			TopLeft:  s.titleForms(s.resultsLabel(), w),
			TopRight: s.countForms(),
			Body:     strings.Join(s.results(rw, rh), "\n"),
			Focus:    s.focused && s.area == resultsArea && !s.preview,
		}
		return strings.Split(s.st.frames.Frame(c, w, rh+2), "\n")
	}
	preview := func(w int) []string {
		c := panes.Content{
			TopLeft: s.titleForms(paneDefs[1].Name, w),
			Body:    s.pv.View(),
			Focus:   s.focused && s.area == resultsArea && s.preview,
		}
		return strings.Split(s.st.frames.Frame(c, w, rh+2), "\n")
	}
	switch {
	case s.split():
		return ui.Beside(nil, results(), preview(s.width-s.listOuter()))
	case s.preview:
		return preview(s.width)
	default:
		return results()
	}
}

// cyclePane moves the focus between the results and the preview, either
// way; the query is not part of the cycle, and a press from it goes to the
// results.
func (s *Section) cyclePane() {
	if s.area == resultsArea && !s.preview {
		s.focusPane(resultsArea, true)
		return
	}
	s.focusPane(resultsArea, false)
}
