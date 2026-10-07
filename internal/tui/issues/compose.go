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
// focus, if the viewer may take a, what the prompt sends.
func (m *detailModal) compose(what composing, a ui.Action) tea.Cmd {
	if !m.loaded {
		return nil
	}
	if cmd, refused := m.gate().Refuse(a, &m.issue); refused {
		return cmd
	}
	num := "#" + strconv.Itoa(m.number)
	opts := []prompt.Option{prompt.WithStyles(m.theme.Prompt(m.icons)), prompt.WithKeyMap(m.keys.prompt)}
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

// promptDone handles what the prompt reports, if it is this modal's. A
// submit asks the user to confirm what it sends, and the prompt stays
// open with the text behind the question, so that a no goes back to it.
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
		// A second submit, such as a repeated key, arrives while the first
		// one asks.
		if msg.ID != m.prompt.ID() || m.ask != nil {
			return nil
		}
		if m.composing == composeComment {
			return m.submitComment(msg.Value)
		}
		return m.submitLabels(msg.Value)
	}
	return nil
}

// submitComment asks to post typed as a comment on the issue. An empty
// comment asks nothing, and the prompt stays open.
func (m *detailModal) submitComment(typed string) tea.Cmd {
	body := strings.TrimSpace(typed)
	c, ok, refusal := m.comment(body)
	if !ok {
		return refusal
	}
	ask := ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (ui.Confirm, bool, tea.Cmd) {
		if m.composing != composeComment || strings.TrimSpace(m.prompt.Value()) != body {
			return ui.Confirm{}, false, nil
		}
		return m.comment(body)
	})
	m.ask = &ask
	return nil
}

// comment returns posting body as a comment on the issue as a question,
// or ok unset when there is nothing to post or the viewer may not, which
// refusal then says. Its run closes the prompt, and the service shows the
// comment in the cache at once, as sending, so the thread reloads to show
// it.
func (m *detailModal) comment(body string) (c ui.Confirm, ok bool, refusal tea.Cmd) {
	if body == "" || !m.loaded {
		return ui.Confirm{}, false, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.issue); refused {
		return ui.Confirm{}, false, cmd
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return ui.Confirm{
		Question: "Post this comment on " + num + "?",
		Run: func() tea.Cmd {
			m.closePrompt()
			op := m.svc.Comment(m.repo, n, body)
			return tea.Batch(m.reload(), m.thread.Reload(), m.changed(),
				m.send(op, "comment on "+num))
		},
	}, true, nil
}

// submitLabels asks to make the issue's labels the comma-separated names
// in typed. When they are the labels it has, it closes the prompt and
// asks nothing.
func (m *detailModal) submitLabels(typed string) tea.Cmd {
	c, ok, refusal := m.labels(typed)
	switch {
	case refusal != nil:
		return refusal
	case !ok:
		m.closePrompt()
		return nil
	}
	// Label names may hold ", ", so two changes can read the same: the
	// yes compares what they add and remove too.
	added, removed := labelDiff(m.issue.Labels, typed)
	ask := ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (ui.Confirm, bool, tea.Cmd) {
		if m.composing != composeLabels || m.prompt.Value() != typed {
			return ui.Confirm{}, false, nil
		}
		a, r := labelDiff(m.issue.Labels, typed)
		if !sameNames(a, added) || !sameNames(r, removed) {
			return ui.Confirm{}, false, nil
		}
		return m.labels(typed)
	})
	m.ask = &ask
	return nil
}

// labels returns making the issue's labels the names in typed as a
// question, which names what it adds and removes, or ok unset when that
// changes nothing or the viewer may not, which refusal then says. Its run
// closes the prompt, adds the new labels in one change and removes each
// dropped one. The changes go one after another, so each answer from
// GitHub, which holds every label, includes the changes before it.
func (m *detailModal) labels(typed string) (c ui.Confirm, ok bool, refusal tea.Cmd) {
	if !m.loaded {
		return ui.Confirm{}, false, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActLabel, &m.issue); refused {
		return ui.Confirm{}, false, cmd
	}
	added, removed := labelDiff(m.issue.Labels, typed)
	if len(added) == 0 && len(removed) == 0 {
		return ui.Confirm{}, false, nil
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return ui.Confirm{
		Question: labelQuestion(num, added, removed),
		Run: func() tea.Cmd {
			m.closePrompt()
			cmds := make([]tea.Cmd, 0, len(removed)+1)
			if len(added) > 0 {
				op := m.svc.AddLabels(m.repo, n, added)
				cmds = append(cmds, m.send(op, "add "+strings.Join(added, ", ")+" to "+num))
			}
			for _, name := range removed {
				op := m.svc.RemoveLabel(m.repo, n, name)
				cmds = append(cmds, m.send(op, "remove "+name+" from "+num))
			}
			return tea.Batch(m.reload(), m.changed(), tea.Sequence(cmds...))
		},
	}, true, nil
}

// labelQuestion asks to add and remove the labels of issue num, such as
// "Add the label bug to #12 and remove wontfix?".
func labelQuestion(num string, added, removed []string) string {
	switch {
	case len(removed) == 0:
		return "Add " + labelNames(added) + " to " + num + "?"
	case len(added) == 0:
		return "Remove " + labelNames(removed) + " from " + num + "?"
	}
	return "Add " + labelNames(added) + " to " + num + " and remove " + oneLine(removed) + "?"
}

// labelNames names labels in a question, such as "the labels bug, ui".
func labelNames(names []string) string {
	if len(names) == 1 {
		return "the label " + oneLine(names)
	}
	return "the labels " + oneLine(names)
}

// oneLine joins names on one line, which a name with a line break of its
// own would break.
func oneLine(names []string) string {
	return ui.OneLine(strings.Join(names, ", "))
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

// sameNames reports whether a and b hold the same label names, in any
// order.
func sameNames(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func sameLabel(name string) func(string) bool {
	return func(other string) bool { return strings.EqualFold(strings.TrimSpace(other), name) }
}
