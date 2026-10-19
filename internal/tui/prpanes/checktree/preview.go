package checktree

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// shortSHA is how many characters of a commit's SHA a preview names it by.
const shortSHA = 7

// Preview is what a row of the checks has to show beside the tree. Title and
// Info are plain text, for the frame to style. Body is empty for a row that has
// no preview yet, and the host then shows a placeholder under the title.
type Preview struct {
	Title, Info, Body string
}

// PreviewOf returns the preview of the row with the ID id of the checks c, as
// [Rows] and [ChecksNode] name them. Now, Prefix and Width of o are read: the
// lines are cut to the width. ok is false for an ID that no row of c has.
func PreviewOf(c core.CommitChecks, id string, o Options, st Styles) (p Preview, ok bool) {
	rest, ok := strings.CutPrefix(id, o.Prefix)
	if !ok {
		return Preview{}, false
	}
	b := &builder{c: c, o: o, st: &st}
	flows, apps, other := b.collect()
	kind, key, _ := strings.Cut(rest, "/")
	switch kind {
	case "checks":
		return b.checksPreview(flows, b.groups(apps, other))
	case "run":
		for _, w := range flows {
			if strconv.FormatInt(w.id, 10) == key {
				return b.runPreview(w), true
			}
		}
	case "grp":
		runID, groupKey, _ := strings.Cut(key, "/")
		for _, w := range flows {
			if strconv.FormatInt(w.id, 10) == runID {
				return b.groupPreview(w, groupKey)
			}
		}
	case "app":
		for _, a := range b.groups(apps, nil) {
			if a.key == key {
				return b.appPreview(a), true
			}
		}
	case "other":
		if len(other) > 0 {
			return b.appPreview(named{name: otherChecks, checks: other}), true
		}
	case "status":
		for _, s := range c.Statuses {
			if s.Context == key {
				return b.statusPreview(s), true
			}
		}
	case "job", "check":
		// The body is the host's: the log of a job, the page of a check run.
		for i := range c.Runs {
			cc := &c.Runs[i]
			n := cc.ID
			if kind == "job" && cc.JobID != 0 {
				n = cc.JobID
			}
			if strconv.FormatInt(n, 10) != key {
				continue
			}
			if kind == "check" {
				return b.checkPreview(cc), true
			}
			return Preview{Title: termtext.OneLine(cc.Name)}, true
		}
	}
	return Preview{}, false
}

// named is a group of the check runs of one App, or of none.
type named struct {
	key, name, glyph string
	checks           []core.CommitCheck
}

// groups returns the runs of each App and then those of none.
func (b *builder) groups(apps []*app, other []core.CommitCheck) []named {
	out := make([]named, 0, len(apps)+1)
	for _, a := range apps {
		out = append(out, named{key: a.slug, name: a.name, glyph: b.st.app, checks: a.checks})
	}
	if len(other) > 0 {
		out = append(out, named{name: otherChecks, checks: other})
	}
	return out
}

// worst is the worst outcome of checks.
func worst(checks []core.CommitCheck) core.CheckState {
	w := core.CheckNone
	for i := range checks {
		w = core.Worst(w, checks[i].Outcome())
	}
	return w
}

// cut cuts s to the width with an ellipsis.
func (b *builder) cut(s string) string {
	if b.o.Width <= 0 || ansi.StringWidth(s) <= b.o.Width {
		return s
	}
	return termtext.Truncate(s, b.o.Width, b.st.Glyphs.Ellipsis)
}

// entry is a line of left, indented, and right dim after it.
func (b *builder) entry(indent int, left, right string) string {
	s := strings.Repeat(" ", indent) + left
	if right != "" {
		s += "  " + b.dim(right)
	}
	return b.cut(s)
}

// note is text wrapped to the width and drawn dim.
func (b *builder) note(text string) []string {
	wrapped := wrap(text, b.o.Width)
	out := make([]string, 0, strings.Count(wrapped, "\n")+1)
	for l := range strings.SplitSeq(wrapped, "\n") {
		out = append(out, b.dim(l))
	}
	return out
}

