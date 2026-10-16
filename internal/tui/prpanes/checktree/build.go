package checktree

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// otherChecks titles the group of check runs that no App reported.
const otherChecks = "Other checks"

// CommitIcon returns the glyph that marks a commit whose checks GitHub
// rolled up to s, before they are read: "" when there are none, and then the
// host draws no fold on the commit.
func CommitIcon(s core.ChecksState, st Styles) string {
	switch s {
	case core.ChecksFailure:
		return st.icon(core.CheckFailing)
	case core.ChecksPending:
		return st.icon(core.CheckPending)
	case core.ChecksSuccess:
		return st.icon(core.CheckPassing)
	case core.ChecksNone:
	}
	return ""
}

// StateOf returns the worst state of any check run or status of c, and
// [core.CheckNone] when it has none.
func StateOf(c core.CommitChecks) core.CheckState {
	w := core.CheckNone
	for i := range c.Runs {
		w = core.Worst(w, c.Runs[i].Outcome())
	}
	for i := range c.Statuses {
		w = core.Worst(w, c.Statuses[i].Outcome())
	}
	return w
}

// Rows returns the rows of the checks of c, which are open folds, and what
// enter on each of them asks for. It returns none when c has no check run
// and no status. The rows share nothing with c.
func Rows(c core.CommitChecks, o Options, st Styles) ([]foldtree.Row, Targets) {
	if len(c.Runs) == 0 && len(c.Statuses) == 0 {
		return nil, nil
	}
	b := &builder{c: c, o: o, st: &st, targets: Targets{}}
	b.build()
	return b.rows, b.targets
}

// builder makes the rows of one commit.
type builder struct {
	c       core.CommitChecks
	o       Options
	st      *Styles
	rows    []foldtree.Row
	targets Targets
}

// workflow is the jobs of one workflow run, and app the check runs of one
// App, in the order GitHub lists them.
type workflow struct {
	id     int64
	checks []core.CommitCheck
}

type app struct {
	slug   string
	name   string
	checks []core.CommitCheck
}

func (b *builder) build() {
	var (
		flows []*workflow
		apps  []*app
		other []core.CommitCheck
	)
	flowAt, appAt := map[int64]*workflow{}, map[string]*app{}
	for i := range b.c.Runs {
		cc := &b.c.Runs[i]
		switch {
		case cc.InActions():
			w := flowAt[cc.RunID]
			if w == nil {
				w = &workflow{id: cc.RunID}
				flowAt[cc.RunID] = w
				flows = append(flows, w)
			}
			w.checks = append(w.checks, *cc)
		case cc.App == nil:
			other = append(other, *cc)
		default:
			slug := cc.App.Slug
			if slug == "" {
				slug = cc.App.Name
			}
			a := appAt[slug]
			if a == nil {
				a = &app{slug: slug, name: cc.App.Name}
				appAt[slug] = a
				apps = append(apps, a)
			}
			a.checks = append(a.checks, *cc)
		}
	}
	for _, w := range flows {
		b.workflow(w)
	}
	for _, a := range apps {
		b.group("app/"+a.slug, b.st.app, a.name, a.checks)
	}
	if len(other) > 0 {
		b.group("other", "", otherChecks, other)
	}
	for _, s := range b.c.Statuses {
		b.status(s)
	}
	if b.c.Truncated {
		b.truncated()
	}
}

func (b *builder) id(kind string, n int64) string {
	return b.o.Prefix + kind + "/" + strconv.FormatInt(n, 10)
}

