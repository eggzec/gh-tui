package diff

import (
	"context"
	"errors"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// keepWindows is how many windows' height away from the window the tokens
// of a file are kept; those of a file farther off are dropped, and lexed
// again if it comes back.
const keepWindows = 3

// highlights holds the tokens of the files in view, which the copies of
// a model share, as they do the layout.
type highlights struct {
	// gen counts the states the tokens were made for; a result of an older
	// one is dropped.
	gen int
	// asked says that a file was handed to the lexer, so it is not again.
	asked map[int]bool
	// spans holds the spans of the files near the window.
	spans map[int]*fileSpans
}

// fileSpans holds the spans of the body rows of a file in one slice: those
// of row i are all[off[i]:off[i+1]], and a row with none shows plain.
type fileSpans struct {
	all []syntax.Span
	off []int32
}

func newFileSpans(rows [][]syntax.Span) *fileSpans {
	n := 0
	for _, r := range rows {
		n += len(r)
	}
	f := &fileSpans{all: make([]syntax.Span, 0, n), off: make([]int32, 0, len(rows)+1)}
	for _, r := range rows {
		f.off = append(f.off, int32(len(f.all)))
		f.all = append(f.all, r...)
	}
	f.off = append(f.off, int32(len(f.all)))
	return f
}

// row returns the spans of body row i, or nil.
func (f *fileSpans) row(i int) []syntax.Span {
	if f == nil || i < 0 || i+1 >= len(f.off) || f.off[i] == f.off[i+1] {
		return nil
	}
	return f.all[f.off[i]:f.off[i+1]:f.off[i+1]]
}

func newHighlights() *highlights {
	return &highlights{asked: map[int]bool{}, spans: map[int]*fileSpans{}}
}

// highlightMsg carries the tokens of file file back to the view with ID id,
// made for generation gen.
type highlightMsg struct {
	id    int
	gen   int
	file  int
	spans *fileSpans
	// busy says that a lexer that overran still ran, so the file was not
	// lexed and is asked for again.
	busy bool
}

// SetTabWidth sets the width of a tab stop. The default is
// [DefaultTabWidth]. The lines are lexed again, as the tokens of the old
// width no longer fit them.
func (m *Model) SetTabWidth(n int) {
	n = max(n, 1)
	if n == m.tabs {
		return
	}
	m.tabs = n
	m.hl.gen++
	clear(m.hl.asked)
	clear(m.hl.spans)
	m.rebuildSearch()
	m.dirty = true
}

// side is the lines of one side of a hunk, as the lexer reads them: the
// context and the deleted lines of the old side, the context and the
// added lines of the new one.
type side struct {
	lines []string
	rows  []int  // the body row of each line
	kinds []Kind // and its kind
}

// hunkSides is the two sides of a hunk.
type hunkSides struct{ old, new side }

// highlight returns the command that lexes each file in the window that
// has not been yet.
func (m *Model) highlight() tea.Cmd {
	n := m.layout.Len()
	if m.styles.Syntax == nil || m.highlightLimit <= 0 || m.top >= n || m.bodyHeight() <= 0 {
		return nil
	}
	f, ok := m.layout.FileAt(m.top)
	if !ok {
		return nil
	}
	end := min(m.top+m.bodyHeight(), n)
	var cmds []tea.Cmd
	for ; f < m.layout.Files(); f++ {
		if start, _ := m.layout.FileRow(f); start >= end {
			break
		}
		if m.hl.asked[f] || m.layout.Collapsed(f) {
			continue
		}
		m.hl.asked[f] = true
		if cmd := m.lex(f); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.dropFar(end)
	return tea.Batch(cmds...)
}

// dropFar drops the tokens of the files more than keepWindows windows from
// the window, which ends at row end.
func (m *Model) dropFar(end int) {
	reach := keepWindows * m.bodyHeight()
	for f := range m.hl.asked {
		start, _ := m.layout.FileRow(f)
		if start+m.layout.rowsOf(f) < m.top-reach || start > end+reach {
			delete(m.hl.asked, f)
			delete(m.hl.spans, f)
		}
	}
}

// lex returns the command that lexes file f, or nil if it has no text to.
func (m *Model) lex(f int) tea.Cmd {
	m.layout.ensure(f)
	s := &m.layout.files[f]
	if len(s.body) == 0 {
		return nil
	}
	var hunks []hunkSides
	size, last := 0, -2
	for i, r := range s.body {
		switch r.Kind {
		case KindContext, KindAdded, KindDeleted:
		case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw:
			continue
		}
		size += len(r.Text)
		if size > m.highlightLimit {
			return nil
		}
		if r.Hunk != last {
			hunks = append(hunks, hunkSides{})
			last = r.Hunk
		}
		h := &hunks[len(hunks)-1]
		line := termtext.Clean(r.Text, m.tabs)
		if r.Kind != KindAdded {
			h.old.add(line, i, r.Kind)
		}
		if r.Kind != KindDeleted {
			h.new.add(line, i, r.Kind)
		}
	}
	if len(hunks) == 0 {
		return nil
	}
	// A deleted file has only its old path to go by.
	path := s.file.Path
	if s.file.Status == StatusRemoved && s.file.OldPath != "" {
		path = s.file.OldPath
	}
	parent, id, gen, rows := m.parent, m.id, m.hl.gen, len(s.body)
	return func() tea.Msg {
		spans, busy := lexHunks(parent, path, hunks, rows)
		if busy {
			return highlightMsg{id: id, gen: gen, file: f, busy: true}
		}
		if spans == nil {
			return nil
		}
		return highlightMsg{id: id, gen: gen, file: f, spans: newFileSpans(spans)}
	}
}

func (s *side) add(line string, row int, k Kind) {
	s.lines = append(s.lines, line)
	s.rows = append(s.rows, row)
	s.kinds = append(s.kinds, k)
}

// lexHunks lexes each side of each hunk as the file at path, and returns
// the spans of each of rows body rows, or nil if the file shows plain. It
// says busy if a lexer that overran still ran, so that it can be tried
// again.
// Chroma can take long even to pick a lexer, so it is called only here, in
// the command.
func lexHunks(parent context.Context, path string, hunks []hunkSides, rows int) (spans [][]syntax.Span, busy bool) {
	var guess strings.Builder
	for i := range hunks {
		for _, l := range hunks[i].new.lines {
			guess.WriteString(l)
			guess.WriteByte('\n')
		}
		if guess.Len() > 1<<10 {
			break
		}
	}
	lexer := syntax.Ready(syntax.File(path, guess.String()))
	if lexer == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(parent, syntax.LexLimit)
	defer cancel()
	out := make([][]syntax.Span, rows)
	found := false
	for i := range hunks {
		h := &hunks[i]
		for _, sd := range []struct {
			side side
			old  bool
		}{{h.old, true}, {h.new, false}} {
			if len(sd.side.lines) == 0 {
				continue
			}
			toks, err := syntax.Head(ctx, lexer, syntax.Lexable(sd.side.lines), syntax.LexLimit)
			if ctx.Err() != nil {
				return keep(out, found), false
			}
			if errors.Is(err, syntax.ErrBusy) {
				return nil, true
			}
			if err != nil || toks == nil {
				continue
			}
			spans := syntax.Spans(toks, len(sd.side.lines))
			for i, row := range sd.side.rows {
				if i < len(spans) && (!sd.old || sd.side.kinds[i] == KindDeleted) {
					out[row] = spans[i]
					found = found || len(spans[i]) > 0
				}
			}
		}
	}
	return keep(out, found), false
}

func keep(out [][]syntax.Span, found bool) [][]syntax.Span {
	if !found {
		return nil
	}
	return out
}

// rowSpans returns the spans of row, which is the row number index of the
// layout, or nil if it shows plain.
func (m Model) rowSpans(row Row, index int) []syntax.Span {
	if len(m.hl.spans) == 0 || row.File < 0 {
		return nil
	}
	spans := m.hl.spans[row.File]
	if spans == nil {
		return nil
	}
	start, _ := m.layout.FileRow(row.File)
	return spans.row(index - start - 1)
}

// tokenStyles holds, for a kind of line, how each token type looks:
// what the style of the line looks like with the colors of the syntax
// style. Styling a token by concatenating its escape sequences is much
// cheaper than rendering it with lipgloss.
type tokenStyles struct {
	text   sgr
	tokens map[chroma.TokenType]sgr
}

func newTokenStyles(base lipgloss.Style, syn *chroma.Style) tokenStyles {
	t := tokenStyles{text: wrapOf(base), tokens: map[chroma.TokenType]sgr{}}
	if syn == nil {
		return t
	}
	plain := syn.Get(chroma.Text)
	for typ := range chroma.StandardTypes {
		e := syn.Get(typ)
		st := base
		if e.Colour.IsSet() && e.Colour != plain.Colour {
			st = st.Foreground(color.RGBA{R: e.Colour.Red(), G: e.Colour.Green(), B: e.Colour.Blue(), A: 0xff})
		}
		st = st.Bold(e.Bold == chroma.Yes).Italic(e.Italic == chroma.Yes).Underline(e.Underline == chroma.Yes)
		if p := wrapOf(st); p != t.text {
			t.tokens[typ] = p
		}
	}
	return t
}

func (t tokenStyles) token(typ chroma.TokenType) sgr {
	if p, ok := t.tokens[typ]; ok {
		return p
	}
	return t.text
}

// coloured writes s, a line of text of the kind t styles, in the colors of
// its spans; the text after the last span in the style of the line. Runs
// that look the same share one pair of escape sequences.
func (t tokenStyles) coloured(s string, spans []syntax.Span) string {
	var b strings.Builder
	b.Grow(len(s) + 24*len(spans))
	pos := 0
	var cur sgr
	for _, sp := range spans {
		end := min(sp.End, len(s))
		if end <= pos {
			continue
		}
		if p := t.token(sp.Type); pos == 0 || p != cur {
			if pos > 0 {
				b.WriteString(cur.suf)
			}
			b.WriteString(p.pre)
			cur = p
		}
		b.WriteString(s[pos:end])
		pos = end
	}
	if pos > 0 {
		b.WriteString(cur.suf)
	}
	if pos < len(s) {
		b.WriteString(t.text.on(s[pos:]))
	}
	return b.String()
}
