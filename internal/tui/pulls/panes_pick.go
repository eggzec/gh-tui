package pulls

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// labelEditor is what the labels picker needs to change the labels.
type labelEditor interface {
	// EditLabels shows the labels in add as put on the pull request, and those
	// named in remove as taken off, in the cache at once. The returned Op sends
	// the change.
	EditLabels(repo core.RepoRef, number int, add []core.Label, remove []string) *optimistic.Op
}

// pickerOptions are the options the people pickers share. known are the rows
// that stay ticked when Enter comes before the first answer, or after a
// failed search.
func (m *panesModal) pickerOptions(known []picker.Item) []picker.Option {
	return []picker.Option{
		picker.WithContext(m.ctx),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithMarkedFirst(true),
		picker.WithDetailMatch(true),
		picker.WithKeyMap(m.keys.pick),
		picker.WithKnown(known),
	}
}

// labelsReadMsg is the answer of the read of the labels of the repository.
type labelsReadMsg struct {
	id     int64
	labels []core.Label
	err    error
}

// openLabels reads the labels the repository has and opens the picker of the
// labels of the pull request over them. Without a read of the labels it offers
// those the pull request has.
func (m *panesModal) openLabels() tea.Cmd {
	if !m.loaded || m.top() != nil {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActLabel, &m.detail.Issue); refused {
		return cmd
	}
	f := m.list.facets
	if f == nil {
		m.showLabels(nil)
		return m.focusPicker()
	}
	id, ctx, repo := m.id, m.ctx, m.repo
	return func() tea.Msg {
		labels, err := f.Labels(ctx, repo)
		return labelsReadMsg{id: id, labels: labels, err: err}
	}
}

// labelsRead opens the picker over the labels that were read.
func (m *panesModal) labelsRead(msg labelsReadMsg) tea.Cmd {
	if msg.id != m.id || m.top() != nil {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		return ui.Fail("labels of #"+strconv.Itoa(m.number), core.About(core.Target{Repo: m.repo, Number: m.number}.String(), msg.err))
	}
	m.showLabels(msg.labels)
	return m.focusPicker()
}

// focusPicker focuses the picker that was just opened.
func (m *panesModal) focusPicker() tea.Cmd {
	if o, ok := m.top().(*labelsOverlay); ok {
		return o.P.Focus()
	}
	return nil
}

// showLabels opens the picker over known and the labels the pull request has,
// which stay on offer though the read of the repository lacks them.
func (m *panesModal) showLabels(known []core.Label) {
	have := m.detail.Labels
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
			return m.applyLabels(add, remove)
		},
	}
	items := make([]picker.Item, len(all))
	for i, l := range all {
		labels[l.Name] = l
		items[i] = picker.Item{Title: l.Name, Detail: l.Description, Value: l.Name}
	}
	for _, l := range have {
		// The pull request's name may differ in case from the repository's.
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
	m.open(o)
}

// labelsOverlay is the checklist of the labels of the pull request, keyed by
// label name.
type labelsOverlay = prpanes.TickOverlay[string]

// applyLabels changes the labels of the pull request, which the header shows
// at once, and reads it again once the change went through or was rolled back.
func (m *panesModal) applyLabels(add []core.Label, remove []string) tea.Cmd {
	op := m.svc.EditLabels(m.repo, m.number, add, remove)
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
		m.refreshInvolved()
	}
	return m.sendOp(op, "labels on #"+strconv.Itoa(m.number))
}
