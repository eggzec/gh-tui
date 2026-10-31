package pulls

import (
	"errors"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	actions "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The preview of a job of the Merge pane shows its log, scrolled to the first
// error. The log is read once per job, and drawn once per width until the
// pane changes. A log longer than shownLines shows a window of that many
// lines around the first error, since a preview that holds every line of a
// big log would cost a frame its whole length.
const (
	shownLines = 2000
	// errAbove is how many lines before the first error the window starts at,
	// and errSeen how many lines of the preview show above it.
	errAbove = 1000
	errSeen  = 3
)

type jobLogState uint8

const (
	jobLogLoading jobLogState = iota
	// jobLogPending is a log that GitHub publishes nothing of yet.
	jobLogPending
	jobLogReady
	// jobLogGone is a log that couldn't be read, such as an expired one.
	jobLogGone
)

// jobLog is the log of a job as the Merge pane keeps it.
type jobLog struct {
	state jobLogState
	lines []core.LogLine
	// from is the first line shown, and first the line of the first error,
	// which found says there is.
	from, first int
	found       bool
	// final is set for the log of a job that had finished when it was read.
	final bool
	// gen and at are those of the partial log that l holds, to tell a log that
	// started over from one that grew.
	gen int
	at  time.Time
	// seen is the state of the job when its log was found pending, so that a
	// log is read again only once the job moved.
	seen jobState
}

// jobState is the status and conclusion of a job.
type jobState struct {
	status     core.RunStatus
	conclusion core.Conclusion
}

func stateOf(cc core.CommitCheck) jobState { return jobState{cc.Status, cc.Conclusion} }

// mergeLogMsg carries the log of a job that the Merge pane read, and the steps
// of the job when they were read with it.
type mergeLogMsg struct {
	owner *panesModal
	job   int64
	run   int64
	final bool
	log   core.Log
	gen   int
	at    time.Time
	steps []core.Step
	seen  jobState
	err   error
}

// jobRow is the ID of the row of a job in the Merge pane.
func jobRow(id int64) string { return "job/" + strconv.FormatInt(id, 10) }

// jobOf returns the check run that the row with the ID id is the job of.
func (p *mergePane) jobOf(id string) (core.CommitCheck, bool) {
	rest, ok := strings.CutPrefix(id, "job/")
	if !ok {
		return core.CommitCheck{}, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || n == 0 {
		return core.CommitCheck{}, false
	}
	i := slices.IndexFunc(p.data.Checks.Runs, func(c core.CommitCheck) bool { return c.JobID == n })
	if i < 0 {
		return core.CommitCheck{}, false
	}
	return p.data.Checks.Runs[i], true
}

// hoveredJob is the job that the preview of the Merge pane shows, when it is
// shown and the modal can read logs.
func (m *panesModal) hoveredJob() (core.CommitCheck, bool) {
	p := m.merge
	if p == nil || m.checks == nil || m.nav.Focus() != paneMerge || m.closed {
		return core.CommitCheck{}, false
	}
	if _, ok := m.layout().Box(paneMerge, panes.Preview); !ok {
		return core.CommitCheck{}, false
	}
	return p.jobOf(m.hover[paneMerge-1].id)
}

// unwatchLog ends the follow of the log of a running job.
func (p *mergePane) unwatchLog() {
	if p.stopLog != nil {
		p.stopLog()
	}
	p.stopLog, p.watchedLog = nil, 0
}

// loadLog reads the log of the job that the preview shows, once, and follows
// it only while it is shown and running. It runs after every message, and does
// nothing when there is nothing to read.
func (m *panesModal) loadLog() tea.Cmd {
	p := m.merge
	cc, ok := m.hoveredJob()
	if !ok {
		if p != nil {
			p.unwatchLog()
		}
		return nil
	}
	svc, repo := m.checks, m.repo
	id := cc.JobID
	running := cc.Status != core.RunCompleted
	if !running || p.watchedLog != id {
		p.unwatchLog()
	}
	if running && p.stopLog == nil {
		p.stopLog, p.watchedLog = m.followLog(cc), id
	}
	l := p.logs[id]
	now := stateOf(cc)
	// A log that is pending, or can't be read, is read again only when the job
	// moved on, so that a log GitHub is slow to publish isn't asked for in a
	// loop.
	if l != nil && (l.state == jobLogLoading || l.state == jobLogGone || l.final || running ||
		l.state == jobLogPending && l.seen == now) {
		return nil
	}
	steps, haveSteps := m.cachedSteps(cc)
	if running {
		if pl, ok := svc.CachedPartialLog(repo, id); ok {
			m.storePartial(cc, pl, steps)
			return nil
		}
	} else if lg, ok := svc.CachedLog(repo, id); ok && haveSteps {
		m.storeLog(cc, lg, true, steps)
		return nil
	}
	p.logs[id] = &jobLog{state: jobLogLoading}
	p.version++
	ctx, owner, run := m.ctx, m, cc.RunID
	return func() tea.Msg {
		if running {
			pl, err := svc.PartialLog(ctx, repo, id)
			return mergeLogMsg{owner: owner, job: id, run: run, log: pl.Log, gen: pl.Gen, at: pl.At, steps: steps, seen: now, err: err}
		}
		lg, err := svc.Log(ctx, repo, id)
		// The job view opens on the first failed step, so the steps are read with
		// the log when nothing has read them yet.
		if err == nil && !haveSteps {
			if pg, jerr := svc.AllJobs(ctx, actions.JobsQuery{Repo: repo, RunID: run}); jerr == nil {
				steps = stepsOf(pg, id)
			}
		}
		return mergeLogMsg{owner: owner, job: id, run: run, final: true, log: lg, steps: steps, seen: now, err: err}
	}
}

// followLog starts what keeps the log of the running job of cc current: the
// service's watch of the log, and the follow of its run, whose poll reads the
// run's jobs and the log. The returned func ends both.
func (m *panesModal) followLog(cc core.CommitCheck) func() {
	stopWatch := m.checks.WatchLog(m.repo, cc.RunID, cc.JobID)
	var stopRun func()
	if m.merge.follow != nil {
		stopRun = m.merge.follow(m.repo, cc.RunID)
	}
	return func() {
		stopWatch()
		if stopRun != nil {
			stopRun()
		}
	}
}

// receiveLog keeps the log that a read brought.
func (m *panesModal) receiveLog(msg mergeLogMsg) {
	p := m.merge
	switch {
	case msg.owner != m:
		return
	case errors.Is(msg.err, core.ErrLogPending):
		p.logs[msg.job] = &jobLog{state: jobLogPending, seen: msg.seen}
	case msg.err != nil:
		if m.ctx.Err() != nil {
			return
		}
		p.logs[msg.job] = &jobLog{state: jobLogGone}
	default:
		cc, ok := p.jobOf(jobRow(msg.job))
		if !ok {
			cc = core.CommitCheck{JobID: msg.job, RunID: msg.run}
		}
		l := m.storeLog(cc, msg.log, msg.final, msg.steps)
		l.gen, l.at = msg.gen, msg.at
		return
	}
	p.version++
}

// refreshLog takes up what a poll of the run of the running job shown added to
// its log, when msg is the sync of that run.
func (m *panesModal) refreshLog(msg ui.SyncMsg) {
	cc, ok := m.hoveredJob()
	if !ok || cc.Status == core.RunCompleted || msg.Err != nil || msg.Key != actions.RunSyncKey(m.repo, cc.RunID) {
		return
	}
	pl, ok := m.checks.CachedPartialLog(m.repo, cc.JobID)
	if !ok {
		return
	}
	l := m.merge.logs[cc.JobID]
	if l == nil || l.state != jobLogReady || l.gen != pl.Gen || !l.at.Equal(pl.At) || len(l.lines) != len(pl.Lines) {
		steps, _ := m.cachedSteps(cc)
		m.storePartial(cc, pl, steps)
	}
}

// storePartial keeps pl as the log of the running job of cc.
func (m *panesModal) storePartial(cc core.CommitCheck, pl core.PartialLog, steps []core.Step) {
	l := m.storeLog(cc, pl.Log, false, steps)
	l.gen, l.at = pl.Gen, pl.At
}

// storeLog keeps lg as the log of the job of cc, and scrolls its preview to the
// first error unless the user scrolled the log once it was shown.
func (m *panesModal) storeLog(cc core.CommitCheck, lg core.Log, final bool, steps []core.Step) *jobLog {
	p := m.merge
	shown := p.logs[cc.JobID]
	l := &jobLog{state: jobLogReady, lines: lg.Lines, final: final}
	l.first, l.found = jobview.FirstError(lg.Lines, steps)
	if l.found {
		l.from = max(0, l.first-errAbove)
	}
	p.logs[cc.JobID] = l
	p.version++
	// Scrolling the note that the log is loading moves nothing, so it doesn't
	// count as the user's choice.
	scrolled := shown != nil && shown.state == jobLogReady
	if key := (previewKey{paneMerge, jobRow(cc.JobID)}); l.found {
		if _, set := m.previews[key]; !set || !scrolled {
			m.previews[key] = max(0, l.hidden()+l.first-l.from-errSeen)
		}
	}
	return l
}

// hidden is how many rows the preview of l shows above its first line: the
// note about the lines left out before it.
func (l *jobLog) hidden() int {
	if l.from > 0 {
		return 1
	}
	return 0
}

// cachedSteps are the steps of the job of cc, and whether the cache holds the
// jobs of its run.
func (m *panesModal) cachedSteps(cc core.CommitCheck) ([]core.Step, bool) {
	pg, ok := m.checks.CachedAllJobs(actions.JobsQuery{Repo: m.repo, RunID: cc.RunID})
	if !ok {
		return nil, false
	}
	return stepsOf(pg, cc.JobID), true
}

// stepsOf returns the steps of the job id in pg.
func stepsOf(pg core.Page[core.Job], id int64) []core.Step {
	for i := range pg.Items {
		if pg.Items[i].ID == id {
			return pg.Items[i].Steps
		}
	}
	return nil
}

// jobLogBody draws the log of the job of the row id to width, or reports false
// for a row that is no job.
func (m *panesModal) jobLogBody(id string, width int) (string, bool) {
	p := m.merge
	cc, ok := p.jobOf(id)
	if !ok || m.checks == nil {
		return "", false
	}
	dim := m.theme.Subtle
	l := p.logs[cc.JobID]
	switch {
	case l == nil || l.state == jobLogLoading:
		return dim.Render("Loading the log" + m.icons.Ellipsis), true
	case l.state == jobLogPending:
		return dim.Render("The log isn't published yet."), true
	case l.state == jobLogGone:
		return dim.Render("The log has expired or can't be read."), true
	case len(l.lines) == 0:
		return dim.Render("The log is empty."), true
	}
	return m.drawLog(l, width), true
}

// drawLog draws the lines of l that are shown, each cut to width: the error
// lines on a red ground behind a red edge, and the warnings behind an amber
// mark.
func (m *panesModal) drawLog(l *jobLog, width int) string {
	to := min(len(l.lines), l.from+shownLines)
	out := make([]string, 0, to-l.from+2)
	dim := m.theme.Subtle
	if l.from > 0 {
		out = append(out, dim.Render(strconv.Itoa(l.from)+" earlier lines not shown"))
	}
	edge := m.theme.Error.Render(m.icons.Edge)
	warn := m.theme.Warning.Render("!")
	ground := lipgloss.NewStyle().Background(m.errGround())
	cut := max(width-2, 0)
	for _, ln := range l.lines[l.from:to] {
		text := termtext.Clean(ansi.Strip(ln.Text), 4)
		text = termtext.Truncate(strings.ReplaceAll(text, "\n", " "), cut, m.icons.Ellipsis)
		switch ln.Kind {
		case core.LogError:
			pad := strings.Repeat(" ", max(cut-ansi.StringWidth(text), 0))
			out = append(out, edge+ground.Render(" "+text+pad))
		case core.LogWarning:
			out = append(out, warn+" "+text)
		case core.LogDebug:
			out = append(out, "  "+dim.Render(text))
		default:
			out = append(out, "  "+text)
		}
	}
	if to < len(l.lines) {
		out = append(out, dim.Render(strconv.Itoa(len(l.lines)-to)+" more lines not shown"))
	}
	return strings.Join(out, "\n")
}

// errGround is the color behind an error line: the error color, mixed into the
// ground of the terminal.
func (m *panesModal) errGround() color.Color {
	c := lipgloss.Color(m.theme.Palette.Error)
	if m.theme.Dark {
		return lipgloss.Darken(c, 0.6)
	}
	// Lightening a color drifts its hue, so mix it into white instead.
	r, g, b, _ := c.RGBA()
	mix := func(v uint32) uint8 { return uint8((v>>8)*15/100 + 255*85/100) }
	return color.RGBA{R: mix(r), G: mix(g), B: mix(b), A: 0xff}
}
