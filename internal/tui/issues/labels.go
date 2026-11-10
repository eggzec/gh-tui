package issues

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// labelsReadMsg is the labels of the repository, read for the picker of the
// modal with id.
type labelsReadMsg struct {
	id     int64
	labels []core.Label
	err    error
}

// openLabels shows the labels of the repository over the panes, ticked for
// those the issue has. The repository's labels are read first, if the section
// has a source for them.
func (m *detailModal) openLabels() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, "Still reading #"+strconv.Itoa(m.number)+".")
	}
	if m.busy() {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActLabel, &m.issue); refused {
		return cmd
	}
	if m.facets == nil {
		return m.showLabels(nil)
	}
	id, ctx, repo, f := m.id, m.ctx, m.repo, m.facets
	return func() tea.Msg {
		labels, err := f.Labels(ctx, repo)
		return labelsReadMsg{id: id, labels: labels, err: err}
	}
}

// busy reports whether something else is open over the conversation: an
// overlay, a question, or the references.
func (m *detailModal) busy() bool {
	return m.over != nil || m.ask != nil || m.refs != nil
}

// labelsRead opens the picker over the labels read, unless the modal has
// since opened something else.
func (m *detailModal) labelsRead(msg labelsReadMsg) tea.Cmd {
	if msg.id != m.id || m.busy() {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		return ui.Fail("labels of #"+strconv.Itoa(m.number), core.About(m.subject(), msg.err))
	}
	return m.showLabels(msg.labels)
}

// labelsOverlay is the checklist of the labels of the issue, keyed by label
// name.
type labelsOverlay = prpanes.TickOverlay[string]

// showLabels opens the picker over known and the labels the issue has, which
// stay on offer though the read of the repository lacks them. Applying asks
// to confirm the change.
func (m *detailModal) showLabels(known []core.Label) tea.Cmd {
	have := m.issue.Labels
	all := slices.Clone(known)
	for _, l := range have {
		if !slices.ContainsFunc(all, func(o core.Label) bool { return strings.EqualFold(o.Name, l.Name) }) {
			all = append(all, l)
		}
	}
	labels := map[string]core.Label{}
	o := &labelsOverlay{
		Heading: "Labels",
		Order:   strings.ToLower,
		On:      map[string]bool{},
		Was:     map[string]bool{},
		Change: func(on, was map[string]bool) tea.Cmd {
			var add []core.Label
			var remove []string
			for name, l := range labels {
				switch {
				case on[name] && !was[name]:
					add = append(add, l)
				case !on[name] && was[name]:
					remove = append(remove, name)
				}
			}
			if len(add) == 0 && len(remove) == 0 {
				return nil
			}
			slices.SortFunc(add, func(a, b core.Label) int { return strings.Compare(a.Name, b.Name) })
			slices.Sort(remove)
			return m.askLabels(add, remove)
		},
	}
	items := make([]picker.Item, len(all))
	for i, l := range all {
		labels[l.Name] = l
		items[i] = picker.Item{Title: l.Name, Detail: l.Description, Value: l.Name}
	}
	for _, l := range have {
		// The issue's name may differ in case from the repository's.
		for _, a := range all {
			if strings.EqualFold(a.Name, l.Name) {
				o.On[a.Name], o.Was[a.Name] = true, true
			}
		}
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o.Rows = len(items)
	o.P = picker.New(nil,
		picker.WithContext(m.ctx),
		picker.WithItems(items),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithMarkedFirst(true),
		picker.WithDetailMatch(true),
		picker.WithTitleStyle(func(it picker.Item, base lipgloss.Style) lipgloss.Style {
			name, _ := it.Value.(string)
			return m.chips.Tint(base, labels[name].Color)
		}),
		picker.WithKeyMap(m.keys.pick),
		picker.WithStyles(st),
	)
	o.P.SetMarked(o.Marks())
	// The ticked labels are listed first, so the cursor starts on the first.
	if first, ok := o.FirstTicked(); ok {
		o.P.Select(first)
	}
	m.over = o
	return o.P.Focus()
}

// askLabels asks to add and remove labels on the issue, naming each.
func (m *detailModal) askLabels(add []core.Label, remove []string) tea.Cmd {
	c, ok, refusal := m.labels(add, remove)
	if !ok {
		return refusal
	}
	ask := ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (confirm.Ask, bool, tea.Cmd) {
		return m.labels(add, remove)
	})
	ask.Danger = len(remove) > 0
	return m.asks(ask)
}

// labels returns adding and removing the labels as a question, or ok unset
// when there is nothing to change or the viewer may not, which refusal then
// says. Its run shows the change in the cache at once and sends it.
func (m *detailModal) labels(add []core.Label, remove []string) (c confirm.Ask, ok bool, refusal tea.Cmd) {
	if !m.loaded || (len(add) == 0 && len(remove) == 0) {
		return confirm.Ask{}, false, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActLabel, &m.issue); refused {
		return confirm.Ask{}, false, cmd
	}
	names := make([]string, len(add))
	for i, l := range add {
		names[i] = l.Name
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return confirm.Ask{
		Question: labelQuestion(num, names, remove),
		Run: func() tea.Cmd {
			op := m.svc.EditLabels(m.repo, n, add, remove)
			return tea.Batch(m.reload(), m.changed(), m.send(op, "labels on "+num))
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
