package ui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
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

// Fitter is a Modal that needs less room than the app offers. Fit returns
// the size it wants inside the frame, given the most it can have.
type Fitter interface {
	Fit(maxWidth, maxHeight int) (width, height int)
}

// FilterModal is the filter form of a Filterable as a modal. A list that
// can be sorted shows its sort on a tab after the filters, in the top edge
// of the frame. Enter applies the form from any row, on either tab, and
// closes it, applying both; esc and q close it as it was.
type FilterModal struct {
	title  string
	target Filterable
	form   filterform.Model
	rows   int
	icons  Icons
}

// Size of the filter modal: wide enough for a row of values, and as tall as
// the rows and what is under them, and the room a dropdown that is open
// needs beyond that.
const (
	// FilterWidth is the width of the modal of a filter form.
	FilterWidth = 100
	// filterBelow is what the form shows under its rows besides the
	// query: the rule and the help line.
	filterBelow = 2
	// sortRows are the rows of the Sort tab: what is sorted by, and the
	// order.
	sortRows = 2
)

// FilterHeight returns the height that a filter form width cells wide with
// rows rows needs: the rows, the rule, the query, which takes a second line
// when it wraps, and the help line under them, and the lines more that the
// dropdown that is open needs to fit under its row or above it. The
// dropdown floats over the rows under its row, the rule and the query, so a
// form with room enough doesn't grow.
func FilterHeight(rows, width int, f *filterform.Model) int {
	return rows + filterBelow + f.QueryLines(width) + f.DropdownExtra(rows, width)
}

// FilterOption configures a FilterModal in [NewFilterModal].
type FilterOption func(*filterOptions)

type filterOptions struct {
	tab   filterform.Tab
	keys  filterform.KeyMap
	voice *Voice
	icons Icons
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
// retry key the form replaces with its own, r by default. By default the
// form says it in words of its own.
func WithFormVoice(v Voice) FilterOption {
	return func(o *filterOptions) { o.voice = &v }
}

// WithFormIcons marks what failed to load with the error glyph of ic.
// Without it, the icons are the config's default.
func WithFormIcons(ic Icons) FilterOption {
	return func(o *filterOptions) { o.icons = ic }
}

// FilterFormKeys returns the keys of a filter form in the modal of context
// ctx, "filter" or "actions_filter". Its tabs switch with the keys that
// switch the tabs of the lists and the other modals, and it closes and
// loads again with their quit and refresh keys. ctrl+c quits the app from
// every modal, so it doesn't close this one.
func FilterFormKeys(keys config.Keymap, ctx string) filterform.KeyMap {
	return filterform.NewKeyMap(Lookup(keys, ctx))
}

// NewFilterModal returns the modal that filters target with f, titled
// "Filter · section · subject". ctx bounds what the form loads.
func NewFilterModal(ctx context.Context, section string, target Filterable, f Filter, opts ...FilterOption) *FilterModal {
	o := filterOptions{keys: FilterFormKeys(config.Default().Keys, "filter"), icons: NewIcons(config.Default().UI.Icons)}
	for _, opt := range opts {
		opt(&o)
	}
	title := "Filter" + o.icons.Separator + section
	if f.Subject != "" {
		title += o.icons.Separator + f.Subject
	}
	formOpts := []filterform.Option{
		filterform.WithQuery(f.Query),
		filterform.WithTab(o.tab),
		filterform.WithTabBar(false),
		filterform.WithHelpLine(true),
		filterform.WithKeyNames(o.icons.Key),
		filterform.WithKeyMap(o.keys),
		filterform.WithContext(ctx),
	}
	if o.voice != nil {
		// The form loads the options again with its retry key, and has no
		// key that opens GitHub.
		v := *o.voice
		v.Retry, v.Open, v.Icons = o.keys.Retry, key.Binding{}, &o.icons
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
	return &FilterModal{title: title, target: target, form: form, rows: rows, icons: o.icons}
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
	case OnlineMsg:
		// The options that failed for want of an answer load again.
		return RetryUnreached(&m.form)
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
	width = min(maxWidth, FilterWidth)
	return width, min(maxHeight, FilterHeight(m.rows, width, &m.form))
}

// SetTheme implements Modal.
func (m *FilterModal) SetTheme(t Theme) { m.form.SetStyles(t.FilterForm(m.icons)) }

// KeyLayers implements Keyed: the keys of the form, which types what the
// picker or the query line takes in insert mode. The form shows its own
// help line, so the layer has no short help for the footer.
func (m *FilterModal) KeyLayers() []keyhelp.Layer {
	ctx := map[filterform.Capture]string{
		filterform.CaptureNone: "filter", filterform.CaptureQuery: "filter_query",
		filterform.CaptureEditor: "filter_query", filterform.CapturePicker: "picker",
	}[m.form.CapturedBy()]
	l := ContextHelp(ctx, m.form, m.form.Capturing())
	l.Short = nil
	return []keyhelp.Layer{l}
}

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

var _ Actor = (*FilterModal)(nil)

// Act implements Actor. The quit key closes the modal without applying
// what the form holds; every other intent is the app's to refuse while it
// is open.
func (m *FilterModal) Act(action string) (tea.Cmd, bool) {
	if action == ActQuit {
		return CloseModal(m), true
	}
	return nil, false
}
