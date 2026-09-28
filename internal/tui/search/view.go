package search

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Sizes of the page. The query takes a framed line on top. From
// wideWidth columns the kinds go in a column of kindsWidth on the left, as
// on github.com; below it they go on a line above the results.
const (
	inputHeight = 3
	wideWidth   = 70
	kindsWidth  = 24
)

// wide reports whether the kinds have a column of their own.
func (s *Section) wide() bool { return s.width >= wideWidth }

// resultsSize is the room inside the frame of the results, below the line
// of kinds when the page is narrow.
func (s *Section) resultsSize() (width, height int) {
	w, h := s.width-2, s.height-inputHeight-2
	if s.wide() {
		w -= kindsWidth
	} else {
		h--
	}
	return max(w, 0), max(h, 0)
}

func (s *Section) layout() {
	// The input draws one cell more than its width, for the cursor.
	s.input.SetWidth(max(s.width-2-2-1-1, 1))
	for _, l := range s.hits {
		s.sizeList(&l.feed)
	}
	if s.code != nil {
		s.sizeList(&s.code.feed)
	}
	_, h := s.resultsSize()
	s.starts.resize(h)
}

func (s *Section) sizeList(f interface{ SetSize(width, height int) }) {
	f.SetSize(s.resultsSize())
}

// render renders the page.
func (s *Section) render() {
	if s.width <= 0 || s.height <= 0 {
		s.view = ""
		return
	}
	lines := make([]string, 0, s.height)
	lines = append(lines, s.frame(" "+s.st.accent.render("›")+" "+s.input.View(), "Search GitHub", s.area == inputArea, s.width, inputHeight)...)
	rw, rh := s.resultsSize()
	body := s.results(rw, rh)
	if !s.wide() {
		body = append([]string{s.kindsLine(rw)}, body...)
	}
	label := s.resultsLabel()
	right := s.frame(strings.Join(body, "\n"), label, s.area == resultsArea, rw+2, rh+2+boolInt(!s.wide()))
	if s.wide() {
		left := s.frame(strings.Join(s.kindsColumn(kindsWidth-2), "\n"), "Filter by", s.area == kindsArea, kindsWidth, len(right))
		for i := range right {
			right[i] = left[i] + right[i]
		}
	}
	lines = append(lines, right...)
	blank := strings.Repeat(" ", s.width)
	for len(lines) < s.height {
		lines = append(lines, blank)
	}
	s.view = strings.Join(lines[:s.height], "\n")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// frame draws body in a frame w by h cells, with label in its top edge,
// in the accent while the part it holds has the focus.
func (s *Section) frame(body, label string, focused bool, w, h int) []string {
	if w < 2 || h < 2 {
		return nil
	}
	edge, title := s.st.edge, s.st.title
	if focused && s.focused {
		edge, title = s.st.focusEdge, s.st.focusTitle
	}
	b := lipgloss.RoundedBorder()
	lines := make([]string, 0, h)
	label = truncate(label, max(w-5, 0))
	lw := ansi.StringWidth(label)
	lines = append(lines, edge.render(b.TopLeft+b.Top)+title.render(label)+edge.render(" "+strings.Repeat(b.Top, max(w-4-lw, 0))+b.TopRight))
	side, inner := edge.render(b.Left), w-2
	for range h - 2 {
		var l string
		l, body, _ = strings.Cut(body, "\n")
		lines = append(lines, side+fit(l, inner)+side)
	}
	return append(lines, edge.render(b.BottomLeft+strings.Repeat(b.Bottom, w-2)+b.BottomRight))
}

// countText is what the kinds show next to kind k: its count, or how to
// search code.
func (s *Section) countText(k core.SearchKind) string {
	if s.text == "" {
		return ""
	}
	if n, ok := s.counts[k]; ok {
		return count(n)
	}
	if k != core.SearchCode {
		// A search that failed counts nothing.
		if l := s.hits[s.kind]; l != nil && l.text == s.text && l.feed.Err() != nil {
			return ""
		}
		return "…"
	}
	if s.limited() {
		return "in " + s.wait()
	}
	return s.keys.Select.Help().Key + " search"
}

// kindsColumn renders the kinds, one a line, with their counts on the
// right.
func (s *Section) kindsColumn(w int) []string {
	st := &s.st
	lines := make([]string, 0, len(kinds))
	for _, k := range kinds {
		gutter := "  "
		name := st.text
		if k == s.kind {
			name = st.name
			gutter = st.blurred
			if s.focused && s.area == kindsArea {
				gutter, name = st.cursor, st.accent
			}
		}
		c := s.countText(k)
		lines = append(lines, spread(gutter+name.render(kindTitles[k]), st.muted.render(c)+" ", w))
	}
	return lines
}

// kindsLine renders the kinds on one line, for a narrow page.
func (s *Section) kindsLine(w int) string {
	st := &s.st
	var b strings.Builder
	b.WriteByte(' ')
	for i, k := range kinds {
		if i > 0 {
			st.subtle.write(&b, " · ")
		}
		title := kindTitles[k]
		if k == core.SearchPulls {
			title = "PRs"
		}
		name := st.muted
		if k == s.kind {
			name = st.focusTitle
		}
		name.write(&b, title)
		if c := s.countText(k); c != "" {
			b.WriteByte(' ')
			st.subtle.write(&b, c)
		}
	}
	return fit(ansi.Truncate(b.String(), w, "…"), w)
}

// resultsLabel names the results on view, and counts them.
func (s *Section) resultsLabel() string {
	if s.text == "" {
		return "Start"
	}
	label := kindTitles[s.kind]
	if _, ok := s.staleView(); ok {
		return label + " " + s.spin.View()
	}
	if n, ok := s.counts[s.kind]; ok {
		noun := " results"
		if n == 1 {
			noun = " result"
		}
		label += " · " + commas(n) + noun
	}
	return label
}

// wait is how long until code search resumes, such as "42s".
func (s *Section) wait() string {
	d := max(s.codeReset.Sub(s.now()), 0).Round(time.Second)
	if d < time.Minute {
		return strconv.Itoa(int(d/time.Second)) + "s"
	}
	return strconv.Itoa(int(d/time.Minute)) + "m" + strconv.Itoa(int(d%time.Minute/time.Second)) + "s"
}

// results renders the results on view in w by h cells, or what stands in
// for them.
func (s *Section) results(w, h int) []string {
	st := &s.st
	if s.text == "" {
		return s.startLines(w, h)
	}
	var f interface {
		View() string
		Err() error
	}
	if l, ok := s.visibleHits(); ok {
		f = feedOf(&l.feed)
	} else if l, ok := s.visibleCode(); ok {
		f = feedOf(&l.feed)
	}
	var err error
	if f != nil {
		err = f.Err()
	}
	switch {
	case errors.Is(err, core.ErrInvalidQuery):
		reason := "GitHub can't run this query."
		if iq, ok := errors.AsType[*core.InvalidQueryError](err); ok && iq.Reason != "" {
			reason = "GitHub can't run this query: " + cleanLine(iq.Reason)
		}
		return notice(w, st.fail.render(reason), st.subtle.render("Check the qualifiers, such as is:open, language:go or repo:owner/name."))
	case s.kind == core.SearchCode && s.limited():
		return notice(w, st.warning.render("Code search resumes in "+s.wait()+"."),
			st.subtle.render("GitHub allows 10 code searches a minute. Repositories, issues and pull requests still search as you type."))
	case f != nil:
		if lines, ok := s.staleView(); ok {
			return lines
		}
		return strings.Split(f.View(), "\n")
	case s.kind == core.SearchCode:
		return notice(w, st.text.render("Press "+s.keys.Select.Help().Key+" to search code for “"+s.text+"”."),
			st.subtle.render("Code search runs only when you ask, since GitHub allows 10 a minute."))
	}
	return nil
}

// feedOf lets results read a feed of either kind of result.
func feedOf[T any](f *feed.Model[T]) interface {
	View() string
	Err() error
} {
	return f
}

// notice renders lines of text, wrapped to w cells, with a blank line
// before each.
func notice(w int, lines ...string) []string {
	out := make([]string, 0, 2*len(lines))
	for _, l := range lines {
		out = append(out, "")
		for wrapped := range strings.SplitSeq(ansi.Wordwrap(l, max(w-2, 1), ""), "\n") {
			out = append(out, " "+wrapped)
		}
	}
	return out
}

// startLines renders the recent searches and the repositories offered
// before the user types.
func (s *Section) startLines(w, h int) []string {
	st, l := &s.st, &s.starts
	if len(l.items) == 0 {
		switch {
		case l.loading:
			return notice(w, st.muted.render("Loading your repositories…"))
		case l.err != nil:
			return append(s.startError(l.err, w), notice(w, st.subtle.render("Type to search GitHub."))...)
		}
		return notice(w, st.muted.render("Type to search repositories, issues, pull requests and code."),
			st.subtle.render("Use GitHub's qualifiers, such as is:open, author:@me or language:go."))
	}
	lines := make([]string, 0, h)
	focused := s.focused && s.area == resultsArea
	for i := l.top; i < len(l.items) && len(lines) < h; i++ {
		it := l.items[i]
		if it.header != "" {
			lines = append(lines, " "+st.muted.render(it.header))
			continue
		}
		gutter := "  "
		selected := l.sel < len(l.rows) && l.rows[l.sel] == i
		if selected {
			gutter = st.blurred
			if focused {
				gutter = st.cursor
			}
		}
		if it.query != "" {
			lines = append(lines, gutter+st.subtle.render("↺ ")+st.text.render(truncate(it.query, w-4)))
			continue
		}
		r := it.repo
		name := truncate(r.Ref.String(), w-2)
		line := gutter + st.name.render(name)
		if d := cleanLine(r.Description); d != "" && ansi.StringWidth(name)+4 < w-2 {
			line += "  " + st.muted.render(truncate(d, w-2-ansi.StringWidth(name)-2))
		}
		lines = append(lines, line)
	}
	return lines
}

// startError renders why the repositories offered before the user types
// failed to load, after a blank line, as notice does. They load once, and
// the open key opens a recent search or repository, so no hint names a
// key.
func (s *Section) startError(err error, w int) []string {
	v := s.voice
	v.Retry.SetEnabled(false)
	v.Open.SetEnabled(false)
	text, hint := ui.ErrorText("load your repositories", "", v)(err)
	lines := ui.ErrorLine(s.errs, text, hint, max(w-2, 1))
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return append([]string{""}, lines...)
}

// Widths of the right of a result row.
const (
	starsWidth    = 6
	commentsWidth = 5
)

// renderHit renders a repository, issue or pull request in two lines of
// width cells.
func (s *Section) renderHit(hit core.SearchHit, selected bool, width int) string {
	if hit.Kind == core.SearchRepos {
		return s.renderRepo(hit.Repo, selected, width)
	}
	st := &s.st
	is := &hit.Issue
	title := st.text
	if selected {
		title = st.name
	}
	ref := is.Repo.String() + "#" + strconv.Itoa(is.Number)
	right := ""
	if is.Comments > 0 {
		right = st.muted.render("◦ " + strconv.Itoa(is.Comments))
	}
	room := max(width-2-commentsWidth-1, 0)
	// The title matters more than where it is.
	ref = truncate(ref, min(ansi.StringWidth(ref), max(room/3, 12)))
	// Where it is and its title link to its page.
	head := s.stateGlyph(hit) + " " + s.links.Link(is.URL, st.muted.render(ref)+" "+
		title.render(truncate(cleanLine(is.Title), max(room-ansi.StringWidth(ref)-1, 0))))
	first := spread(head, right, width)

	parts := make([]string, 0, 4)
	if labels := s.labels(is.Labels); labels != "" {
		parts = append(parts, labels)
	}
	if is.Author.Login != "" {
		parts = append(parts, st.muted.render(is.Author.Login))
	}
	if !is.UpdatedAt.IsZero() {
		parts = append(parts, st.subtle.render("updated "+ui.AgoProse(is.UpdatedAt, s.now())))
	}
	second := "  " + strings.Join(parts, st.subtle.render(" · "))
	return first + "\n" + fit(ansi.Truncate(second, width, "…"), width)
}

// stateGlyph marks an issue or pull request by its state; the shapes
// differ as well as the colors.
func (s *Section) stateGlyph(hit core.SearchHit) string {
	state := ui.HitState(hit)
	return s.st.states[state].render(s.icons.State(state))
}

// langGlyph renders the glyph of r's language in its color.
func (s *Section) langGlyph(r core.Repo) string {
	k := r.Language + "\x00" + r.LanguageColor
	g, ok := s.langs[k]
	if !ok {
		g = s.theme.Language(r.Language, r.LanguageColor).Render(s.icons.Language(r.Language))
		s.langs[k] = g
	}
	return g
}

// labels renders the names of labels, each after a dot of its color.
func (s *Section) labels(labels []core.Label) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		dot, ok := s.dots[l.Color]
		if !ok {
			dot = "●"
			if isHex(l.Color) {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#" + l.Color)).Render("●")
			}
			s.dots[l.Color] = dot
		}
		parts = append(parts, dot+" "+s.st.muted.render(cleanLine(l.Name)))
	}
	return strings.Join(parts, " ")
}

