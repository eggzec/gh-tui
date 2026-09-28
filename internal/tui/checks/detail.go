package checks

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
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

// markdown renders src at width with the thread's markdown style, indented
// by two cells with as much room on the right. The renderer is kept until
// the theme changes.
func (s *Step) markdown(src string, width int) string {
	if s.md == nil {
		s.md = markdown.New(s.theme.Thread(s.opts.icons).Markdown)
	}
	s.md.SetHint(s.openHint())
	return markdown.Indent(s.md.Render(src, markdown.Room(width, 4)), "  ")
}

// openHint is what the note that ends a detail cut short offers: the
// check's page, if it has one, where the rest shows.
func (s *Step) openHint() string {
	u := s.check.url()
	if u == "" {
		return ""
	}
	if p, err := url.Parse(u); err == nil && p.Host == "github.com" {
		return ui.OpenHint(s.keys.Open)
	}
	if k := s.keys.Open.Help().Key; k != "" && s.keys.Open.Enabled() {
		return k + " to open its page"
	}
	return ""
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
