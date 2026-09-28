package pager

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// binarySniff is how many leading bytes are checked for a NUL, the way git
// tells binary files apart.
const binarySniff = 8000

// SetContent shows text, named name, from its first line. The name picks
// the syntax, usually by its file extension, or else a script's shebang
// line does; only common languages are highlighted. The returned command
// highlights the text in the background, and the pager shows it plain
// until then. Text with a NUL byte is taken for binary and not shown.
func (m *Model) SetContent(name, text string) tea.Cmd {
	return m.setContent(name, text, func(full string) chroma.Lexer { return lexerFor(name, full) })
}

// SetContentSyntax shows text as SetContent does, but highlights it as
// lang, a lexer's name or alias such as "diff", rather than by the name of
// the content. An unknown lang shows the text plain.
func (m *Model) SetContentSyntax(name, lang, text string) tea.Cmd {
	return m.setContent(name, text, func(string) chroma.Lexer { return lexerNamed(lang) })
}

// setContent shows text, and returns the command that highlights it with
// the lexer lexerOf picks. Chroma can take long even to pick one, so it is
// called only in the command.
func (m *Model) setContent(name, text string, lexerOf func(full string) chroma.Lexer) tea.Cmd {
	m.reset(name, stateReady, nil)
	if strings.IndexByte(text[:min(len(text), binarySniff)], 0) >= 0 {
		m.state = stateBinary
		return nil
	}
	full := strings.TrimSuffix(termtext.Clean(text, m.tabWidth), "\n")
	if full != "" {
		m.lines = strings.Split(full, "\n")
	}
	m.size = len(full)
	m.clamp()
	if full == "" || len(full) > m.highlightLimit {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	id, gen, lines := m.id, m.gen, m.lines
	return func() tea.Msg {
		defer cancel()
		lexer := lexerOf(full)
		if lexer == nil {
			return nil
		}
		toks, err := syntax.Head(ctx, lexer, lexable(lines), lexLimit)
		if err != nil || toks == nil || ctx.Err() != nil {
			// The plain text stays; a failed highlight is not worth a
			// message.
			return nil
		}
		return highlightMsg{id: id, gen: gen, spans: spansOf(toks, len(lines))}
	}
}

// SetLoading shows a spinner while the content named name is on its way.
// The returned command starts the spinner, which stops once the content or
// an error is set.
func (m *Model) SetLoading(name string) tea.Cmd {
	m.reset(name, stateLoading, nil)
	return m.spin.Tick
}

// SetError shows that the content named name failed to load with err.
func (m *Model) SetError(name string, err error) {
	m.reset(name, stateFailed, err)
}

// SetMessage shows text in place of the content named name, for example
// why the parent doesn't show a file: that it is too large, or where a link
// points. Text longer than the width wraps.
func (m *Model) SetMessage(name, text string) {
	m.reset(name, stateMessage, nil)
	// One line without escape sequences, so it can't break the layout.
	m.note = strings.Join(strings.Fields(termtext.OneLine(text)), " ")
}

// reset forgets the content and stops its highlighter.
func (m *Model) reset(name string, s state, err error) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
	m.name, m.state, m.err, m.note, m.flash = name, s, err, "", ""
	m.errText, m.errHint = "", ""
	if s == stateFailed {
		m.errText, m.errHint = m.errorWords()
	}
	m.renderName()
	m.lines, m.spans, m.size = nil, nil, 0
	m.top, m.row, m.left = 0, 0, 0
	m.mark = -1
	m.clearSearch()
}

// renderName renders the name for the status line, on one line and without
// escape sequences.
func (m *Model) renderName() {
	m.nameView = m.styles.Name.Render(strings.Join(strings.Fields(termtext.OneLine(m.name)), " "))
}
