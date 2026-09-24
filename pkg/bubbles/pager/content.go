package pager

import (
	"context"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// binarySniff is how many leading bytes are checked for a NUL, the way git
// tells binary files apart.
const binarySniff = 8000

// SetContent shows text, named name, from its first line. The name picks
// the syntax, usually by its file extension. The returned command
// highlights the text in the background, and the pager shows it plain
// until then. Text with a NUL byte is taken for binary and not shown.
func (m *Model) SetContent(name, text string) tea.Cmd {
	m.reset(name, stateReady, nil)
	if strings.IndexByte(text[:min(len(text), binarySniff)], 0) >= 0 {
		m.state = stateBinary
		return nil
	}
	full := strings.TrimSuffix(clean(text, m.tabWidth), "\n")
	if full != "" {
		m.lines = strings.Split(full, "\n")
	}
	m.clamp()
	if len(full) > m.highlightLimit {
		return nil
	}
	lexer := lexerFor(name, full)
	if lexer == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	id, gen, n := m.id, m.gen, len(m.lines)
	return func() tea.Msg {
		defer cancel()
		spans, err := highlight(ctx, lexer, full, n)
		if err != nil {
			// The plain text stays; a failed highlight is not worth a
			// message.
			return nil
		}
		return highlightMsg{id: id, gen: gen, spans: spans}
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

// reset forgets the content and stops its highlighter.
func (m *Model) reset(name string, s state, err error) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
	m.name, m.state, m.err = name, s, err
	m.renderName()
	m.lines, m.spans = nil, nil
	m.top, m.row, m.left = 0, 0, 0
	m.clearSearch()
}

// clean expands tabs to spaces and replaces control characters and invalid
// UTF-8, so that the text can neither break the layout nor send escape
// sequences to the terminal. It drops the CR of CRLF line endings.
func clean(src string, tabWidth int) string {
	var b strings.Builder
	b.Grow(len(src))
	// col is the column at byte mark of the output, which is where the
	// last tab or line ended; the columns after it are measured lazily.
	mark, col := 0, 0
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == '\n':
			b.WriteByte('\n')
			mark, col = b.Len(), 0
		case r == '\t':
			col += ansi.StringWidth(b.String()[mark:])
			n := tabWidth - col%tabWidth
			for range n {
				b.WriteByte(' ')
			}
			mark, col = b.Len(), col+n
		case r == '\r' && strings.HasPrefix(src[i+size:], "\n"):
		case r == utf8.RuneError && size == 1, r < 0x20, r >= 0x7f && r < 0xa0:
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteString(src[i : i+size])
		}
		i += size
	}
	return b.String()
}

// renderName renders the name for the status line, on one line and without
// escape sequences.
func (m *Model) renderName() {
	m.nameView = m.styles.Name.Render(strings.Join(strings.Fields(ansi.Strip(m.name)), " "))
}