// workflow adds a workflow run and its jobs.
func (b *builder) workflow(w *workflow) {
	jobs := make([]core.Job, len(w.checks))
	states := make([]core.CheckState, len(w.checks))
	var name, url string
	var number int
	for i := range w.checks {
		cc := &w.checks[i]
		jobs[i] = jobOf(cc)
		states[i] = jobs[i].Outcome()
		if name == "" {
			name = cc.Workflow
		}
		if url == "" {
			url = cc.RunURL
		}
		if number == 0 {
			number = cc.RunNumber
		}
	}
	kids := b.jobs(core.GroupJobs(jobs), jobs, b.o.Depth+1)
	if len(kids) == 0 {
		return
	}
	state := core.Worst(states...)
	if name == "" {
		name = "Workflow run"
	}
	text := b.line(b.st.icon(state), b.st.actions, b.st.link(url, name))
	if number > 0 {
		text += b.dim(" #" + strconv.Itoa(number))
	}
	b.rows = append(b.rows, b.fold(b.id("run", w.id), b.o.Depth, text, b.dim(stateWord(state))))
	b.rows = append(b.rows, kids...)
}

// jobOf returns the job that check run cc reports, for grouping.
func jobOf(cc *core.CommitCheck) core.Job {
	id := cc.JobID
	if id == 0 {
		id = cc.ID
	}
	return core.Job{
		ID:           id,
		RunID:        cc.RunID,
		Name:         cc.Name,
		WorkflowName: cc.Workflow,
		Status:       cc.Status,
		Conclusion:   cc.Conclusion,
		StartedAt:    cc.StartedAt,
		CompletedAt:  cc.CompletedAt,
		URL:          cc.DetailsURL,
	}
}

// jobs returns the rows of nodes at depth.
func (b *builder) jobs(nodes []core.JobNode, jobs []core.Job, depth int) []foldtree.Row {
	var rows []foldtree.Row
	for _, n := range nodes {
		if n.Group == nil {
			rows = append(rows, b.job(jobs[n.Job], n.Label, depth))
			continue
		}
		g := n.Group
		kids := b.jobs(g.Kids, jobs, depth+1)
		if len(kids) == 0 {
			continue
		}
		text := b.line(b.st.icon(g.State), b.st.link("", n.Label))
		id := b.o.Prefix + "grp/" + strconv.FormatInt(jobs[g.Jobs[0]].RunID, 10) + "/" + g.Key
		right := b.dim(b.groupWord(g, jobs))
		rows = append(rows, b.fold(id, depth, text, right))
		rows = append(rows, kids...)
	}
	return rows
}

// groupWord says how many jobs of a group of a caller or a matrix fail, or
// run, or that they all pass.
func (b *builder) groupWord(g *core.JobGroup, jobs []core.Job) string {
	var failing, running int
	for _, i := range g.Jobs {
		switch jobs[i].Outcome() {
		case core.CheckFailing:
			failing++
		case core.CheckPending:
			running++
		case core.CheckNone, core.CheckPassing:
		}
	}
	n := len(g.Jobs)
	switch {
	case failing > 0:
		return fmt.Sprintf("%d of %d failing", failing, n)
	case running > 0:
		return fmt.Sprintf("%d of %d running", running, n)
	}
	return fmt.Sprintf("%d passing", n)
}

// job returns the row of a job or a leg, whose enter opens the job.
func (b *builder) job(j core.Job, label string, depth int) foldtree.Row {
	state := j.Outcome()
	id := b.id("job", j.ID)
	b.targets[id] = OpenJobMsg{SHA: b.c.SHA, RunID: j.RunID, JobID: j.ID}
	return foldtree.Row{
		ID:     id,
		Depth:  depth,
		Lines:  []string{b.line(b.st.icon(state), b.st.link(j.URL, label))},
		Right:  b.dim(b.took(state, j.Status, j.StartedAt, j.CompletedAt)),
		Action: true,
	}
}

// took tells how a job or check run stands and how long it took, or has
// taken so far: "failed · 1m 5s", and just "running" when it has not started.
func (b *builder) took(state core.CheckState, status core.RunStatus, start, end time.Time) string {
	word := stateWord(state)
	if status == core.RunInProgress {
		word = "running"
	}
	if d, ok := ui.Span(start, end, b.o.Now); ok {
		return word + b.st.Glyphs.Separator + ui.Duration(d)
	}
	return word
}

