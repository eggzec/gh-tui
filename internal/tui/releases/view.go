package releases

import (
	"cmp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// gutter indents the header and the files, as the other modals do.
const gutter = "  "

// styles are the styles of the header and the files, built once per
// theme.
type styles struct {
	title, text, muted, subtle, warning, rule, fail lipgloss.Style
	sep                                             string
}

func newStyles(t ui.Theme) styles {
	return styles{
		title:   t.Title,
		text:    t.Text,
		muted:   t.Muted,
		subtle:  t.Subtle,
		warning: t.Warning,
		rule:    t.Subtle,
		fail:    t.Error,
		sep:     t.Subtle.Render(" · "),
	}
}

// View implements ui.Modal. Until the release is read, a failed read says
// why in place of the thread.
func (m *Modal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.failed() {
		return m.errorView()
	}
	return m.thread.View()
}

// errorView says why the release couldn't be read, and how to go on, in
// lines that fill the size.
func (m *Modal) errorView() string {
	st := &m.st
	text := []string{
		gutter + st.fail.Render("Couldn't load the release: "+m.err.Error()),
		gutter + st.muted.Render(hint(m.keys.Refresh.Help().Key, "retry", m.keys.Open.Help().Key, "open it on GitHub")),
	}
	lines := make([]string, m.height)
	for i := range lines {
		var l string
		if i > 0 && i-1 < len(text) {
			l = text[i-1]
		}
		lines[i] = fit(l, m.width)
	}
	return strings.Join(lines, "\n")
}

// hint names the keys that do what, such as "Press r to retry, or o to
// open it on GitHub.", leaving out the unbound ones.
func hint(retry, retryDo, open, openDo string) string {
	var parts []string
	if retry != "" {
		parts = append(parts, retry+" to "+retryDo)
	}
	if open != "" {
		parts = append(parts, open+" to "+openDo)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Press " + strings.Join(parts, ", or ") + "."
}

// fit truncates or pads s to width cells.
func fit(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	if n := ansi.StringWidth(s); n < width {
		s += strings.Repeat(" ", width-n)
	}
	return s
}

// releaseName is what a release is called: its name, or else its tag.
func releaseName(r core.Release) string {
	if name := strings.TrimSpace(r.Name); name != "" {
		return name
	}
	return r.Tag
}

// header renders the head of the release at width: its name, its tag,
// who published it and when, and its files, over a rule.
func (m *Modal) header(width int) string {
	r, st := &m.rel, &m.st
	inner := max(width-len(gutter), 1)
	var lines []string
	for l := range strings.SplitSeq(ansi.Wrap(releaseName(*r), inner, ""), "\n") {
		lines = append(lines, gutter+st.title.Render(l))
	}
	lines = append(lines, "")

	parts := []string{st.text.Render(r.Tag)}
	switch {
	case r.Draft:
		parts = append(parts, st.muted.Render("draft"))
	case r.Prerelease:
		parts = append(parts, st.warning.Render("pre-release"))
	}
	when, verb := r.PublishedAt, " published "
	if when.IsZero() {
		when, verb = r.CreatedAt, " drafted "
	}
	by := st.text.Render(cmp.Or(r.Author.Login, "someone")) + st.muted.Render(verb+ui.AgoProse(when, m.now()))
	if !when.IsZero() {
		by += st.subtle.Render(" (" + when.In(m.loc).Format("2006-01-02") + ")")
	}
	parts = append(parts, by)
	lines = append(lines, gutter+strings.Join(parts, st.sep))

	if n := len(r.Assets); n > 0 {
		downloads := 0
		for _, a := range r.Assets {
			downloads += a.Downloads
		}
		lines = append(lines, gutter+st.muted.Render(plural(n, "file"))+st.sep+st.muted.Render(plural(downloads, "download")))
	}
	lines = append(lines, gutter+st.rule.Render(strings.Repeat("─", inner)))
	return strings.Join(lines, "\n")
}

// renderAsset renders a file of the release on a line of width cells: its
// name, then its size and downloads at the right.
func (m *Modal) renderAsset(a core.ReleaseAsset, width int) string {
	st := &m.st
	size := ui.Size(a.Size)
	downloads := "↓ " + thousands(a.Downloads)
	// The numbers line up in columns wide enough for most files.
	right := strings.Repeat(" ", max(6-len(size), 0)) + size + "  " + strings.Repeat(" ", max(9-ansi.StringWidth(downloads), 0)) + downloads
	room := max(width-len(gutter)-ansi.StringWidth(right)-2, 1)
	name := ansi.Truncate(a.Name, room, "…")
	pad := max(width-len(gutter)-ansi.StringWidth(name)-ansi.StringWidth(right), 1)
	return gutter + st.text.Render(name) + strings.Repeat(" ", pad) + st.muted.Render(right[:len(right)-len(downloads)]) + st.subtle.Render(downloads)
}

// thousands writes n with commas between its thousands, as GitHub does.
func thousands(n int) string {
	s := strconv.Itoa(max(n, 0))
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func plural(n int, noun string) string {
	s := thousands(n) + " " + noun
	if n != 1 {
		s += "s"
	}
	return s
}
