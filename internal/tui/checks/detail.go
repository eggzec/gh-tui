package checks

import (
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// openDetail shows what the app of the check r reported, or the commit
// status r.
func (s *Step) openDetail(r row) {
	s.mode, s.check = detailMode, r
	s.rendered = ""
	s.layout()
	s.detail.GotoTop()
}

// renderDetail renders the detail of the check shown at the width of the
// viewport, once per check and width, since markdown is slow to render.
func (s *Step) renderDetail() {
	w := s.detail.Width()
	if s.mode != detailMode || w <= 0 {
		return
	}
	key := s.check.key() + "\x00" + strconv.Itoa(w)
	if s.rendered == key {
		return
	}
	s.rendered = key
	s.detail.SetContent(s.markdown(s.detailSource(), w))
}

// detailSource is the markdown of the detail: the title, summary and text
// of a check, or the description of a status.
func (s *Step) detailSource() string {
	var parts []string
	switch r := s.check; {
	case r.check != nil:
		c := r.check
		if t := strings.TrimSpace(c.Title); t != "" {
			parts = append(parts, "**"+t+"**")
		}
		parts = append(parts, c.Summary, c.Text)
	case r.status != nil:
		parts = append(parts, r.status.Description)
	}
	parts = compact(parts)
	if len(parts) == 0 {
		text := "The check reported nothing more here."
		if k := s.keys.Open.Help().Key; k != "" && s.check.url() != "" {
			text += " " + k + " opens its page."
		}
		return text
	}
	return strings.Join(parts, "\n\n")
}

// compact drops the blank parts.
func compact(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

// markdown renders src at width with the thread's markdown style. The
// renderer is kept until the width or the theme changes.
func (s *Step) markdown(src string, width int) string {
	if s.md == nil || s.mdWidth != width {
		md, err := glamour.NewTermRenderer(
			glamour.WithStyles(s.theme.Thread().Markdown),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return src
		}
		s.md, s.mdWidth = md, width
	}
	out, err := s.md.Render(src)
	if err != nil {
		return src
	}
	return trimBlank(out)
}

// trimBlank drops the blank lines around s.
func trimBlank(s string) string {
	lines := strings.Split(s, "\n")
	blank := func(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// detailLines renders the detail, h lines of w cells: how the check stands
// over what it reported.
func (s *Step) detailLines(w, h int) []string {
	st, r := &s.st, s.check
	head := st.run.Glyphs[r.state()] + " " + st.run.Strong.Render(r.name())
	var right string
	if c := r.check; c != nil {
		right = st.run.Took(c.Status, c.Conclusion, c.StartedAt, c.CompletedAt, s.now())
	}
	lines := make([]string, 0, h)
	lines = append(lines, ui.Spread(head, right, w))
	for l := range strings.SplitSeq(s.detail.View(), "\n") {
		lines = append(lines, ui.Fit(l, w))
	}
	return ui.PadLines(lines, w, h)
}
