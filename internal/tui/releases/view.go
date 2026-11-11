package releases

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/desc"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// gutter indents the error lines.
const gutter = "  "

// View implements ui.Modal: the header over the panes, or why the release
// couldn't be read.
func (m *Modal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.failed() {
		return m.errorView()
	}
	l := m.paneLayout()
	if l.Height <= 0 {
		return ui.Seal("", m.width)
	}
	lines := strings.Split(m.drawPanes(l), "\n")
	if m.loaded {
		lines = append(m.headerLines(), lines...)
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	blank := ui.Seal("", m.width)
	for len(lines) < m.height {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

// errorView says why the release couldn't be read, and how to go on, in
// lines that fill the size.
func (m *Modal) errorView() string {
	v := m.voice
	v.Retry = m.keys.Refresh
	subject := ""
	if errors.Is(m.err, core.ErrForbidden) {
		subject = m.repo.String()
	}
	text, hint := ui.ErrorText("load the release", subject, v)(m.err)
	errs := ui.ErrorLine(m.errs, text, hint, max(m.width-len(gutter), 1))
	lines := make([]string, m.height)
	for i := range lines {
		var l string
		if i > 0 && i-1 < len(errs) {
			l = gutter + errs[i-1]
		}
		lines[i] = termtext.Fit(l, m.width, m.icons.Ellipsis)
	}
	return strings.Join(lines, "\n")
}

// header is the lines over the panes, as one string.
func (m *Modal) header() string {
	return strings.Join(m.headerLines(), "\n")
}

// headerRows is the height of the header: its lines at the modal's width.
func (m *Modal) headerRows() int { return len(m.headerLines()) }

// releaseDate is the day the release was published, or else drafted; the
// top edge of the frame shows it.
func releaseDate(r core.Release) string {
	when := r.PublishedAt
	if when.IsZero() {
		when = r.CreatedAt
	}
	if when.IsZero() {
		return ""
	}
	return when.Local().Format("2006-01-02")
}

// headerLines tells of the release in as many lines as it takes at the
// width: author, tag, target, the files and their downloads, and the page.
// The state and the date are in the top edge of the frame.
func (m *Modal) headerLines() []string {
	r, t := &m.rel, &m.theme
	attr := func(label, value string) string {
		return t.Subtle.Render(label+" ") + t.Text.Render(value)
	}
	parts := []string{attr("by", cmp.Or(ui.OneLine(r.Author.Login), "someone"))}
	if r.Tag != "" {
		parts = append(parts, attr("tag", ui.OneLine(r.Tag)))
	}
	if r.Target != "" {
		parts = append(parts, attr("target", ui.OneLine(r.Target)))
	}
	downloads := 0
	for _, a := range r.Assets {
		downloads += a.Downloads
	}
	parts = append(parts, attr("files", thousands(len(r.Assets))), attr("downloads", thousands(downloads)))
	sep := t.Subtle.Render(" " + m.icons.Separator + " ")
	sepW := lipgloss.Width(sep)
	var lines []string
	line, lineW := "", 0
	for _, p := range parts {
		w := lipgloss.Width(p)
		if line != "" && lineW+sepW+w > m.width-1 {
			lines = append(lines, line)
			line, lineW = "", 0
		}
		if line != "" {
			line += sep
			lineW += sepW
		}
		line += p
		lineW += w
	}
	lines = append(lines, line)
	if r.URL != "" {
		lines = append(lines, termtext.Link(r.URL, t.Subtle.Render("url ")+t.Text.Render(ui.OneLine(r.URL))))
	}
	for i, l := range lines {
		lines[i] = ui.Seal(termtext.Fit(" "+l, m.width, m.icons.Ellipsis), m.width)
	}
	return lines
}

// releaseName is what a release is called: its name, or else its tag.
func releaseName(r core.Release) string {
	if name := strings.TrimSpace(ui.OneLine(r.Name)); name != "" {
		return name
	}
	return ui.OneLine(r.Tag)
}

// buildNotes renders the notes into the tree again when the text, the width
// or the look changed.
func (m *Modal) buildNotes(width int) {
	k := notesKey{text: m.rel.Body, width: width, look: m.look}
	if m.notesMade && m.notesBuilt == k {
		return
	}
	set := m.icons.Set()
	if set == "" {
		set = config.IconsNerd
	}
	var rows []treeview.Node
	if strings.TrimSpace(m.rel.Body) != "" {
		rows = desc.Rows(m.rel.Body, desc.Options{Width: width, Styles: desc.NewStyles(m.theme, m.icons, set), MD: m.md})
	}
	m.notes.SetStyles(m.theme.Tree(m.icons))
	m.notes.SetRows(rows)
	m.notesBuilt, m.notesMade = k, true
}

// notesKey is what the rows of the notes depend on.
type notesKey struct {
	text  string
	width int
	look  int
}

// buildFiles makes the rows of the Files pane: the name, then the size and
// the downloads at the right, in columns that line up.
func (m *Modal) buildFiles() {
	rows := make([]treeview.Node, 0, len(m.rel.Assets))
	sizeW, downW := 0, 0
	for _, a := range m.rel.Assets {
		sizeW = max(sizeW, len(ui.Size(a.Size)))
		downW = max(downW, len(thousands(a.Downloads)))
	}
	for _, a := range m.rel.Assets {
		right := fmt.Sprintf("%*s  %s %*s", sizeW, ui.Size(a.Size), m.icons.Down, downW, thousands(a.Downloads))
		rows = append(rows, treeview.Node{ID: strconv.FormatInt(a.ID, 10), Name: ui.OneLine(a.Name), Right: right, Value: a})
	}
	m.files.SetStyles(m.theme.Tree(m.icons))
	m.files.SetEmpty("No assets.", "")
	m.files.SetRows(rows)
}

// thousands writes n with commas between its thousands, as GitHub does.
func thousands(n int) string {
	s := strconv.Itoa(max(n, 0))
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