func isHex(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// repoCols are the widths of the columns at the right of the first line
// of a repository, which line up from row to row. A narrow row shows the
// language by its glyph alone and the age in its short form, and the
// narrowest only the stars.
type repoCols struct {
	lang, age int
	short     bool
}

// repoLayouts are the columns of repository rows, widest first:
// "TypeScript" and "11mo ago", then a glyph and "11mo".
var repoLayouts = []repoCols{{lang: 12, age: 8}, {lang: 1, age: 4, short: true}, {}}

// minRepoName is the room the name of a repository keeps, before the
// columns give way.
const minRepoName = 20

func repoColumns(width int) repoCols {
	for _, c := range repoLayouts {
		if width-c.width()-1 >= minRepoName {
			return c
		}
	}
	return repoCols{}
}

// width is the width of the columns with the gaps between them.
func (c repoCols) width() int {
	w := starsWidth
	if c.lang > 0 {
		w += c.lang + 2
	}
	if c.age > 0 {
		w += c.age + 2
	}
	return w
}

// renderRepo renders a repository in two lines: its name and the glyphs
// of what it is, with its language, stars and age in columns at the
// right, then its description.
func (s *Section) renderRepo(r core.Repo, selected bool, width int) string {
	st := &s.st
	cols := repoColumns(width)
	cells := make([]string, 0, 3)
	if cols.lang > 0 {
		cells = append(cells, s.langCell(r, cols.lang))
	}
	cells = append(cells, padLeft(st.muted.render(s.icons.Star+" "+count(r.Stars)), starsWidth))
	if cols.age > 0 {
		age := ""
		switch {
		case r.UpdatedAt.IsZero():
		case cols.short:
			age = ui.Ago(r.UpdatedAt, s.now())
		default:
			age = ui.AgoProse(r.UpdatedAt, s.now())
		}
		cells = append(cells, padLeft(st.subtle.render(age), cols.age))
	}
	right := strings.Join(cells, "  ")

	flags := s.icons.Flags(r)
	room := width - ansi.StringWidth(right) - 1 - 2*len(flags)
	name := st.text
	if selected {
		name = st.name
	}
	var head strings.Builder
	head.WriteString(s.links.Link(s.repoURL(r), name.render(truncate(r.Ref.String(), max(room, 0)))))
	for _, f := range flags {
		head.WriteByte(' ')
		st.muted.write(&head, f)
	}
	first := spread(head.String(), right, width)
	second := ""
	if d := cleanLine(r.Description); d != "" {
		second = "  " + st.muted.render(truncate(d, width-2))
	}
	return first + "\n" + fit(second, width)
}

// langCell renders the language of r in width cells: its glyph and name,
// or its glyph alone in a cell of one.
func (s *Section) langCell(r core.Repo, width int) string {
	if r.Language == "" {
		return strings.Repeat(" ", width)
	}
	if width < 3 {
		return fit(s.langGlyph(r), width)
	}
	return fit(s.langGlyph(r)+" "+s.st.text.render(truncate(r.Language, width-2)), width)
}

// renderCode renders a file that matches: its repository and path, then a
// few lines of what matched, the matches in the accent.
func (s *Section) renderCode(hit core.CodeHit, selected bool, width int) string {
	st := &s.st
	path := st.text
	if selected {
		path = st.name
	}
	lines := make([]string, 0, codeHeight)
	// The repository and the file link to their pages.
	repo := s.links.Link(s.repoURL(core.Repo{Ref: hit.Repo}), st.muted.render(hit.Repo.String()))
	file := s.links.Link(hit.URL, path.render(ui.OneLine(hit.Path)))
	lines = append(lines, fit(ansi.Truncate(repo+st.subtle.render(" · ")+file, width, "…"), width))
	var frag core.Fragment
	if len(hit.Fragments) > 0 {
		frag = hit.Fragments[0]
	}
	for _, l := range fragmentView(frag, fragmentLines) {
		lines = append(lines, fit(ansi.Truncate("  "+s.highlight(l), width, "…"), width))
	}
	for len(lines) < codeHeight {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

// fragLine is a line of a fragment with the matches in it, as byte
// offsets into text.
type fragLine struct {
	text    string
	matches [][2]int
}

// fragmentView returns up to n lines of f, from the line of its first
// match.
func fragmentView(f core.Fragment, n int) []fragLine {
	var all []fragLine
	start := 0
	for start <= len(f.Text) {
		end := strings.IndexByte(f.Text[start:], '\n')
		if end < 0 {
			end = len(f.Text)
		} else {
			end += start
		}
		l := fragLine{text: f.Text[start:end]}
		for _, m := range f.Matches {
			lo, hi := max(m[0], start), min(m[1], end)
			if lo < hi {
				l.matches = append(l.matches, [2]int{lo - start, hi - start})
			}
		}
		all = append(all, l)
		start = end + 1
	}
	first := 0
	for i, l := range all {
		if len(l.matches) > 0 {
			first = i
			break
		}
	}
	all = all[first:]
	// Lines without text at the end show nothing.
	for len(all) > 0 && strings.TrimSpace(all[len(all)-1].text) == "" {
		all = all[:len(all)-1]
	}
	return all[:min(n, len(all))]
}

// highlight renders a line of code, its matches in the accent. Tabs become
// spaces and other control characters are dropped, so the layout holds.
func (s *Section) highlight(l fragLine) string {
	var b strings.Builder
	at := 0
	for _, m := range l.matches {
		if m[0] < at || m[1] > len(l.text) {
			continue
		}
		b.WriteString(s.st.muted.render(codeText(l.text[at:m[0]])))
		b.WriteString(s.st.match.render(codeText(l.text[m[0]:m[1]])))
		at = m[1]
	}
	b.WriteString(s.st.muted.render(codeText(l.text[at:])))
	return b.String()
}

func codeText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case termtext.Control(r), r == utf8.RuneError:
			// Invalid UTF-8 comes as RuneError, and a lone byte of it
			// may read as C1.
			return -1
		}
		return r
	}, s)
}
