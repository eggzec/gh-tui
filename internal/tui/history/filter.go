package history

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// fieldBranch is the key of the branch field of the filter.
const fieldBranch = "branch"

// openFilter opens the filter over the modal, once every page of branches is
// read, since the form's choices are fixed when it opens.
func (m *Modal) openFilter() tea.Cmd {
	b := &m.branches
	if !b.feed.Settled() {
		return ui.Notify(toast.Info, "The branches are still loading.")
	}
	b.wanted = true
	if cmd := m.openWanted(); cmd != nil {
		return cmd
	}
	// No branch is named like this, so the feed reads every page looking
	// for it.
	return b.feed.Seek("\x00")
}

// openWanted opens the filter that waits for the branches, if the last page
// is read or the read failed, and then waits no more. It also waits no more
// once the cursor moves or another pane takes the focus: wanted resets on
// either. A failed read opens it with the branches read so far, and says so.
func (m *Modal) openWanted() tea.Cmd {
	b := &m.branches
	err := b.feed.Err()
	if !b.wanted || (!b.feed.Done() && err == nil) {
		return nil
	}
	b.wanted = false
	open := ui.OpenModalOver(m.filterModal(), m)
	if err != nil {
		return tea.Batch(open, ui.Notify(toast.Warning, "The branch list is partial: reading it failed."))
	}
	return open
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
	if br, ok := m.branches.feed.Selected(); ok {
		q = fieldBranch + ":" + br.Name
	}
	return ui.Filter{Spec: m.spec(), Query: q, Subject: m.repo.String()}, true
}

// spec is the form of the filter: one choice among the branches.
func (m *Modal) spec() filterform.Spec {
	items := make([]filterform.Item, 0, m.branches.feed.Len())
	for i := range m.branches.feed.Len() {
		br, ok := m.branches.feed.LoadedItem(i)
		if !ok {
			continue
		}
		item := filterform.Item{Label: br.Name, Value: br.Name}
		if br.Name == m.branches.defaultBranch {
			item.Detail = "default"
		}
		items = append(items, item)
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
	// A branch the form listed before a reload may be on a page not read
	// yet, which the feed reads until it finds it.
	return tea.Batch(m.showBranch(name), m.branches.feed.Seek(name), m.branchSeen())
}
