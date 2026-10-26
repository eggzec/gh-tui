package history

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// fieldBranch is the key of the branch field of the filter.
const fieldBranch = "branch"

// openFilter opens the filter over the modal, once every page of branches is
// read, since the form's choices are fixed when it opens.
func (m *Modal) openFilter() tea.Cmd {
	b := &m.branches
	if !b.loaded {
		return nil
	}
	b.wanted = true
	if cmd := m.openWanted(); cmd != nil {
		return cmd
	}
	return m.moreBranches()
}

// openWanted opens the filter that waits for the branches, if the last page
// is read or the read failed, and then waits no more. It also waits no more
// once the cursor moves or another pane takes the focus.
func (m *Modal) openWanted() tea.Cmd {
	b := &m.branches
	if !b.wanted || b.loading || (b.next != "" && b.err == nil) {
		return nil
	}
	b.wanted = false
	return ui.OpenModalOver(m.filterModal(), m)
}

// filterModal returns the filter over the modal.
func (m *Modal) filterModal() ui.Modal {
	f, _ := m.Filter()
	return ui.NewFilterModal(m.ctx, "History", m, f,
		ui.WithFormKeys(m.keys.form),
		ui.WithFormBack(m.keys.Back),
		ui.WithFormVoice(*m.opts.voice),
		ui.WithFormIcons(m.opts.icons),
	)
}

// Filter implements ui.Filterable: the form opens on the branch under the
// cursor.
func (m *Modal) Filter() (ui.Filter, bool) {
	q := ""
	if br, ok := m.branches.selected(); ok {
		q = fieldBranch + ":" + br.Name
	}
	return ui.Filter{Spec: m.spec(), Query: q, Subject: m.repo.String()}, true
}

// spec is the form of the filter: one choice among the branches.
func (m *Modal) spec() filterform.Spec {
	items := make([]filterform.Item, len(m.branches.items))
	for i, br := range m.branches.items {
		items[i] = filterform.Item{Label: br.Name, Value: br.Name}
		if br.Name == m.branches.defaultBranch {
			items[i].Detail = "default"
		}
	}
	return filterform.Spec{Fields: []filterform.Field{{
		Key: fieldBranch, Label: "Branch", Kind: filterform.Choice, Qualifier: fieldBranch, Options: items,
	}}}
}

// ApplyFilter implements ui.Filterable: the branch chosen is shown.
func (m *Modal) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	name := msg.Values[fieldBranch].Text()
	if name == "" {
		return nil
	}
	b := &m.branches
	b.follow = ""
	if i := b.index(name); i >= 0 {
		b.cursor = i
		m.scrollBranches()
	} else {
		// Only what the form listed before the branches were read again
		// from the first page has it, so the cursor follows it to the page
		// that brings it back.
		b.follow = name
	}
	return tea.Batch(m.showBranch(name), m.branchMoved())
}
