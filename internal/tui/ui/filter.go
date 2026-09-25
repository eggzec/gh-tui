package ui

import (
	"context"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

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

// FilterModal is the filter form of a Filterable as a modal. Applying the
// form closes it and applies the filter; esc closes it as it was.
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
)

// NewFilterModal returns the modal that filters target with f, titled
// "Filter · section · subject". ctx bounds what the form loads.
func NewFilterModal(ctx context.Context, section string, target Filterable, f Filter) *FilterModal {
	title := "Filter · " + section
	if f.Subject != "" {
		title += " · " + f.Subject
	}
	form := filterform.New(f.Spec,
		filterform.WithQuery(f.Query),
		filterform.WithHelpLine(false),
		filterform.WithContext(ctx),
	)
	// Focusing the rows, where the form starts, needs no command.
	_ = form.Focus()
	rows := len(f.Spec.Fields)
	if f.Spec.Sort != nil {
		rows++
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

// Query returns the GitHub query the form holds.
func (m *FilterModal) Query() string { return m.form.Query() }
