package issues

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

// composing is what the prompt under the thread of a modal is for.
type composing int

const (
	composeNone composing = iota
	composeComment
	composeLabels
)

// Heights of the comment prompt, which takes a third of the modal within
// these bounds.
const (
	minCommentHeight = 5
	maxCommentHeight = 10
)

// compose opens the prompt for what under the thread, and gives it the
// focus.
func (m *detailModal) compose(what composing) tea.Cmd {
	if !m.loaded {
		return nil
	}
	num := "#" + strconv.Itoa(m.number)
	opts := []prompt.Option{prompt.WithStyles(m.theme.Prompt())}
	switch what {
	case composeComment:
		opts = append(opts,
			prompt.WithTitle("Comment on "+num),
			prompt.WithPlaceholder("Write a comment in markdown."))
	case composeLabels:
		names := make([]string, len(m.issue.Labels))
		for i, l := range m.issue.Labels {
			names[i] = l.Name
		}
		opts = append(opts,
			prompt.WithMode(prompt.SingleLine),
			prompt.WithTitle("Labels of "+num),
			prompt.WithPlaceholder("Comma-separated, such as bug, help wanted"),
			prompt.WithValue(strings.Join(names, ", ")))
	default:
		return nil
	}
	m.prompt = prompt.New(opts...)
	m.composing = what
	m.layout()
	m.thread.Blur()
	return m.prompt.Focus()
}

// closePrompt closes the prompt and gives the focus back to the thread.
func (m *detailModal) closePrompt() {
	if m.composing == composeNone {
		return
	}
	m.composing = composeNone
	m.prompt.Blur()
	m.layout()
	m.thread.Focus()
}

// layout shares the height of the modal between the thread and the prompt,
// if it is open.
func (m *detailModal) layout() {
	h := m.height
	ph := 0
	switch m.composing {
	case composeComment:
		ph = min(max(h/3, minCommentHeight), maxCommentHeight, h)
	case composeLabels:
		ph = min(prompt.SingleLineHeight, h)
	case composeNone:
	}
	if ph > 0 {
		m.prompt.SetSize(m.width, ph)
	}
	m.thread.SetSize(m.width, h-ph)
}

// promptDone handles what the prompt reports, if it is this modal's.
func (m *detailModal) promptDone(msg tea.Msg) tea.Cmd {
	if m.composing == composeNone {
		return nil
	}
	switch msg := msg.(type) {
	case prompt.CancelMsg:
		if msg.ID == m.prompt.ID() {
			m.closePrompt()
		}
	case prompt.SubmitMsg:
		if msg.ID != m.prompt.ID() {
			return nil
		}
		if m.composing == composeComment {
			return m.submitComment(msg.Value)
		}
		return m.submitLabels(msg.Value)
	}
	return nil
}

// submitComment sends body as a comment on the issue. The service shows it
// in the cache at once, as sending, so the thread reloads to show it. An
// empty comment is not sent, and the prompt stays open.
func (m *detailModal) submitComment(body string) tea.Cmd {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	m.closePrompt()
	n := m.number
	op := m.svc.Comment(m.repo, n, body)
	return tea.Batch(m.reload(), m.thread.Reload(), m.changed(),
		ui.Do(m.sendCtx, ui.IssuesTitle, op, "comment on #"+strconv.Itoa(n)))
}

// submitLabels makes the issue's labels the comma-separated names in
// typed: it adds the new ones in one change and removes each dropped one.
// The changes go one after another, so each answer from GitHub, which holds
// every label, includes the changes before it.
func (m *detailModal) submitLabels(typed string) tea.Cmd {
	m.closePrompt()
	added, removed := labelDiff(m.issue.Labels, typed)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	cmds := make([]tea.Cmd, 0, len(removed)+1)
	if len(added) > 0 {
		op := m.svc.AddLabels(m.repo, n, added)
		cmds = append(cmds, ui.Do(m.sendCtx, ui.IssuesTitle, op, "add "+strings.Join(added, ", ")+" to "+num))
	}
	for _, name := range removed {
		op := m.svc.RemoveLabel(m.repo, n, name)
		cmds = append(cmds, ui.Do(m.sendCtx, ui.IssuesTitle, op, "remove "+name+" from "+num))
	}
	return tea.Batch(m.reload(), m.changed(), tea.Sequence(cmds...))
}

// labelDiff compares the comma-separated label names in typed with the
// labels an issue has. GitHub treats names that differ only in case as the
// same label, so the comparison does too, and space around names doesn't
// count. Added names keep their order and removed ones the issue's.
func labelDiff(have []core.Label, typed string) (added, removed []string) {
	var want []string
	for name := range strings.SplitSeq(typed, ",") {
		name = strings.TrimSpace(name)
		if name != "" && !slices.ContainsFunc(want, sameLabel(name)) {
			want = append(want, name)
		}
	}
	for _, name := range want {
		if !slices.ContainsFunc(have, func(l core.Label) bool { return sameLabel(name)(l.Name) }) {
			added = append(added, name)
		}
	}
	for _, l := range have {
		if !slices.ContainsFunc(want, sameLabel(l.Name)) {
			removed = append(removed, l.Name)
		}
	}
	return added, removed
}

func sameLabel(name string) func(string) bool {
	return func(other string) bool { return strings.EqualFold(strings.TrimSpace(other), name) }
}
