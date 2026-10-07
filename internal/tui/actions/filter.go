package actions

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// filterStep is the filter of the runs, a step of the modal. It waits for
// the workflows, which its first field chooses from, before it shows the
// form.
type filterStep struct {
	form *filterform.Model
	// rows is the number of rows of the form.
	rows int
}

// workflows are the workflows of the repository, read the first time the
// filter opens.
type workflows struct {
	items           []core.Workflow
	loaded, loading bool
	// kept reports that the items were served from what an earlier read
	// kept, because GitHub couldn't be reached or rate limited the read,
	// so they are read again once it answers.
	kept bool
}

// Keys of the fields of the filter.
const (
	fieldWorkflow = "workflow"
	fieldBranch   = "branch"
	fieldEvent    = "event"
	fieldStatus   = "status"
	fieldActor    = "actor"
)

// events are the events that trigger most runs, as the runs API names
// them. Others can be typed in the query.
var events = []string{"push", "pull_request", "pull_request_target", "schedule", "workflow_dispatch", "merge_group", "release"}

// statuses are what the status of the runs API takes most: a status or a
// conclusion.
var statuses = []filterform.Item{
	{Label: "Any"},
	{Label: "Failure", Value: "failure"},
	{Label: "Success", Value: "success"},
	{Label: "In progress", Value: "in_progress"},
	{Label: "Queued", Value: "queued"},
	{Label: "Cancelled", Value: "cancelled"},
	{Label: "Action required", Value: "action_required"},
	{Label: "Timed out", Value: "timed_out"},
}

// openFilter opens the filter step, once the workflows are read.
func (m *Modal) openFilter() tea.Cmd {
	m.filterStep = &filterStep{}
	if m.workflows.loaded {
		return m.showForm()
	}
	if m.workflows.loading {
		return nil
	}
	m.workflows.loading = true
	return tea.Batch(m.readWorkflows(false), m.startSpinner())
}

// workflowsMsg carries the workflows of the repository. Stale reports that
// they were kept by an earlier session, and kept that they were served
// kept while GitHub couldn't be reached or rate limited the read.
type workflowsMsg struct {
	id          int64
	items       []core.Workflow
	stale, kept bool
	err         error
}

// readWorkflows reads the workflows of the repository, past the ones an
// earlier session kept with again set.
func (m *Modal) readWorkflows(again bool) tea.Cmd {
	svc, ctx, id, repo := m.svc, m.ctx, m.id, m.repo
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.workflows")
		p, err := svc.Workflows(ctx, actionssvc.WorkflowsQuery{Repo: repo, Again: again})
		end(err, "span", "tui", "workflows", len(p.Items), "stale", p.Stale, "offline", p.Offline, "limited", p.Limited)
		return workflowsMsg{id: id, items: p.Items, stale: p.Stale, kept: p.Offline || p.Limited, err: err}
	}
}

// receiveWorkflows keeps the workflows, and shows the form that waited for
// them. Without them, the form has no workflows to choose from, and a read
// later tries again. Workflows an earlier session kept are shown, and read
// again at once.
func (m *Modal) receiveWorkflows(msg workflowsMsg) tea.Cmd {
	m.workflows.loading = false
	if msg.err != nil && ui.Unreached(msg.err) && m.workflows.loaded {
		// The workflows shown are still the kept ones, read again at the
		// next wake.
		m.workflows.kept = true
	}
	if msg.err == nil {
		m.workflows.items, m.workflows.loaded, m.workflows.kept = msg.items, true, msg.kept
	}
	var again tea.Cmd
	if msg.err == nil && msg.stale {
		again = m.readWorkflows(true)
	}
	if m.filterStep == nil || m.filterStep.form != nil {
		return again
	}
	return tea.Batch(m.showForm(), again)
}

// rereadWorkflows reads the workflows again, if they were served kept,
// now that GitHub answers again.
func (m *Modal) rereadWorkflows() tea.Cmd {
	w := &m.workflows
	if !w.kept || w.loading {
		return nil
	}
	w.kept, w.loading = false, true
	return m.readWorkflows(false)
}

// showForm shows the form of the filter, on the filter shown.
func (m *Modal) showForm() tea.Cmd {
	spec := filterform.Spec{Fields: []filterform.Field{
		{Key: fieldWorkflow, Label: "Workflow", Kind: filterform.Choice, Qualifier: "workflow", Options: m.workflowItems()},
		{Key: fieldBranch, Label: "Branch", Kind: filterform.Text, Qualifier: "branch", Hint: "any branch"},
		{Key: fieldEvent, Label: "Event", Kind: filterform.Choice, Qualifier: "event", Options: eventItems()},
		{Key: fieldStatus, Label: "Status", Kind: filterform.Choice, Qualifier: "status", Options: statuses},
		{Key: fieldActor, Label: "Actor", Kind: filterform.Person, Qualifier: "actor", Hint: "anyone", Options: m.actorItems()},
	}}
	f := filterform.New(spec,
		filterform.WithQuery(queryOf(m.filter, m.workflows.items)),
		filterform.WithContext(m.ctx),
		filterform.WithKeyNames(m.opts.icons.Key),
		filterform.WithKeyMap(m.keys.form),
		filterform.WithStyles(m.theme.FilterForm(m.opts.icons)),
		filterform.WithSize(m.width, m.bodyHeight()),
	)
	m.filterStep.form, m.filterStep.rows = &f, len(spec.Fields)
	return f.Focus()
}

