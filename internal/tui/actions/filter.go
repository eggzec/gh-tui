package actions

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// workflows are the workflows of the repository, read the first time the
// filter opens.
type workflows struct {
	items           []core.Workflow
	loaded, loading bool
	// wanted reports that the filter waits for the workflows, which its
	// workflow field chooses from, to open.
	wanted bool
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
	{Label: "Failure", Value: "failure"},
	{Label: "Success", Value: "success"},
	{Label: "In progress", Value: "in_progress"},
	{Label: "Queued", Value: "queued"},
	{Label: "Cancelled", Value: "cancelled"},
	{Label: "Action required", Value: "action_required"},
	{Label: "Timed out", Value: "timed_out"},
}

// openFilter opens the filter over the modal, once the workflows are read.
func (m *Modal) openFilter() tea.Cmd {
	if m.workflows.loaded {
		return ui.OpenModal(m.filterModal())
	}
	m.workflows.wanted = true
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
// them. When the read failed the form still opens, without workflows to
// choose from, and a read later tries again. Workflows an earlier session
// kept are shown, and read again at once.
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
	if !m.workflows.wanted {
		return again
	}
	m.workflows.wanted = false
	return tea.Batch(ui.OpenModal(m.filterModal()), again)
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

// filterModal returns the filter over the modal, on the filter shown.
func (m *Modal) filterModal() ui.Modal {
	f, _ := m.Filter()
	return ui.NewFilterModal(m.ctx, Title, m, f,
		ui.WithFormKeys(m.keys.form),
		ui.WithFormBack(m.keys.Back),
		ui.WithFormVoice(*m.opts.voice),
		ui.WithFormIcons(m.opts.icons),
	)
}

// Filter implements ui.Filterable: the form opens on the filter shown.
func (m *Modal) Filter() (ui.Filter, bool) {
	return ui.Filter{
		Spec:    m.spec(),
		Query:   queryOf(m.toggle.Edited(m.filter), m.workflows.items),
		Subject: m.repo.String(),
	}, true
}

// ApplyFilter implements ui.Filterable. The qualifiers no field claims join
// the words of the filter.
func (m *Modal) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	var free []string
	for _, tok := range filterform.Tokenize(msg.Query) {
		switch tok.Qualifier {
		case "", fieldWorkflow, fieldBranch, fieldEvent, fieldStatus, fieldActor:
		default:
			free = append(free, tok.Raw)
		}
	}
	m.toggle.On()
	return m.setFilter(m.filterOf(msg.Values, free))
}

// spec is the form of the filter.
func (m *Modal) spec() filterform.Spec {
	f := m.toggle.Edited(m.filter)
	return filterform.Spec{Fields: []filterform.Field{
		ui.SearchField,
		ui.MultiField(fieldWorkflow, "Workflow", ui.WithValues(m.workflowItems(), m.namesOf(f))),
		ui.MultiField(fieldBranch, "Branch", ui.WithValues(m.branchItems(), chosen(f.Branch, f.Branches))),
		ui.MultiField(fieldEvent, "Event", ui.WithValues(eventItems(), chosen(f.Event, f.Events))),
		ui.MultiField(fieldStatus, "Status", ui.WithValues(slices.Clone(statuses), chosen(f.Status, f.Statuses))),
		ui.MultiField(fieldActor, "Actor", ui.WithValues(m.actorItems(), chosen(f.Actor, f.Actors))),
	}}
}

// namesOf returns the names of the workflows f selects.
func (m *Modal) namesOf(f core.RunFilter) []string {
	ids := f.Workflows.List()
	if f.WorkflowID != 0 {
		ids = []string{strconv.FormatInt(f.WorkflowID, 10)}
	}
	var names []string
	for _, id := range ids {
		for _, w := range m.workflows.items {
			if strconv.FormatInt(w.ID, 10) == id {
				names = append(names, ui.OneLine(w.Name))
			}
		}
	}
	return names
}

func (m *Modal) workflowItems() []filterform.Item {
	items := make([]filterform.Item, 0, len(m.workflows.items))
	for _, w := range m.workflows.items {
		if name := ui.OneLine(w.Name); name != "" {
			items = append(items, filterform.Item{Label: name, Value: name})
		}
	}
	return items
}

func eventItems() []filterform.Item {
	items := make([]filterform.Item, 0, len(events))
	for _, e := range events {
		items = append(items, filterform.Item{Label: e, Value: e})
	}
	return items
}

