package jobview

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/core"
)

// asOf is when a poll read a partial log, on the local clock the view
// shows it by.
func asOf(minute int) time.Time { return time.Date(2026, 9, 24, 14, minute, 0, 0, time.Local) }

// partialLog is the log of the running job as far as GitHub publishes it:
// the line of its first step and n of its second, read at minute of gen.
func partialLog(n, minute, gen int) core.PartialLog {
	lines := make([]core.LogLine, 0, n+1)
	lines = append(lines, core.LogLine{Time: at(80 * time.Second), Text: "Current runner version: '2.337.0'", Step: 1})
	for i := range n {
		lines = append(lines, core.LogLine{Time: at(70 * time.Second), Text: "lint " + strconv.Itoa(i+1), Step: 2})
	}
	return core.PartialLog{Lines: lines, At: asOf(minute), Gen: gen}
}

func TestPartialLog(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(2, 5, 1)
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	if m.State() != Partial || m.Lines() != 3 {
		t.Fatalf("state %d with %d lines, want the partial log", m.State(), m.Lines())
	}
	if s := text(m); !strings.Contains(s, "partial, as of 14:05") || !strings.Contains(s, "Run golangci-lint") {
		t.Errorf("the partial log isn't labelled with its time:\n%s", s)
	}
	if f.watching[runningJob] != 1 || !slices.Equal(f.partialReads, []int64{runningJob}) {
		t.Errorf("watches %v and reads %v, want one of each", f.watching, f.partialReads)
	}
	assertFits(t, m.View(), 80, 12)
}

