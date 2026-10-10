package refs

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// readMsg carries the links one read found to the step that asked.
type readMsg struct {
	id   int64
	seq  int
	refs core.References
	err  error
}

// read reads the links. The ones this session read, or an earlier one
// kept within the TTL, cost no request. again reads past a kept copy.
func (s *Step) read(again bool) tea.Cmd {
	s.seq++
	s.loading = true
	q, svc, ctx, id, seq := s.query(again), s.svc, s.ctx, s.id, s.seq
	read := func() tea.Msg {
		ctx, end := obs.Begin(ctx, "refs.read")
		r, err := svc.References(ctx, q)
		end(err, "span", "tui", "repo", q.Repo.String(), "number", q.Number, "stale", r.Stale, "offline", r.Offline, "limited", r.Limited)
		return readMsg{id: id, seq: seq, refs: r, err: err}
	}
	return tea.Batch(read, s.startSpinner())
}

// Reread reads the links again unless they are as new as the item, which
// the service decides by the update time the modal knows by then, so that
// a change shows without costing a request when there is none.
func (s *Step) Reread() tea.Cmd {
	return s.read(false)
}

// refresh drops what is cached of the item and reads it again, with the
// mentions that were read.
func (s *Step) refresh() tea.Cmd {
	s.svc.Invalidate(s.self.Repo, s.self.Number)
	s.again.Store(true)
	return s.read(true)
}

// Update handles the keys while the step shows, and the messages of its
// reads and of the tree. It ignores those meant for others.
func (s *Step) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return s.press(msg)
	case readMsg:
		if msg.id != s.id {
			return nil
		}
		cmd = s.receive(msg)
	case tree.OpenMsg:
		if msg.ID != s.tree.ID() {
			return nil
		}
		return s.pick(msg.Node)
	case spinner.TickMsg:
		if msg.ID == s.spin.ID() {
			return s.spun(msg)
		}
		cmd = s.updateTree(msg)
	case ui.OnlineMsg:
		return s.online()
	case ui.ReopenedMsg:
		if s.opts.ret == nil || msg.Modal != s.opts.ret {
			return nil
		}
		// The links read meanwhile show; the tree keeps what it was left at.
		return s.read(false)
	case tea.PasteMsg:
		if s.prompt.Focused() {
			return s.updatePrompt(msg)
		}
		return nil
	default:
		cmd = s.updateTree(msg)
	}
	s.headings()
	return cmd
}

// updateTree passes msg to the tree, and starts a start that waited for
// it.
func (s *Step) updateTree(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.tree, cmd = s.tree.Update(msg)
	return tea.Batch(cmd, s.openFolds(), s.land())
}

// land puts the cursor on the first mention that a page read for the row of
// more brought, once it is listed, in place of the row of more that was
// under it.
func (s *Step) land() tea.Cmd {
	if s.landFrom < 0 {
		return nil
	}
	n := 0
	for i := range s.tree.Len() {
		node, _ := s.tree.At(i)
		if _, ok := node.Value.(itemValue); !ok || !strings.HasPrefix(node.ID, mentionedID+":") {
			continue
		}
		if n == s.landFrom {
			s.landFrom = -1
			return s.tree.Reveal(mentionedID, node.ID)
		}
		n++
	}
	return nil
}

// spun moves the spinner on while something loads.
func (s *Step) spun(msg spinner.TickMsg) tea.Cmd {
	if !s.loading || s.loaded {
		s.spinning = false
		return nil
	}
	var cmd tea.Cmd
	s.spin, cmd = s.spin.Update(msg)
	return cmd
}

// receive takes the links of a read. Links kept by an earlier session are
// shown, and read again at once, as the cache's rules ask. A read that
// failed leaves what is shown, and says so.
func (s *Step) receive(msg readMsg) tea.Cmd {
	if msg.seq != s.seq {
		return nil
	}
	s.loading = false
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			return nil
		}
		if s.loaded {
			return ui.Fail("read the links of "+s.self.String(), core.About(s.self.String(), msg.err))
		}
		s.err = msg.err
		s.layout()
		return nil
	}
	s.err = nil
	cmd := s.setRefs(msg.refs)
	if msg.refs.Stale {
		cmd = tea.Batch(cmd, s.read(true))
	}
	return cmd
}

