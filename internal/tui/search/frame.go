package search

import (
	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
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
		body := append([]string{s.kindsLine(rw)}, s.results(rw, rh)...)
		return s.box(s.resultsLabel(), 0, s.area == resultsArea && !s.preview, rw+2, rh+3, body)
	}
	preview := func(w int) []string {
		return s.box(paneNames[1], 1, s.area == resultsArea && s.preview, w, rh+3, []string{" " + s.previewTitle()})
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

// cyclePane moves the focus to the next pane, or the previous one: the
// query, the results, the preview.
func (s *Section) cyclePane(next bool) {
	switch {
	case s.area == inputArea:
		s.focusPane(resultsArea, !next)
	case s.preview:
		if next {
			s.focusArea(inputArea)
		} else {
			s.focusArea(resultsArea)
		}
	case next:
		s.focusPane(resultsArea, true)
	default:
		s.focusArea(inputArea)
	}
}

// previewTitle is the title of the hit under the cursor, which the preview
// shows until it renders the hit itself.
func (s *Section) previewTitle() string {
	l, ok := s.visibleHits()
	if !ok {
		return ""
	}
	h, ok := l.feed.Selected()
	switch {
	case !ok:
		return ""
	case h.Kind == core.SearchRepos:
		return h.Repo.Ref.String()
	default:
		return h.Issue.Title
	}
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

// foot is the strip of keys that reach the results and the preview, for the
// bottom edge of a frame w cells wide, and its width. The key of pane cur is
// in brackets.
func (s *Section) foot(cur, w int) (string, int) {
	var keys [2]string
	for i, b := range []key.Binding{s.keys.Panes[resultsArea], s.keys.Next} {
		if b.Enabled() {
			keys[i] = b.Help().Key
		}
	}
	return ownerui.Foot(keys[:], [][]string{paneNames[:]}, cur, s.st.footOn, s.st.footOff, w-5)
}
