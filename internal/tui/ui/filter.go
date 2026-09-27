package ui

import (
	"context"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// Filterable is a Section whose list is filtered and sorted in the filter
// modal, which the app opens with the filter key while the section has
// focus.
type Filterable interface {
	// Filter returns what the modal edits, or false while there is
	// nothing to filter, such as before a repository is chosen.
	Filter() (Filter, bool)
	// ApplyFilter shows the list the user applied in the modal. The app
	// calls it from Update.
	ApplyFilter(msg filterform.AppliedMsg) tea.Cmd
}

// Filter is what the filter modal edits for a Filterable.
type Filter struct {
	Spec filterform.Spec
	// Query is the GitHub query of the filters in force, which the form
	// opens on.
	Query string
	// Subject names what is filtered, such as the repository, in the
	// title of the modal after the section's.
	Subject string
}

// Sort options, whose orders read the same in every list and start where
// a list is best read: times newest first, counts most first, and names
// from A.

// SortByTime returns a sort option by a time, such as updated.
func SortByTime(label, value string) filterform.SortOption {
	return filterform.SortOption{Label: label, Value: value, Desc: "Newest first", Asc: "Oldest first"}
}

// SortByCount returns a sort option by a count, such as stars.
func SortByCount(label, value string) filterform.SortOption {
	return filterform.SortOption{Label: label, Value: value, Desc: "Most first", Asc: "Fewest first"}
}

// SortByName returns a sort option by a name.
func SortByName(label, value string) filterform.SortOption {
	return filterform.SortOption{Label: label, Value: value, Desc: "Z to A", Asc: "A to Z", Ascending: true}
}

// BestMatch is the sort option of GitHub's search that writes no sort:
// it ranks by relevance, which has no order.
var BestMatch = filterform.SortOption{Label: "Best match"}

// Chipper is a Section whose pane title says more than its title, such as
// the filters in force. Chips returns what follows the title, or "" for
// nothing. It is called after every message, so it must be cheap.
type Chipper interface {
	Chips() string
}

// Claimer is a Section that at times takes keys the app would handle,
// such as ] and [, which switch the section's tabs while the app would
// move to the next pane. Claims reports whether the section takes msg.
type Claimer interface {
	Claims(msg tea.KeyPressMsg) bool
}

// Fitter is a Modal that needs less room than the app offers. Fit returns
// the size it wants inside the frame, given the most it can have.
type Fitter interface {
	Fit(maxWidth, maxHeight int) (width, height int)
}

// FilterModal is the filter form of a Filterable as a modal. A list that
// can be sorted shows its sort on a tab after the filters, in the top edge
// of the frame. Applying the form, from either tab, closes it and applies
// both; esc closes it as it was.
type FilterModal struct {
	title  string
	target Filterable
	form   filterform.Model
	rows   int
}

// Size of the filter modal: wide enough for a row of chips, and as tall as
// the rows, an open picker and the query.
const (
	filterWidth = 100
	// filterSpare leaves room for the picker of a field, the rule and two
	// lines of query. Rows that wrap scroll.
	filterSpare = filterform.DefaultEditorHeight + 3
	// sortRows are the rows of the Sort tab: what is sorted by, and the
	// order.
	sortRows = 2
)

// FilterOption configures a FilterModal in [NewFilterModal].
type FilterOption func(*filterOptions)

type filterOptions struct {
	tab   filterform.Tab
	keys  filterform.KeyMap
	voice *Voice
}

// OnTab opens the modal on tab t. A list without a sort has only the
// Filters tab.
func OnTab(t filterform.Tab) FilterOption {
	return func(o *filterOptions) { o.tab = t }
}

// WithFormKeys sets the keys of the form.
func WithFormKeys(k filterform.KeyMap) FilterOption {
	return func(o *filterOptions) { o.keys = k }
}

// WithFormVoice words the options that failed to load with v, whose
// retry key the form replaces with its own. By default the form says it in
// words of its own.
func WithFormVoice(v Voice) FilterOption {
	return func(o *filterOptions) { o.voice = &v }
}

// FilterFormKeys returns the keys of a filter form. Its tabs switch with
// the keys that switch the tabs of the lists and the other modals.
func FilterFormKeys(keys map[string][]string) filterform.KeyMap {
	k := filterform.DefaultKeyMap()
	k.NextTab = Binding(keys, config.ActionNextFilter, "next tab")
	k.PrevTab = Binding(keys, config.ActionPrevFilter, "previous tab")
	return k
}

// NewFilterModal returns the modal that filters target with f, titled
// "Filter · section · subject". ctx bounds what the form loads.
func NewFilterModal(ctx context.Context, section string, target Filterable, f Filter, opts ...FilterOption) *FilterModal {
	o := filterOptions{keys: filterform.DefaultKeyMap()}
	for _, opt := range opts {
		opt(&o)
	}
	title := "Filter · " + section
	if f.Subject != "" {
		title += " · " + f.Subject
	}
	formOpts := []filterform.Option{
		filterform.WithQuery(f.Query),
		filterform.WithTab(o.tab),
		filterform.WithTabBar(false),
		filterform.WithHelpLine(false),
		filterform.WithKeyMap(o.keys),
		filterform.WithContext(ctx),
	}
	if o.voice != nil {
		// The form loads the options again with the key that opens them,
		// and has no key that opens GitHub.
		v := *o.voice
		v.Retry, v.Open = o.keys.Edit, key.Binding{}
		formOpts = append(formOpts, filterform.WithErrorText(ErrorText("load the options", f.Subject, v)))
	}
	form := filterform.New(f.Spec, formOpts...)
	// Focusing the rows, where the form starts, needs no command.
	_ = form.Focus()
	// The modal keeps its height on either tab.
	rows := len(f.Spec.Fields)
	if f.Spec.Sort != nil {
		rows = max(rows, sortRows)
	}
	return &FilterModal{title: title, target: target, form: form, rows: rows}
}

// Title implements Modal.
func (m *FilterModal) Title() string { return m.title }

// Update implements Modal. The form's own messages carry its ID, so the
// modal ignores those of other forms.
func (m *FilterModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case filterform.AppliedMsg:
		if msg.ID != m.form.ID() {
			return nil
		}
		return tea.Batch(CloseModal(m), m.target.ApplyFilter(msg))
	case filterform.CancelMsg:
		if msg.ID != m.form.ID() {
			return nil
		}
		return CloseModal(m)
	}
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return cmd
}

// View implements Modal.
func (m *FilterModal) View() string { return m.form.View() }

// SetSize implements Modal.
func (m *FilterModal) SetSize(width, height int) { m.form.SetSize(width, height) }

// Fit implements Fitter.
func (m *FilterModal) Fit(maxWidth, maxHeight int) (width, height int) {
	return min(maxWidth, filterWidth), min(maxHeight, m.rows+filterSpare)
}

// SetTheme implements Modal.
func (m *FilterModal) SetTheme(t Theme) { m.form.SetStyles(t.FilterForm()) }

// Help implements Modal.
func (m *FilterModal) Help() help.KeyMap { return m.form }

// Tabs implements Tabbed: Filters and Sort, or none for a list that can't
// be sorted.
func (m *FilterModal) Tabs() (names []string, active int) {
	names = m.form.Tabs()
	if names == nil {
		return nil, -1
	}
	return names, int(m.form.Tab())
}

// Query returns the GitHub query the form holds.
func (m *FilterModal) Query() string { return m.form.Query() }