// group adds the group of the check runs of one App, or of none, and its
// runs. glyph marks the kind of the group, and is "" for none.
func (b *builder) group(id, glyph, title string, checks []core.CommitCheck) {
	states := make([]core.CheckState, len(checks))
	kids := make([]foldtree.Row, len(checks))
	for i := range checks {
		states[i] = checks[i].Outcome()
		kids[i] = b.check(&checks[i], states[i])
	}
	state := core.Worst(states...)
	text := b.line(b.st.icon(state), glyph, b.st.link("", title))
	b.rows = append(b.rows, b.fold(b.o.Prefix+id, b.o.Depth, text, b.dim(stateWord(state))))
	b.rows = append(b.rows, kids...)
}

// check returns the row of a check run of an App, whose enter opens the run.
func (b *builder) check(cc *core.CommitCheck, state core.CheckState) foldtree.Row {
	id := b.id("check", cc.ID)
	b.targets[id] = OpenCheckMsg{SHA: b.c.SHA, CheckID: cc.ID, SuiteID: cc.SuiteID}
	right := b.dim(b.took(state, cc.Status, cc.StartedAt, cc.CompletedAt))
	if cc.Annotations > 0 {
		noun := "findings"
		if cc.Annotations == 1 {
			noun = "finding"
		}
		word := strconv.Itoa(cc.Annotations) + " " + noun + b.st.Glyphs.Separator + stateWord(state)
		right = b.line(b.noteGlyph(cc.Notes), b.dim(word))
	}
	return foldtree.Row{
		ID:     id,
		Depth:  b.o.Depth + 1,
		Lines:  []string{b.line(b.st.icon(state), b.st.link(cc.DetailsURL, cc.Name))},
		Right:  right,
		Action: true,
	}
}

// noteGlyph returns the glyph of the worst level of notes, and "" when there
// are none, as when the annotations were not read.
func (b *builder) noteGlyph(notes []core.Annotation) string {
	glyph := ""
	for _, n := range notes {
		switch n.Level {
		case core.AnnotationFailure:
			return b.st.failure
		case core.AnnotationWarning:
			glyph = b.st.warning
		default:
			if glyph == "" {
				glyph = b.st.notice
			}
		}
	}
	return glyph
}

// status adds a commit status, which enter does nothing on.
func (b *builder) status(s core.StatusContext) {
	state := s.Outcome()
	text := b.line(b.st.icon(state), b.st.status, b.st.link(s.TargetURL, s.Context))
	b.rows = append(b.rows, foldtree.Row{
		ID:    b.o.Prefix + "status/" + s.Context,
		Depth: b.o.Depth,
		Lines: []string{text},
		Right: b.dim(termtext.OneLine(s.Description)),
	})
}

// truncated adds the note that GitHub lists more checks than were read.
func (b *builder) truncated() {
	more := b.c.Total - len(b.c.Runs) - len(b.c.Statuses)
	text := "More checks not shown"
	if more > 0 {
		text = strconv.Itoa(more) + " more checks not shown"
	}
	b.rows = append(b.rows, foldtree.Row{
		ID:    b.o.Prefix + "more",
		Depth: b.o.Depth,
		Lines: []string{b.dim(text)},
		Skip:  true,
	})
}

// fold returns an open fold row.
func (b *builder) fold(id string, depth int, text, right string) foldtree.Row {
	return foldtree.Row{ID: id, Depth: depth, Lines: []string{text}, Right: right, Fold: true, Arrow: true}
}

// line joins the parts of a row with a space, leaving out those that are
// empty.
func (b *builder) line(parts ...string) string {
	var s strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if s.Len() > 0 {
			s.WriteByte(' ')
		}
		s.WriteString(p)
	}
	return s.String()
}

// dim draws s dim, and leaves an empty one empty.
func (b *builder) dim(s string) string {
	if s == "" {
		return ""
	}
	return b.st.dim.Render(s)
}

// stateWord says how a check stands.
func stateWord(s core.CheckState) string {
	switch s {
	case core.CheckFailing:
		return "failed"
	case core.CheckPending:
		return "pending"
	case core.CheckPassing:
		return "passed"
	case core.CheckNone:
	}
	return ""
}
