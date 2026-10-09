package refs

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// styles are the step's own, built once per theme.
type styles struct {
	title, rule, note, chip, subtle lipgloss.Style
	// states draw the icon of each state of an item, and unread the icon of
	// one that can't be read.
	states [ui.NumStates]lipgloss.Style
	unread lipgloss.Style
}

func newStyles(t ui.Theme) styles {
	st := styles{title: t.Title, rule: t.Subtle, note: t.Muted, chip: t.Accent, subtle: t.Subtle, unread: t.Muted}
	for i := range st.states {
		st.states[i] = t.State(ui.State(i))
	}
	return st
}

// SetSize sizes the step to the room inside the frame.
func (s *Step) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
}

// SetTheme styles the step and the tree in it.
func (s *Step) SetTheme(t ui.Theme) {
	s.voice.Icons = &s.opts.icons
	s.theme = t
	s.st = newStyles(t)
	s.errs = t.Errors(s.opts.icons)
	s.spin.Style = t.Accent
	s.spin.Spinner = s.opts.icons.SpinnerOr(s.spin.Spinner)
	ts := t.Tree(s.opts.icons)
	// The groups are told apart by their folds, so the rows below one
	// are indented by plain space, not by a guide.
	ts.GuideGlyph = " "
	s.tree.SetStyles(ts)
	s.tree.SetIcons(s.icon)
	ps := cmdline.DefaultStyles(t.Dark)
	ps.Prompt, ps.Text, ps.Cursor = t.Accent.Bold(true), t.Text, t.Accent
	ps.Placeholder, ps.Ellipsis = t.Subtle, s.opts.icons.Ellipsis
	s.prompt.SetStyles(ps)
	s.layout()
}

// icon draws the icon of a row: the state of an item, in the colour the
// lists give it, or a question mark for one that can't be read.
func (s *Step) icon(n tree.Node, _ bool) string {
	v, ok := n.Value.(itemValue)
	switch {
	case !ok:
		return ""
	case v.ref.Problem != "":
		return s.st.unread.Render("?")
	}
	st := stateOf(v.ref)
	return s.st.states[st].Render(s.opts.icons.State(st))
}

// bodyHeight is the height the tree has: what is left under the title, the
// rule, a note, and the prompt of the filter while it is open.
func (s *Step) bodyHeight() int {
	h := s.height - 2
	if s.note() != "" {
		h--
	}
	if s.prompt.Focused() {
		h--
	}
	return max(h, 0)
}

// layout sizes the tree and the prompt to the room.
func (s *Step) layout() {
	s.tree.SetSize(s.width, s.bodyHeight())
	s.prompt.SetSize(s.width, 1)
}

// View renders the step in exactly the size of the last SetSize: the title
// of the item over a rule, a note if the links shown are not the latest,
// and the links, or why there are none. The prompt of the filter takes the
// last line while it is open.
func (s *Step) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	w := s.width
	lines := []string{s.titleLine(w), s.st.rule.Render(strings.Repeat(s.opts.icons.Border.Top, w))}
	if n := s.note(); n != "" {
		lines = append(lines, ui.Fit(s.st.note.Render(termtext.Truncate(n, w, s.opts.icons.Ellipsis)), w))
	}
	h := s.bodyHeight()
	lines = append(lines, ui.FitLines(s.body(w), w, h)...)
	if s.prompt.Focused() {
		lines = append(lines, ui.Fit(s.prompt.View(), w))
	}
	return strings.Join(ui.FitLines(lines, w, s.height), "\n")
}

// titleLine shows the number and title of the item, and at the right the
// filter in force while the prompt is closed.
func (s *Step) titleLine(w int) string {
	left := "#" + itoa(s.self.Number)
	if s.opts.item != nil {
		if t := ui.OneLine(s.opts.item().Title); t != "" {
			left += " " + t
		}
	}
	left = s.st.title.Render(left)
	if s.filter == "" || s.prompt.Focused() {
		return ui.Fit(termtext.Truncate(left, w, s.opts.icons.Ellipsis), w)
	}
	return ui.Spread(left, s.st.chip.Render(promptFilter+" "+ui.OneLine(s.filter)), w, s.opts.icons.Ellipsis)
}

// note says why the links shown may not be the latest: GitHub couldn't be
// reached or rate limited the read, or part of the links failed to read.
func (s *Step) note() string {
	if !s.loaded {
		return ""
	}
	r := s.refs
	at := ""
	if !r.ReadAt.IsZero() {
		at = " at " + ui.Clock(r.ReadAt, s.opts.now())
	}
	switch {
	case r.Offline:
		return "Offline: showing the links read" + at + "."
	case r.Limited:
		return "Rate limited: showing the links read" + at + "."
	case r.Failed > 0:
		why := ""
		if r.FailedWhy != "" {
			why = " (" + strings.ToLower(r.FailedWhy) + ")"
		}
		text := itoa(r.Failed) + " " + plural(r.Failed, "link") + " weren't read" + why + "."
		if r.Failed == 1 {
			text = "1 link wasn't read" + why + "."
		}
		if h := s.keys.Refresh.Help().Key; h != "" {
			text += " " + h + " reads again."
		}
		return text
	}
	return ""
}

// body returns the lines under the note: the links, or that they are
// loading, failed to load, or there are none.
func (s *Step) body(w int) []string {
	snap := s.snap.Load()
	switch {
	case !s.loaded && s.err != nil:
		return ui.ErrorLine(s.errs, s.errorText(), s.errorHint(), w)
	case !s.loaded:
		return []string{" " + s.spin.View() + " " + s.st.note.Render("Reading the links of "+s.self.String()+s.opts.icons.Ellipsis)}
	case snap == nil || len(snap.groups) == 0:
		return s.emptyLines(w)
	}
	return strings.Split(s.tree.View(), "\n")
}

// errorText and errorHint word the failure to read the links.
func (s *Step) errorText() string {
	text, _ := ui.ErrorText("read the links of "+s.self.String(), s.self.String(), s.voice)(s.err)
	return text
}

func (s *Step) errorHint() string {
	_, hint := ui.ErrorText("read the links of "+s.self.String(), s.self.String(), s.voice)(s.err)
	return hint
}

// emptyLines says there are no links, and what a link is.
func (s *Step) emptyLines(w int) []string {
	lines := []string{
		"Nothing links to " + s.self.String() + ", and it links to nothing.",
		"Links are #12, owner/repo#12, or a link to an issue or pull request.",
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, ui.Wrap(s.st.note.Render(l), w)...)
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }
