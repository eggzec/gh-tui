package actions

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// filterStep is the filter of the runs, a step of the modal. It waits for
// the workflows, which its first field chooses from, before it shows the
// form.
type filterStep struct {
	form *filterform.Model
}

// workflows are the workflows of the repository, read the first time the
// filter opens.
type workflows struct {
	items           []core.Workflow
	loaded, loading bool
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
	return tea.Batch(m.readWorkflows(), m.startSpinner())
}

// workflowsMsg carries the workflows of the repository.
type workflowsMsg struct {
	id    int64
	items []core.Workflow
	err   error
}

func (m *Modal) readWorkflows() tea.Cmd {
	svc, ctx, id, repo, off := m.svc, m.ctx, m.id, m.repo, m.opts.offline
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.workflows")
		p, err := svc.Workflows(ctx, repo)
		end(err, "span", "tui", "workflows", len(p.Items), "stale", p.Stale, "offline", p.Offline)
		if p.Offline {
			off.Mark()
		}
		return workflowsMsg{id: id, items: p.Items, err: err}
	}
}

// receiveWorkflows keeps the workflows, and shows the form that waited for
// them. Without them, the form has no workflows to choose from, and a read
// later tries again.
func (m *Modal) receiveWorkflows(msg workflowsMsg) tea.Cmd {
	m.workflows.loading = false
	if msg.err == nil {
		m.workflows.items, m.workflows.loaded = msg.items, true
	}
	if m.filterStep == nil || m.filterStep.form != nil {
		return nil
	}
	return m.showForm()
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
		filterform.WithHelpLine(false),
		filterform.WithStyles(m.theme.FilterForm()),
		filterform.WithSize(m.width, m.bodyHeight()),
	)
	m.filterStep.form = &f
	return f.Focus()
}

func (m *Modal) workflowItems() []filterform.Item {
	items := make([]filterform.Item, 0, len(m.workflows.items)+1)
	items = append(items, filterform.Item{Label: "Any"})
	for _, w := range m.workflows.items {
		if name := oneLine(w.Name); name != "" {
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
		m.filterStep = nil
		return m.setFilter(m.filterOf(msg.Values))
	case filterform.CancelMsg:
		if msg.ID == f.ID() {
			m.filterStep = nil
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
			if strings.EqualFold(oneLine(w.Name), name) {
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
				add("workflow", oneLine(w.Name))
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
		return fitLines([]string{m.spin.View() + m.st.muted.Render("Loading the workflows…")}, w, h)
	}
	return fitLines(strings.Split(f.View(), "\n"), w, h)
}