func (m *Modal) workflowItems() []filterform.Item {
	items := make([]filterform.Item, 0, len(m.workflows.items)+1)
	items = append(items, filterform.Item{Label: "Any"})
	for _, w := range m.workflows.items {
		if name := ui.OneLine(w.Name); name != "" {
			items = append(items, filterform.Item{Label: name, Value: name})
		}
	}
	return items
}

func eventItems() []filterform.Item {
	items := make([]filterform.Item, 0, len(events)+1)
	items = append(items, filterform.Item{Label: "Any"})
	for _, e := range events {
		items = append(items, filterform.Item{Label: e, Value: e})
	}
	return items
}

func (m *Modal) actorItems() []filterform.Item {
	if m.opts.viewer == nil {
		return nil
	}
	return []filterform.Item{{Label: me, Value: me, Detail: "you"}}
}

// Fit implements ui.Fitter. The filter step is as large as its form needs,
// as any filter is; the rest of the modal takes the room it is given.
func (m *Modal) Fit(maxWidth, maxHeight int) (width, height int) {
	if f := m.filterStep; f != nil && f.form != nil {
		// The form takes the lines under the breadcrumb.
		width = min(maxWidth, ui.FilterWidth)
		return width, min(maxHeight, 1+ui.FilterHeight(f.rows, width, f.form))
	}
	return maxWidth, maxHeight
}

// closeFilter closes the filter step. The panes kept their size while the
// form showed, so they take the size of the modal now. The jobs stay
// scrolled as they were: the modal may not have its own size back yet, and
// scrolls to it when it does.
func (m *Modal) closeFilter() {
	m.filterStep = nil
	m.sizePanes()
}

// updateFilter passes msg to the form, and applies or closes it when it
// says so.
func (m *Modal) updateFilter(msg tea.Msg) tea.Cmd {
	f := m.filterStep.form
	if f == nil {
		return nil
	}
	switch msg := msg.(type) {
	case filterform.AppliedMsg:
		if msg.ID != f.ID() {
			return nil
		}
		m.closeFilter()
		return m.setFilter(m.filterOf(msg.Values))
	case filterform.CancelMsg:
		if msg.ID == f.ID() {
			m.closeFilter()
		}
		return nil
	}
	var cmd tea.Cmd
	*f, cmd = f.Update(msg)
	return cmd
}

// filterOf returns the filter that the values of the form make.
func (m *Modal) filterOf(v map[string]filterform.Value) core.RunFilter {
	f := core.RunFilter{
		Branch: strings.TrimSpace(v[fieldBranch].Text()),
		Event:  v[fieldEvent].Text(),
		Status: v[fieldStatus].Text(),
		Actor:  strings.TrimSpace(v[fieldActor].Text()),
	}
	if f.Actor != me {
		f.Actor = strings.TrimPrefix(f.Actor, "@")
	}
	if name := v[fieldWorkflow].Text(); name != "" {
		for _, w := range m.workflows.items {
			if strings.EqualFold(ui.OneLine(w.Name), name) {
				f.WorkflowID = w.ID
				break
			}
		}
	}
	// The form has no field for the head SHA, so it stays.
	f.HeadSHA = m.filter.HeadSHA
	return f
}

// queryOf writes f as the form's query, such as
// "workflow:CI branch:main status:failure actor:@me".
func queryOf(f core.RunFilter, wfs []core.Workflow) string {
	var parts []string
	add := func(q, v string) {
		if v == "" {
			return
		}
		if strings.ContainsAny(v, " \t,") {
			v = `"` + strings.ReplaceAll(v, `"`, "") + `"`
		}
		parts = append(parts, q+":"+v)
	}
	if f.WorkflowID != 0 {
		for _, w := range wfs {
			if w.ID == f.WorkflowID {
				add("workflow", ui.OneLine(w.Name))
			}
		}
	}
	add("branch", f.Branch)
	add("event", f.Event)
	add("status", f.Status)
	add("actor", f.Actor)
	return strings.Join(parts, " ")
}

// filterLines renders the filter step, h lines of w cells.
func (m *Modal) filterLines(w, h int) []string {
	f := m.filterStep.form
	if f == nil {
		return ui.FitLines([]string{m.spin.View() + m.st.Muted.Render("Loading the workflows"+m.st.ic.Ellipsis)}, w, h)
	}
	return ui.FitLines(strings.Split(f.View(), "\n"), w, h)
}