// The poll appends to the partial log, which keeps the cursor and the
// folds.
func TestPartialLogAppends(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(2, 5, 1)
	m := newView(t, f, 80, 12)
	m.Focus()
	run(m, m.Show(running(), false, Hints{}))
	// The steps show folded; open the second, onto its first line.
	keys(m, "j", "enter", "j")
	if s := text(m); !strings.Contains(s, "▌2 lint 1") {
		t.Fatalf("the cursor isn't on the first line of the second step:\n%s", s)
	}

	f.poll(partialLog(4, 15, 1))
	run(m, m.Show(running(), false, Hints{}))
	s := text(m)
	for _, want := range []string{"▌2 lint 1", "row 3/6", "lint 4", "partial, as of 14:15"} {
		if !strings.Contains(s, want) {
			t.Errorf("the view lacks %q after the poll:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Current runner version") {
		t.Errorf("the first step opened:\n%s", s)
	}
	if len(f.partialReads) != 1 {
		t.Errorf("read the partial log %d times, want the poll to bring it", len(f.partialReads))
	}

	// The search finds what was appended, as well as the step's title, and
	// every step still folds.
	keys(m, "/", "l", "i", "n", "t", "enter")
	f.poll(partialLog(5, 16, 1))
	run(m, m.Show(running(), false, Hints{}))
	if s := text(m); !strings.Contains(s, "match 2/6") {
		t.Errorf("the search didn't take the appended line:\n%s", s)
	}
	keys(m, "esc", "*", "*")
	if s := text(m); strings.Contains(s, "lint 2") {
		t.Errorf("* didn't fold the steps of the partial log:\n%s", s)
	}
}

// A partial log that started over replaces the one shown.
func TestPartialLogStartsOver(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(4, 5, 1)
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	f.poll(partialLog(1, 6, 2))
	run(m, m.Show(running(), false, Hints{}))
	if m.State() != Partial || m.Lines() != 2 {
		t.Errorf("state %d with %d lines, want the 2 of the new log", m.State(), m.Lines())
	}
}

// A read of a partial log that replaces the lines shown prepares them in
// its command, as a big log takes a while to; one that adds to them
// leaves that to the view.
func TestPartialLogPreparedInRead(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(4, 5, 1)
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	f.partial[runningJob] = partialLog(6, 6, 1)
	if msg := m.readPartial()().(partialMsg); msg.shown != nil {
		t.Error("a read that adds to the lines shown prepared them")
	}
	f.partial[runningJob] = partialLog(1, 7, 2)
	msg := m.readPartial()().(partialMsg)
	if msg.shown == nil {
		t.Fatal("a read of a new generation didn't prepare it")
	}
	*m, _ = m.Update(msg)
	if m.State() != Partial || m.Lines() != 2 {
		t.Errorf("state %d with %d lines, want the 2 of the new log", m.State(), m.Lines())
	}
}

// Until GitHub publishes some of the log, the steps show, and the poll
// brings the log.
func TestPartialLogPending(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	if m.State() != Pending || !strings.Contains(text(m), "Logs appear when the job finishes") {
		t.Fatalf("state %d, want the steps:\n%s", m.State(), text(m))
	}
	f.poll(partialLog(1, 5, 1))
	run(m, m.Show(running(), false, Hints{}))
	if m.State() != Partial || len(f.partialReads) != 1 {
		t.Errorf("state %d after %d reads, want the partial log the poll read", m.State(), len(f.partialReads))
	}
	m.Clear()
	if f.watching[runningJob] != 0 || strings.Contains(m.view.Title(), "partial") {
		t.Errorf("a cleared view still watches %v, titled %q", f.watching, m.view.Title())
	}
}

// Once the job completes, its whole log replaces the partial one, and the
// view stops watching it.
func TestPartialLogThenDone(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(2, 5, 1)
	f.logs[runningJob] = partialLog(6, 0, 0).Log
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	j := running()
	j.Status, j.Conclusion = core.RunCompleted, core.ConclusionSuccess
	run(m, m.Show(j, false, Hints{}))
	if m.State() != Ready || m.Lines() != 7 || !slices.Equal(f.reads, []int64{runningJob}) {
		t.Errorf("state %d with %d lines after reads %v, want the whole log", m.State(), m.Lines(), f.reads)
	}
	if f.watching[runningJob] != 0 || strings.Contains(text(m), "partial") {
		t.Errorf("the whole log is watched %v or labelled partial:\n%s", f.watching, text(m))
	}
}

// A job shown to rest reads its partial log once the cursor rests on it.
func TestPartialLogRests(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(2, 5, 1)
	m := newView(t, f, 80, 12, WithRest(time.Hour))
	_ = m.Show(running(), true, Hints{})
	if m.State() != Pending || len(f.partialReads) != 0 {
		t.Fatalf("a job shown to rest is in state %d after %d reads, want its steps, unread", m.State(), len(f.partialReads))
	}
	run(m, m.ReadNow())
	if m.State() != Partial {
		t.Errorf("read now left state %d, want the partial log", m.State())
	}
}

// The keys of the log work on a partial log, so the help lists them.
func TestPartialLogKeys(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(2, 5, 1)
	m := newView(t, f, 80, 12)
	m.Focus()
	run(m, m.Show(running(), false, Hints{}))
	layers := m.KeyLayers()
	log := layers[len(layers)-1]
	if !slices.ContainsFunc(log.Bindings, func(b key.Binding) bool { return b.Enabled() && b.Help().Key == "*" }) {
		t.Errorf("the keys of a partial log are off: %+v", log.Bindings)
	}
}

// A read that ends after the poll that followed it doesn't roll the log
// back, and a check that found nothing new moves its time on.
func TestPartialLogOlderReads(t *testing.T) {
	f := newFake()
	f.partial[runningJob] = partialLog(4, 15, 1)
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	*m, _ = m.Update(partialMsg{id: m.id, jobID: runningJob, log: partialLog(2, 5, 1)})
	if m.Lines() != 5 || !strings.Contains(text(m), "partial, as of 14:15") {
		t.Errorf("an older read rolled the log back to %d lines:\n%s", m.Lines(), text(m))
	}
	f.poll(partialLog(4, 17, 1))
	run(m, m.Show(running(), false, Hints{}))
	if m.Lines() != 5 || !strings.Contains(text(m), "partial, as of 14:17") {
		t.Errorf("a check with nothing new left %d lines:\n%s", m.Lines(), text(m))
	}
}

// A job that waits for a runner has no log to watch until it starts, and
// a paused view stops watching until the job shows again.
func TestPartialLogWatch(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12)
	j := running()
	j.Status = core.RunQueued
	run(m, m.Show(j, false, Hints{}))
	if f.watching[runningJob] != 0 || len(f.partialReads) != 0 {
		t.Fatalf("a queued job is watched %v, read %v", f.watching, f.partialReads)
	}
	run(m, m.Show(running(), false, Hints{}))
	if f.watching[runningJob] != 1 || len(f.partialReads) != 0 {
		t.Errorf("a job that started is watched %v, read %v; want watched, for the poll to read", f.watching, f.partialReads)
	}
	m.Pause()
	if f.watching[runningJob] != 0 {
		t.Error("a paused view still watches")
	}
	run(m, m.Show(running(), false, Hints{}))
	if f.watching[runningJob] != 1 {
		t.Error("a view shown again after a pause doesn't watch")
	}
}