// wrap wraps text to width, or leaves it when there is no width.
func wrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.Wrap(text, width, "")
}

// word is the word of state s in its color.
func (b *builder) word(s core.CheckState) string {
	if s < 0 || int(s) >= len(b.st.tone) {
		return stateWord(s)
	}
	return b.st.tone[s].Render(stateWord(s))
}

// run is the line of a check run: its mark, its name as a link, and how it
// stands and how long it took.
func (b *builder) run(indent int, cc *core.CommitCheck, label string) string {
	state := cc.Outcome()
	left := b.line(b.st.icon(state), b.st.link(cc.DetailsURL, label))
	return b.entry(indent, left, b.took(state, cc.Status, cc.StartedAt, cc.CompletedAt))
}

// runName is the name of a workflow run, as its row shows it.
func runName(w *workflow) string {
	name, _, _ := w.meta()
	if name == "" {
		return "Workflow run"
	}
	return termtext.OneLine(name)
}

// runWord is the number of a run and how it stands: "#12 · failed".
func (b *builder) runWord(w *workflow) string {
	_, _, n := w.meta()
	word := stateWord(worst(w.checks))
	if n == 0 {
		return word
	}
	return "#" + strconv.Itoa(n) + b.st.Glyphs.Separator + word
}

// checksPreview lists what fails or runs of the commit: the runs and their
// jobs, the Apps and their runs, and the statuses.
func (b *builder) checksPreview(flows []*workflow, groups []named) (Preview, bool) {
	n := ChecksNode(b.c, b.o, *b.st)
	if n.ID == "" {
		return Preview{}, false
	}
	var out []string
	for _, w := range flows {
		if worst(w.checks) == core.CheckPassing {
			continue
		}
		head := b.line(b.st.icon(worst(w.checks)), b.st.bold.Render(runName(w)))
		out = append(out, b.entry(0, head, b.runWord(w)))
		for i := range w.checks {
			if cc := &w.checks[i]; cc.Outcome() != core.CheckPassing {
				out = append(out, b.run(2, cc, cc.Name))
			}
		}
		out = append(out, "")
	}
	for _, g := range groups {
		if worst(g.checks) == core.CheckPassing {
			continue
		}
		out = append(out, b.line(b.st.icon(worst(g.checks)), g.glyph, b.st.bold.Render(termtext.OneLine(g.name))))
		for i := range g.checks {
			cc := &g.checks[i]
			if cc.Outcome() != core.CheckPassing {
				out = append(out, b.entry(2, b.line(b.st.icon(cc.Outcome()), b.st.link(cc.DetailsURL, cc.Name)), termtext.OneLine(cc.Title)))
			}
		}
		out = append(out, "")
	}
	for _, s := range b.c.Statuses {
		if state := s.Outcome(); state != core.CheckPassing {
			left := b.line(b.st.icon(state), b.st.status, b.st.link(s.TargetURL, s.Context))
			out = append(out, b.entry(0, left, termtext.OneLine(s.Description)))
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	out = append(out, "")
	out = append(out, b.note("enter opens a job's log or a check run's page; statuses only show here")...)
	return Preview{
		Title: ansi.Strip(n.Lines[0]),
		Info:  "failing runs of the head commit " + short(b.c.SHA),
		Body:  strings.Join(out, "\n"),
	}, true
}

// runPreview lists every job of a workflow run.
func (b *builder) runPreview(w *workflow) Preview {
	title := runName(w)
	if _, _, n := w.meta(); n > 0 {
		title += b.st.Glyphs.Separator + "run #" + strconv.Itoa(n)
	}
	on := "on " + short(b.c.SHA)
	for i := range w.checks {
		if e := w.checks[i].Event; e != "" {
			on = "on " + termtext.OneLine(e) + b.st.Glyphs.Separator + short(b.c.SHA)
			break
		}
	}
	out := b.note(on)
	out = append(out, "")
	for i := range w.checks {
		out = append(out, b.run(0, &w.checks[i], w.checks[i].Name))
	}
	out = append(out, "")
	out = append(out, b.note("enter folds the run; enter on a job opens its log")...)
	return Preview{Title: title, Info: stateWord(worst(w.checks)), Body: strings.Join(out, "\n")}
}

// groupPreview lists every leg of a matrix, or job of a reusable workflow, of
// a run.
func (b *builder) groupPreview(w *workflow, key string) (Preview, bool) {
	jobs := make([]core.Job, len(w.checks))
	for i := range w.checks {
		jobs[i] = jobOf(&w.checks[i])
	}
	n, ok := findGroup(core.GroupJobs(jobs), key)
	if !ok {
		return Preview{}, false
	}
	g := n.Group
	noun := " jobs of the reusable workflow"
	if strings.HasSuffix(g.Key, " (") {
		noun = " legs of the matrix"
	}
	out := b.note(strconv.Itoa(len(g.Jobs)) + noun)
	out = append(out, "")
	var legs func(nodes []core.JobNode, prefix string)
	legs = func(nodes []core.JobNode, prefix string) {
		for _, k := range nodes {
			if k.Group != nil {
				legs(k.Group.Kids, prefix+k.Label+" / ")
				continue
			}
			out = append(out, b.run(0, &w.checks[k.Job], prefix+k.Label))
		}
	}
	legs(g.Kids, "")
	out = append(out, "")
	out = append(out, b.note("enter folds the group; enter on a leg opens its log")...)
	return Preview{
		Title: runName(w) + b.st.Glyphs.Separator + termtext.OneLine(n.Label),
		Info:  b.groupWord(g, jobs),
		Body:  strings.Join(out, "\n"),
	}, true
}

// findGroup returns the group with the key key, at any depth of nodes.
func findGroup(nodes []core.JobNode, key string) (core.JobNode, bool) {
	for _, n := range nodes {
		if n.Group == nil {
			continue
		}
		if n.Group.Key == key {
			return n, true
		}
		if m, ok := findGroup(n.Group.Kids, key); ok {
			return m, true
		}
	}
	return core.JobNode{}, false
}

// checkPreview is the frame of the page of a check run of an App: the App and
// the run in the title, and how it stands and how long it took in the info. The
// host draws the page.
func (b *builder) checkPreview(cc *core.CommitCheck) Preview {
	title := termtext.OneLine(cc.Name)
	if cc.App != nil && cc.App.Name != "" {
		title = termtext.OneLine(cc.App.Name) + b.st.Glyphs.Separator + title
	}
	return Preview{Title: title, Info: b.took(cc.Outcome(), cc.Status, cc.StartedAt, cc.CompletedAt)}
}

// appPreview lists every check run of an App, with the title each reported.
func (b *builder) appPreview(g named) Preview {
	out := b.note("check runs on " + short(b.c.SHA))
	for i := range g.checks {
		cc := &g.checks[i]
		out = append(out, b.run(0, cc, cc.Name))
		if t := termtext.OneLine(cc.Title); t != "" {
			out = append(out, b.entry(2, b.dim(t), ""))
		}
	}
	out = append(out, "")
	out = append(out, b.note("enter folds the group; enter on a run opens its page")...)
	return Preview{Title: termtext.OneLine(g.name), Info: stateWord(worst(g.checks)), Body: strings.Join(out, "\n")}
}

// statusPreview tells a commit status in full: how it stands, what its service
// said and where.
func (b *builder) statusPreview(s core.StatusContext) Preview {
	state := s.Outcome()
	out := []string{b.cut(b.st.bold.Render(termtext.OneLine(s.Context)) + b.dim(b.st.Glyphs.Separator+"commit status"+b.st.Glyphs.Separator) + b.word(state))}
	if d := strings.TrimSpace(termtext.OneLine(s.Description)); d != "" {
		out = append(out, "", wrap(d, b.o.Width))
	}
	if s.TargetURL != "" {
		out = append(out, "", b.cut(b.st.link(s.TargetURL, s.TargetURL)))
	}
	return Preview{Title: termtext.OneLine(s.Context), Info: stateWord(state), Body: strings.Join(out, "\n")}
}

// short is the first characters of a commit's SHA.
func short(sha string) string {
	return sha[:min(len(sha), shortSHA)]
}
