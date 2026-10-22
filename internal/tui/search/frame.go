package search

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ownerui"
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

var paneNames = [2]string{"Results", "Preview"}

// split reports whether the results and the preview show side by side.
func (s *Section) split() bool {
	return s.width >= splitMinWidth && s.height >= splitMinHeight
}

// listOuter is the width of the results' frame.
func (s *Section) listOuter() int {
	if s.split() {
		return listWidth
	}
	return s.width
}

// panes draws the results and the preview below the query: side by side
// where the page is large enough, else the one that has the focus.
func (s *Section) panes() []string {
	rw, rh := s.resultsSize()
	results := func() []string {
		body := s.results(rw, rh)
		focused := s.area == resultsArea && !s.preview
		return s.countEdge(s.box(s.resultsLabel(), 0, focused, rw+2, rh+2, body), s.resultsLabel(), s.countLabel(), focused, rw+2)
	}
	preview := func(w int) []string {
		return s.box(paneNames[1], 1, s.area == resultsArea && s.preview, w, rh+2, strings.Split(s.pv.View(), "\n"))
	}
	switch {
	case s.split():
		return ownerui.Beside(nil, results(), preview(s.width-listWidth))
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

// box draws body in a frame w by h cells with label in its top edge. The
// pane that has the focus has the keys of the panes in its bottom edge, the
// one of pane in brackets.
func (s *Section) box(label string, pane int, focused bool, w, h int, body []string) []string {
	edge, title := s.st.frameEdge, s.st.title
	foot, footW := "", 0
	if focused && s.focused {
		edge, title = s.st.frameFocus, s.st.focusTitle
		foot, footW = s.foot(pane, w)
	}
	return ownerui.Frame(title.render(label), ansi.StringWidth(label), foot, footW, w, h, edge, s.icons.Border, s.icons.Ellipsis, body)
}

// countEdge redraws the top edge of the frame lines, w cells wide, with
// count at its right end, after label. It leaves the edge as it is when
// there is no count or no room for it.
func (s *Section) countEdge(lines []string, label, count string, focused bool, w int) []string {
	lw, cw := ansi.StringWidth(label), ansi.StringWidth(count)
	fill := w - 7 - lw - cw
	if len(lines) == 0 || cw == 0 || lw == 0 || fill < 1 {
		return lines
	}
	edge, title := s.st.frameEdge, s.st.title
	if focused && s.focused {
		edge, title = s.st.frameFocus, s.st.focusTitle
	}
	b := s.icons.Border
	lines[0] = edge.Render(b.TopLeft+b.Top) + title.render(label) +
		edge.Render(" "+strings.Repeat(b.Top, fill)+" ") + s.st.subtle.render(count) +
		edge.Render(" "+b.Top+b.TopRight)
	return lines
}

// foot is the strip of keys that reach the results and the preview, for the
// bottom edge of a frame w cells wide, and its width. The key of pane cur is
// in brackets.
func (s *Section) foot(cur, w int) (strip string, width int) {
	var keys [2]string
	if b := s.keys.Next; b.Enabled() {
		keys[1] = b.Help().Key
	}
	return ownerui.Foot(keys[:], [][]string{paneNames[:]}, cur, s.st.footOn, s.st.footOff, w-5)
}