// branchItems offers the branches of the runs loaded.
func (m *Modal) branchItems() []filterform.Item {
	return m.runItems(func(r core.Run) string { return r.Branch }, nil)
}

// actorItems offers the signed-in user, and the actors of the runs loaded.
func (m *Modal) actorItems() []filterform.Item {
	var first []filterform.Item
	if m.opts.viewer != nil {
		first = []filterform.Item{{Label: me, Value: me, Detail: "you"}}
	}
	return m.runItems(func(r core.Run) string { return r.Actor }, first)
}

// runItems lists, after first, the distinct values of the runs loaded, in
// the order of the runs.
func (m *Modal) runItems(of func(core.Run) string, first []filterform.Item) []filterform.Item {
	items := first
	seen := map[string]bool{}
	for i := range m.runs.Len() {
		r, ok := m.runs.Item(i)
		if !ok {
			continue
		}
		v := of(r)
		if k := strings.ToLower(v); v != "" && !seen[k] {
			seen[k] = true
			items = append(items, filterform.Item{Label: v, Value: v})
		}
	}
	return items
}

// filterOf returns the filter that the values of the form make. The words no
// field claims, such as a qualifier the runs have none for, join the words
// of the search row as quoted phrases, so that they show in the title and cut
// the runs.
func (m *Modal) filterOf(v map[string]filterform.Value, free []string) core.RunFilter {
	words := make([]string, 1, 1+len(free))
	words[0] = v[ui.SearchField.Key].Text()
	for _, tok := range free {
		// A quoted token reads as a plain word, not as a qualifier that the
		// words would skip.
		words = append(words, `"`+strings.ReplaceAll(tok, `"`, "")+`"`)
	}
	f := core.RunFilter{Words: strings.TrimSpace(strings.Join(words, " "))}
	f.Branch, f.Branches = picked(v[fieldBranch].List(), nil)
	f.Event, f.Events = picked(v[fieldEvent].List(), nil)
	f.Status, f.Statuses = picked(v[fieldStatus].List(), nil)
	f.Actor, f.Actors = picked(v[fieldActor].List(), func(a string) string {
		if a == me {
			return a
		}
		return strings.TrimPrefix(a, "@")
	})
	var ids []string
	for _, name := range v[fieldWorkflow].List() {
		for _, w := range m.workflows.items {
			if strings.EqualFold(ui.OneLine(w.Name), strings.TrimSpace(name)) {
				ids = append(ids, strconv.FormatInt(w.ID, 10))
				break
			}
		}
	}
	id, many := picked(ids, nil)
	f.Workflows = many
	f.WorkflowID, _ = strconv.ParseInt(id, 10, 64)
	// The form has no field for the head SHA, so it stays.
	f.HeadSHA = m.filter.HeadSHA
	return f
}

// picked splits the values chosen for a field: GitHub is asked for a single
// one, and several are matched on the runs loaded, so they are not sent.
func picked(vs []string, clean func(string) string) (one string, many core.AnyOf) {
	if clean != nil {
		vs = slices.Clone(vs)
		for i, v := range vs {
			vs[i] = clean(strings.TrimSpace(v))
		}
	}
	list := core.AnyOfValues(vs...).List()
	if len(list) == 1 {
		return list[0], ""
	}
	return "", core.AnyOfValues(list...)
}

// chosen returns the values of a field, the single one and the several.
func chosen(one string, many core.AnyOf) []string {
	if one != "" {
		return []string{one}
	}
	return many.List()
}

// queryOf writes f as the form's query, such as
// "workflow:CI branch:main status:failure actor:@me", with a qualifier for
// each value of a field of several.
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
	ids := f.Workflows.List()
	if f.WorkflowID != 0 {
		ids = []string{strconv.FormatInt(f.WorkflowID, 10)}
	}
	for _, id := range ids {
		for _, w := range wfs {
			if strconv.FormatInt(w.ID, 10) == id {
				add("workflow", ui.OneLine(w.Name))
			}
		}
	}
	for _, e := range []struct {
		q    string
		one  string
		many core.AnyOf
	}{
		{"branch", f.Branch, f.Branches},
		{"event", f.Event, f.Events},
		{"status", f.Status, f.Statuses},
		{"actor", f.Actor, f.Actors},
	} {
		for _, v := range chosen(e.one, e.many) {
			add(e.q, v)
		}
	}
	if f.Words != "" {
		parts = append(parts, f.Words)
	}
	return strings.Join(parts, " ")
}