// setRefs shows r. The first links start the tree; later ones load the
// open groups again, which keeps the folds and the cursor.
func (s *Step) setRefs(r core.References) tea.Cmd {
	s.refs, s.loaded = r, true
	snap := s.build(r)
	s.snap.Store(snap)
	if r.Mentioned == 0 {
		s.mread.Store(nil)
	}
	s.layout()
	if !s.started {
		return s.start()
	}
	return s.tree.ReloadNode("")
}

// start starts the tree once there are links to list.
func (s *Step) start() tea.Cmd {
	if s.started || s.snap.Load() == nil {
		return nil
	}
	s.started, s.folds = true, true
	return s.tree.Init()
}

// openFolds opens the groups that start open once the tree has them, and
// puts the cursor on the first item, not on a group.
func (s *Step) openFolds() tea.Cmd {
	snap := s.snap.Load()
	if !s.folds || snap == nil || s.tree.Len() == 0 {
		return nil
	}
	s.folds = false
	var cmds []tea.Cmd
	first := ""
	for _, g := range snap.groups {
		if g.id == mentionedID || g.id == writtenID && len(snap.refs.Written) > maxWrittenOpen {
			continue
		}
		cmds = append(cmds, s.tree.Expand(g.id))
		if first == "" && len(g.rows) > 0 {
			cmds = append(cmds, s.tree.Reveal(g.id, g.rows[0].ID))
			first = g.id
		}
	}
	return tea.Batch(cmds...)
}

// online reads the links again, now that GitHub answers again, if they
// failed or were served kept for want of an answer.
func (s *Step) online() tea.Cmd {
	if !ui.Unreached(s.err) && !s.refs.Offline && !s.refs.Limited {
		return s.tree.Retry()
	}
	return tea.Batch(s.read(true), s.tree.Retry())
}

// press takes a key: the filter's prompt, while it is open, then those of
// the step, and last the tree's.
func (s *Step) press(msg tea.KeyPressMsg) tea.Cmd {
	if s.prompt.Focused() {
		return s.updatePrompt(msg)
	}
	k := s.keys
	switch {
	case keymap.Matches(msg, k.Filter):
		return s.openPrompt()
	case keymap.Matches(msg, k.Dismiss):
		if s.filter == "" {
			id := s.id
			return func() tea.Msg { return CloseMsg{ID: id} }
		}
		return s.setFilter("")
	case keymap.Matches(msg, k.Refresh):
		return tea.Batch(s.refresh(), s.tree.Retry())
	case keymap.Matches(msg, k.Open):
		return s.openInBrowser()
	}
	if !s.loaded {
		return nil
	}
	cmd := s.updateTree(msg)
	s.headings()
	return cmd
}

// pick takes the row that enter opened. An item replaces the modal, which
// the back key returns to; the row of more mentions reads the next page.
func (s *Step) pick(n tree.Node) tea.Cmd {
	switch v := n.Value.(type) {
	case itemValue:
		return s.openItem(v.ref)
	case moreValue:
		if read := s.mread.Load(); read != nil {
			s.landFrom = read.rows
		}
		s.pages.Add(1)
		return s.tree.ReloadNode(mentionedID)
	}
	return nil
}

// openItem opens the item r in place of the modal the step is in, whose
// kind its row says; a link that can't be read says why, and where it can
// be opened.
func (s *Step) openItem(r core.Reference) tea.Cmd {
	if r.Problem != "" {
		text := "Can't read " + r.Target.String() + ": " + r.Problem + "."
		if h := s.keys.Open.Help().Key; h != "" {
			text += " " + h + " opens it in the browser."
		}
		return ui.Notify(toast.Warning, text)
	}
	if r.Target.Kind == core.KindPull {
		return func() tea.Msg { return ui.OpenPullMsg{Repo: r.Target.Repo, Number: r.Target.Number, Back: s.opts.ret} }
	}
	return func() tea.Msg { return ui.OpenIssueMsg{Repo: r.Target.Repo, Number: r.Target.Number, Back: s.opts.ret} }
}

// openInBrowser opens the item under the cursor on GitHub. An item that
// can't be read has no address of its own, so it goes to where a link to
// an issue goes, which GitHub redirects to a pull request.
func (s *Step) openInBrowser() tea.Cmd {
	n, ok := s.tree.Selected()
	v, item := n.Value.(itemValue)
	if !ok || !item {
		return nil
	}
	if v.ref.URL != "" {
		return ui.Open(v.ref.URL)
	}
	t := v.ref.Target
	return ui.Open(core.WebScheme(s.opts.host) + "://" + s.opts.host + "/" + t.Repo.String() + "/issues/" + strconv.Itoa(t.Number))
}

