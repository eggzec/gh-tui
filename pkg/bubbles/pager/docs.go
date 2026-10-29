package pager

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
)

// Doc is one text of a list of documents: its name, the language to
// highlight it as (empty to pick it by the name, as [Model.SetContent]
// does), and its text.
type Doc struct {
	Name, Lang, Text string
}

// docState is what a pager keeps of a document that is not the one shown:
// the fields of Model that belong to a text. What the pager shares between
// documents, such as its settings, prompt and case mode, is not here.
type docState struct {
	// pending is set for a document that was only given as a Doc, which is
	// read when it is first shown; name, lang and raw hold it until then.
	pending bool

	name             string
	state            state
	err              error
	errText, errHint string
	note             string
	rows             Rows
	spans            [][]syntax.Span
	raw, lang        string
	render           Render
	renderedAt       int
	pics             map[int]string
	vis              []int32
	size             int
	anchor           anchor
	atEnd, owed      bool
	sizeSeq          int
	proj, want       projection
	kept             int
	top, row, left   int
	mark             int
	search           search
	hits             hits
}

// SetDocs shows the first of docs and keeps the rest, in order, to step to
// with [Model.Step]; each keeps its own place, search, mark and filter. The
// returned command highlights the first. No documents show an empty pager.
func (m *Model) SetDocs(docs []Doc) tea.Cmd {
	m.dropDocs()
	if len(docs) == 0 {
		m.reset("", stateEmpty, nil)
		return nil
	}
	m.docs = make([]docState, len(docs))
	for i, d := range docs {
		m.docs[i] = pendingDoc(d)
	}
	return m.fill(docs[0])
}

// Add appends d to the documents without showing it, and returns its index.
// A pager that shows one text then has two.
func (m *Model) Add(d Doc) int {
	docs := m.docs
	if docs == nil {
		// The text shown is document 0; its slot is filled when it is left.
		docs = []docState{{}}
	}
	// A copy, since copies of the model share the slice, and one that adds
	// must not show the other its document.
	m.docs = append(slices.Clone(docs), pendingDoc(d))
	return len(m.docs) - 1
}

// Step shows the document delta places after the one shown, or before it
// for a negative delta, stopping at the first and the last. It returns the
// command that restarts what the document it leaves to had not finished
// (its highlight, search or filter), and whether another document shows.
func (m *Model) Step(delta int) (tea.Cmd, bool) {
	to := min(max(m.cur+delta, 0), len(m.docs)-1)
	if len(m.docs) < 2 || to == m.cur {
		return nil, false
	}
	m.save()
	m.leave()
	m.cur = to
	return m.load(), true
}

// SetDoc replaces document i with d. For the one shown it is as
// SetContent, and the list stays; for another it is kept for when that is
// shown, and what is shown stays as it is. It returns the command that
// highlights the document shown.
func (m *Model) SetDoc(i int, d Doc) tea.Cmd {
	if i < 0 || i >= m.Docs() {
		return nil
	}
	if i == m.cur {
		return m.fill(d)
	}
	// A copy, as Add does.
	m.docs = slices.Clone(m.docs)
	m.docs[i] = pendingDoc(d)
	return nil
}

// Docs returns how many documents the pager holds, which is 1 for a pager
// that shows a single text.
func (m Model) Docs() int { return max(len(m.docs), 1) }

// Doc returns the index of the document shown, from 0.
func (m Model) Doc() int { return m.cur }

func pendingDoc(d Doc) docState {
	return docState{pending: true, name: d.Name, lang: d.Lang, raw: d.Text, mark: -1}
}

// dropDocs forgets the list, for a setter that shows one text.
func (m *Model) dropDocs() {
	m.docs, m.cur = nil, 0
}

// fill shows d in place of the document shown.
func (m *Model) fill(d Doc) tea.Cmd {
	if d.Lang == "" {
		return m.setContent(d.Name, d.Text, func(full string) chroma.Lexer { return lexerFor(d.Name, full) })
	}
	cmd := m.setContent(d.Name, d.Text, func(string) chroma.Lexer { return lexerNamed(d.Lang) })
	m.lang = d.Lang
	return cmd
}

// save keeps the document shown in its slot.
func (m *Model) save() {
	d := docState{
		name: m.name, state: m.state, err: m.err, errText: m.errText, errHint: m.errHint,
		note: m.note, rows: m.rows, spans: m.spans, raw: m.raw, lang: m.lang,
		render: m.render, renderedAt: m.renderedAt, pics: m.pics,
		vis: m.vis, size: m.size, anchor: m.anchor, atEnd: m.atEnd, owed: m.owed,
		sizeSeq: m.sizeSeq, proj: m.proj, want: m.want, kept: m.kept,
		top: m.top, row: m.row, left: m.left, mark: m.mark,
		search: m.search, hits: m.hits,
	}
	// A copy, as Add does: a copy of the model that steps writes its own.
	m.docs = slices.Clone(m.docs)
	m.docs[m.cur] = d
}

// leave stops the work of the document shown. The counters only grow, so
// what that work sends later matches no document.
func (m *Model) leave() {
	m.closePrompt()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
	if m.stopSearch != nil {
		m.stopSearch()
		m.stopSearch = nil
	}
	m.qgen++
	if m.stopProject != nil {
		m.stopProject()
		m.stopProject = nil
	}
	m.pgen++
	m.projecting = false
	m.flash, m.opt = "", false
}

// load shows document m.cur from its slot, and returns the command that
// restarts what it had not finished.
func (m *Model) load() tea.Cmd {
	d := m.docs[m.cur]
	if d.pending {
		return m.fill(Doc{Name: d.name, Lang: d.lang, Text: d.raw})
	}
	// Squeeze is an option of the pager, not of the document.
	squeeze := m.proj.squeeze
	changed := d.proj.squeeze != squeeze
	d.proj.squeeze, d.want.squeeze = squeeze, squeeze
	m.name, m.state, m.err, m.errText, m.errHint = d.name, d.state, d.err, d.errText, d.errHint
	m.note, m.rows, m.spans, m.raw, m.lang = d.note, d.rows, d.spans, d.raw, d.lang
	m.render, m.renderedAt, m.pics = d.render, d.renderedAt, d.pics
	m.vis, m.size, m.anchor, m.atEnd, m.owed, m.sizeSeq = d.vis, d.size, d.anchor, d.atEnd, d.owed, d.sizeSeq
	m.proj, m.want, m.kept = d.proj, d.want, d.kept
	m.top, m.row, m.left, m.mark = d.top, d.row, d.left, d.mark
	m.search, m.hits = d.search, d.hits
	m.renderName()
	if m.render != nil {
		// It was fitted to the width at the time, so it fits again once, at
		// the width now, keeping its place.
		m.renderedAt, m.owed, m.atEnd = -1, false, false
		m.fitRendered(false)
	}
	m.clamp()
	m.enableSearchKeys()

	var cmds []tea.Cmd
	if m.state == stateLoading {
		cmds = append(cmds, m.spin.Tick)
	}
	if lines, ok := m.plainLines(); ok && m.state == stateReady && m.render == nil && m.spans == nil && len(lines) > 0 {
		full := strings.Join(lines, "\n")
		lang, name := m.lang, m.name
		cmds = append(cmds, m.highlight(full, func(full string) chroma.Lexer {
			if lang != "" {
				return lexerNamed(lang)
			}
			return lexerFor(name, full)
		}))
	}
	switch {
	case changed || m.want != m.proj:
		cmds = append(cmds, m.project(m.want))
	case m.search.running:
		cmds = append(cmds, m.researchShown())
	}
	return tea.Batch(cmds...)
}
