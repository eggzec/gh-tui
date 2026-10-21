package search

import (
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Sizes of the page. The query takes a framed line on top, and the
// results a frame below it, whose first line holds the kinds as tabs.
const inputHeight = 3

// resultsSize is the room inside the frame of the results, below the line
// of kinds.
func (s *Section) resultsSize() (width, height int) {
	return max(s.listOuter()-2, 0), max(s.height-inputHeight-3, 0)
}

func (s *Section) layout() {
	// The input draws one cell more than its width, for the cursor.
	s.input.SetWidth(max(s.width-2-2-1-1, 1))
	// The hits take a line of titles above them.
	_, h := s.resultsSize()
	for _, l := range s.hits {
		s.sizeHits(l)
	}
	if s.code != nil {
		s.sizeList(&s.code.feed)
	}
	s.starts.resize(h)
}

// sizeHits sizes the feed of l, a line shorter than the room of the results,
// which the titles of the columns take.
func (s *Section) sizeHits(l *hitList) {
	w, h := s.resultsSize()
	l.feed.SetSize(w, max(h-1, 0))
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
	lines = append(lines, s.frame(" "+s.st.accent.render(s.icons.Crumb)+" "+s.input.View(), "Search GitHub", s.area == inputArea, s.width, inputHeight)...)
	lines = append(lines, s.panes()...)
	blank := strings.Repeat(" ", s.width)
	for len(lines) < s.height {
		lines = append(lines, blank)
	}
	s.view = strings.Join(lines[:s.height], "\n")
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
	b := s.icons.Border
	lines := make([]string, 0, h)
	label = s.truncate(label, max(w-5, 0))
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

// codeAsk asks the user to search code for the text with the key that
// searches, or to bind one while the config leaves select without keys.
func (s *Section) codeAsk() string {
	what := s.icons.OpenQuote + s.text + s.icons.CloseQuote
	if k := s.keys.Select.Help().Key; k != "" {
		return "Press " + s.icons.Key(k) + " to search code for " + what + "."
	}
	return "Bind a key to select in the config to search code for " + what + "."
}

// typeAsk asks the user to start typing with the key that does, or to bind
// one while the config leaves it without keys. what finishes the sentence
// "Press it to". While the query types, it says to type.
func (s *Section) typeAsk(what string) string {
	if s.typing {
		return "Type to " + what
	}
	if k := s.keys.Insert.Help().Key; k != "" {
		return "Press " + s.icons.Key(k) + " to " + what
	}
	return "Bind a key to insert in the config to " + what
}

// kindStrip is the strip of kinds on the first line of the results: the
// digit that shows each kind and its title, the kind on view in brackets.
// It lists the kinds it is made with, one digit each.
type kindStrip struct {
	kinds []core.SearchKind
	// keys are the keys of the kinds, by position; a kind with none shows
	// no key.
	keys []string
}

// view renders the strip for the kind cur in w cells, with its titles
// as long as they fit, else with "PRs" for pull requests, else as the keys
// alone.
func (t kindStrip) view(cur core.SearchKind, on, off ownerui.Paint, w int) string {
	full := make([]string, len(t.kinds))
	short := make([]string, len(t.kinds))
	pos := -1
	for i, k := range t.kinds {
		full[i], short[i] = kindTitles[k], kindTitles[k]
		if k == core.SearchPulls {
			short[i] = "PRs"
		}
		if k == cur {
			pos = i
		}
	}
	strip, _ := ownerui.Foot(t.keys, [][]string{full, short}, pos, on, off, max(w-1, 0))
	return " " + strip
}

// kindsLine renders the strip of the kinds in w cells.
func (s *Section) kindsLine(w int) string {
	keys := make([]string, len(kinds))
	for i := range keys {
		if b := s.keys.Kinds[i]; b.Enabled() {
			keys[i] = b.Help().Key
		}
	}
	line := kindStrip{kinds: kinds, keys: keys}.view(s.kind, s.st.footOn, s.st.footOff, w)
	return fit(termtext.Truncate(line, w, s.icons.Ellipsis), w)
}

// countLabel is the count of the results on view for the right of the
// top edge of the results, such as "1,234 results": only the kinds the
// search API counts have one.
func (s *Section) countLabel() string {
	if s.text == "" || s.kind == core.SearchCode {
		return ""
	}
	if _, ok := s.staleView(); ok {
		return ""
	}
	n, ok := s.counts[s.kind]
	if !ok {
		return ""
	}
	if n == 1 {
		return commas(n) + " result"
	}
	return commas(n) + " results"
}

// resultsLabel names the results on view.
func (s *Section) resultsLabel() string {
	if s.text == "" {
		return "Start"
	}
	label := kindTitles[s.kind]
	if _, ok := s.staleView(); ok {
		return label + " " + s.spin.View()
	}
	if chip := s.grouping.Chip(s.icons.Separator); s.kind == core.SearchRepos && chip != "" {
		label += s.icons.Separator + chip
	}
	return label
}

// wait is how long until code search resumes, such as "42s" or "1m 5s".
func (s *Section) wait() string {
	return ui.Duration(s.codeReset.Sub(s.now()))
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
			st.subtle.render("GitHub allows 10 code searches a minute. Repositories, issues and pull requests still search."))
	case f != nil:
		if lines, ok := s.staleView(); ok {
			// The titles stay in front of the dimmed rows, so nothing
			// moves when the new results arrive.
			if l, ok := s.visibleHits(); ok {
				return append([]string{s.header(l, w)}, lines...)
			}
			return lines
		}
		lines := strings.Split(f.View(), "\n")
		if l, ok := s.visibleHits(); ok {
			return append([]string{s.header(l, w)}, lines...)
		}
		return lines
	case s.kind == core.SearchCode:
		return notice(w, st.text.render(s.codeAsk()),
			st.subtle.render("Code search runs only when you ask, since GitHub allows 10 a minute."))
	}
	return nil
}