// openPrompt opens the prompt of the filter on the filter in force, if
// any.
func (s *Step) openPrompt() tea.Cmd {
	cmd := s.prompt.Open(s.filter)
	s.layout()
	return cmd
}

// updatePrompt passes msg to the prompt of the filter. The rows follow
// each key, since there is no request to wait for. Enter keeps the filter
// and gives the keys back, esc clears it and closes the prompt, and
// backspace on an empty line closes it.
func (s *Step) updatePrompt(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case keymap.Matches(k, s.keys.prompt.run):
			s.prompt.Blur()
			s.layout()
			return nil
		case keymap.Matches(k, s.keys.prompt.cancel):
			s.prompt.Blur()
			return s.setFilter("")
		}
	}
	var cmd tea.Cmd
	s.prompt, cmd = s.prompt.Update(msg)
	if !s.prompt.Focused() {
		// Backspace on an empty line closed it.
		s.layout()
		return nil
	}
	return tea.Batch(cmd, s.setFilter(s.prompt.Value()))
}

// setFilter shows only the rows that hold every word of text, in any of
// what the row says. The first words typed read the first page of the
// mentions, once, so that what is in them shows too.
func (s *Step) setFilter(text string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == s.filter {
		return nil
	}
	s.filter = text
	var cmd tea.Cmd
	if text == "" {
		s.tree.SetFilter(nil)
		s.asked = false
	} else {
		s.tree.SetFilter(matcher(text))
		if s.refs.Mentioned > 0 && !s.asked {
			// Once per filter, so that a read that fails isn't tried again
			// on every key; the refresh key tries it.
			s.asked = true
			cmd = s.tree.Load(mentionedID)
		}
	}
	s.headings()
	s.layout()
	return cmd
}

// matcher returns what tells whether a row holds every word of text,
// ignoring case. Only the rows of items hold anything.
func matcher(text string) func(tree.Node) bool {
	words := strings.Fields(strings.ToLower(text))
	return func(n tree.Node) bool {
		v, ok := n.Value.(itemValue)
		if !ok {
			return false
		}
		for _, w := range words {
			if !strings.Contains(v.hay, w) {
				return false
			}
		}
		return true
	}
}

// headings words the title of each group again, with its count: of all its
// rows, or of those the filter shows.
func (s *Step) headings() {
	snap := s.snap.Load()
	if snap == nil {
		return
	}
	for _, g := range snap.groups {
		s.tree.Rename(g.id, s.heading(g, snap.refs))
	}
}

// heading words the title of g: "Closes (2)", or "Closes (1 of 2)" while a
// filter looks at it. The mentions count GitHub's number until they are
// read, and then what they list, and how many of those GitHub counts it
// doesn't show, such as of repositories the viewer can't read.
func (s *Step) heading(g group, r core.References) string {
	shown, total := s.tree.Filtered(g.id)
	read := s.mread.Load()
	if g.id != mentionedID {
		// The notes and the row of more mentions are no items.
		items := len(g.rows) - notes(g.rows)
		if s.filter == "" {
			return g.title + " (" + strconv.Itoa(items) + ")"
		}
		return g.title + " (" + strconv.Itoa(shown) + " of " + strconv.Itoa(items) + ")"
	}
	if read != nil && read.more {
		total--
	}
	switch {
	case s.filter != "" && read != nil && read.more:
		return g.title + " (" + strconv.Itoa(shown) + " of the first " + strconv.Itoa(total) + " of " + strconv.Itoa(r.Mentioned) + ")"
	case s.filter != "":
		return g.title + " (" + strconv.Itoa(shown) + " of " + strconv.Itoa(total) + ")"
	case read == nil || read.more:
		return g.title + " (" + strconv.Itoa(r.Mentioned) + ")"
	}
	// What the other groups list was left out of the pages, so it isn't
	// hidden.
	hidden := r.Mentioned - read.rows - len(r.Closing) - len(r.Written)
	if hidden <= 0 {
		return g.title + " (" + strconv.Itoa(read.rows) + ")"
	}
	return g.title + " (" + strconv.Itoa(read.rows) + ", " + strconv.Itoa(hidden) + " hidden)"
}

// notes counts the rows of nodes that are notes.
func notes(nodes []tree.Node) int {
	n := 0
	for _, r := range nodes {
		if _, ok := r.Value.(noteValue); ok {
			n++
		}
	}
	return n
}