// header renders the titles of the columns of the hits of l over its rows,
// w cells wide, laid out as renderRepo and renderHit lay out their rows: the
// leading column of the other kinds and the widths after Item mirror
// renderHit, so change them together.
func (s *Section) header(l *hitList, w int) string {
	gutter := l.feed.Gutter()
	rw := max(w-gutter, 0)
	var cols []ui.Column
	if l.kind == core.SearchRepos {
		c := repoColumns(rw, s.dates.Width())
		cols = []ui.Column{
			{Title: "Name", Width: rw - c.width() - 2},
			{Title: "Language", Width: c.lang},
			{Title: s.icons.Star, Width: starsWidth, Right: true},
			{Title: updatedTitle, Width: c.age},
		}
	} else {
		cols = []ui.Column{
			{Width: 1},
			{Title: "Item", Width: max(rw-1-commentsWidth-2, 0)},
			{Title: termtext.Cells(s.icons.Comment, 1), Width: commentsWidth, Right: true},
		}
	}
	line := strings.Repeat(" ", gutter) + ui.Header(cols, 2, s.icons.Ellipsis)
	return fit(s.st.subtle.render(line), w)
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
			return notice(w, st.muted.render("Loading your repositories"+s.icons.Ellipsis))
		case l.err != nil:
			return append(s.startError(l.err, w), notice(w, st.subtle.render(s.typeAsk("search GitHub.")))...)
		}
		return notice(w, st.muted.render(s.typeAsk("search repositories, issues, pull requests and code.")),
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
			lines = append(lines, gutter+st.subtle.render(termtext.Cells(s.icons.Recent, 1)+" ")+st.text.render(s.truncate(it.query, w-4)))
			continue
		}
		r := it.repo
		name := s.truncate(r.Ref.String(), w-2)
		line := gutter + st.name.render(name)
		if d := cleanLine(r.Description); d != "" && ansi.StringWidth(name)+4 < w-2 {
			line += "  " + st.muted.render(termtext.TruncateMiddle(d, max(w-2-ansi.StringWidth(name)-2, 0), s.icons.Ellipsis))
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

// wideRow is the width from which an issue or pull request row names its
// repository; a narrower row keeps the number alone.
const wideRow = 80

// renderHit renders a repository, issue or pull request in one line of
// width cells.
func (s *Section) renderHit(hit core.SearchHit, selected bool, width int) string {
	if hit.Kind == core.SearchRepos {
		return s.renderRepo(hit.Repo, selected, width)
	}
	st := &s.st.look[ui.Look(selected, s.focused && s.area == resultsArea)]
	is := &hit.Issue
	repo, num := is.Repo.String(), "#"+strconv.Itoa(is.Number)
	right := ""
	if is.Comments > 0 {
		right = st.muted.render(termtext.Cells(s.icons.Comment, 1) + " " + strconv.Itoa(is.Comments))
	}
	room := max(width-2-commentsWidth-1, 0)
	ref := num
	if width >= wideRow {
		// The title matters more than where it is, and the number more than
		// the repository, which is cut first.
		refWidth := min(ansi.StringWidth(repo)+len(num), max(room/3, 12))
		ref = s.truncate(repo, max(refWidth-len(num), 1)) + num
	}
	// Where it is and its title link to its page.
	head := st.stateGlyph(s, hit) + " " + s.links.Link(is.URL, st.muted.render(ref)+" "+
		st.title.render(s.truncate(cleanLine(is.Title), max(room-ansi.StringWidth(ref)-1, 0))))
	return s.spread(head, right, width)
}

// stateGlyph marks an issue or pull request by its state; the shapes
// differ as well as the colors.
func (c *cells) stateGlyph(s *Section, hit core.SearchHit) string {
	state := ui.HitState(hit)
	return c.states[state].render(s.icons.State(state))
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

// repoCols are the widths of the columns at the right of the first line
// of a repository, which line up from row to row. A narrow row shows the
// language by its glyph alone, and the narrowest only the stars.
type repoCols struct {
	lang, age int
}

// minRepoName is the room the name of a repository keeps, before the
// columns give way.
const minRepoName = 20

// updatedTitle heads the column of the dates of repositories.
const updatedTitle = "Updated"

// repoColumns returns the columns of repository rows of width cells,
// with dates of age cells at most: the first that fits of "TypeScript"
// and the date, a glyph and the date, and neither.
func repoColumns(width, age int) repoCols {
	// The column is as wide as its title, so the title shows whole.
	age = max(age, len(updatedTitle))
	for _, c := range []repoCols{{lang: 12, age: age}, {lang: 1, age: age}, {}} {
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

// renderRepo renders a repository in one line: the repository glyph, the
// glyphs of what it is and its name, with its language, stars and age in
// columns at the right.
func (s *Section) renderRepo(r core.Repo, selected bool, width int) string {
	st := &s.st.look[ui.Look(selected, s.focused && s.area == resultsArea)]
	cols := repoColumns(width, s.dates.Width())
	right := make([]string, 0, 3)
	if cols.lang > 0 {
		right = append(right, s.langCell(r, cols.lang, st.text))
	}
	right = append(right, padLeft(st.muted.render(s.icons.Star+" "+count(r.Stars)), starsWidth))
	if cols.age > 0 {
		age := ""
		if !r.UpdatedAt.IsZero() {
			age = s.dates.Short(r.UpdatedAt, s.now())
		}
		right = append(right, fit(st.subtle.render(age), cols.age))
	}
	cellsRight := strings.Join(right, "  ")

	flags := s.icons.Flags(r)
	room := width - ansi.StringWidth(cellsRight) - 1 - 2*len(flags) - 2
	var head strings.Builder
	st.subtle.write(&head, termtext.Cells(s.icons.Repo, 1))
	head.WriteByte(' ')
	for _, f := range flags {
		st.muted.write(&head, f)
		head.WriteByte(' ')
	}
	head.WriteString(s.links.Link(s.repoURL(r), st.title.render(s.truncate(r.Ref.String(), max(room, 0)))))
	return s.spread(head.String(), cellsRight, width)
}

// langCell renders the language of r in width cells: its glyph and name,
// or its glyph alone in a cell of one.
func (s *Section) langCell(r core.Repo, width int, text paint) string {
	if r.Language == "" {
		return strings.Repeat(" ", width)
	}
	if width < 3 {
		return fit(s.langGlyph(r), width)
	}
	return fit(s.langGlyph(r)+" "+text.render(s.truncate(r.Language, width-2)), width)
}

// renderCode renders a file that matches in one line: its icon, its path
// and its repository.
func (s *Section) renderCode(hit core.CodeHit, selected bool, width int) string {
	st := &s.st.look[ui.Look(selected, s.focused && s.area == resultsArea)]
	icon := ""
	if fi := s.icons.Entry(core.TreeEntry{Path: hit.Path, Name: path.Base(hit.Path)}, false); fi.Glyph != "" {
		icon = s.theme.FileIcon(fi).Render(termtext.Cells(fi.Glyph, 1)) + " "
	}
	// The repository and the file link to their pages.
	repo := s.links.Link(s.repoURL(core.Repo{Ref: hit.Repo}), st.muted.render(hit.Repo.String()))
	file := s.links.Link(hit.URL, st.title.render(ui.OneLine(hit.Path)))
	return fit(termtext.Truncate(icon+file+st.subtle.render(s.icons.Separator)+repo, width, s.icons.Ellipsis), width)
}

// fragLine is a line of a fragment with the matches in it, as byte
// offsets into text.
type fragLine struct {
	text    string
	matches [][2]int
}

// fragmentView returns up to n lines of f that have text, from the line
// of its first match.
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
	// Lines without text show nothing, so they don't take the few rows
	// a result has. The kept lines are written over all in place, from
	// first: shown starts empty there and never passes the line read.
	shown := all[first:first]
	for _, l := range all[first:] {
		if strings.TrimSpace(l.text) != "" {
			shown = append(shown, l)
		}
	}
	return shown[:min(n, len(shown))]
}
