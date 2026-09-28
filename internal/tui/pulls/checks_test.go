package pulls

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeChecks serves the checks of every pull request, with a failing
// check of another app, so that nothing more is read.
type fakeChecks struct {
	read bool
}

func (f *fakeChecks) value() core.Checks {
	return core.Checks{SHA: "abc", Total: 2, Runs: []core.Check{
		{ID: 1, Name: "codecov", Status: core.RunCompleted, Conclusion: core.ConclusionFailure, Summary: "Coverage fell"},
		{ID: 2, Name: "dco", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess},
	}}
}

func (f *fakeChecks) CachedChecks(actionssvc.ChecksQuery) (core.Checks, bool) {
	return f.value(), f.read
}
func (f *fakeChecks) Checks(context.Context, actionssvc.ChecksQuery) (core.Checks, error) {
	f.read = true
	return f.value(), nil
}
func (f *fakeChecks) CachedRun(core.RepoRef, int64) (core.Run, bool) { return core.Run{}, false }
func (f *fakeChecks) Run(context.Context, core.RepoRef, int64) (core.Run, error) {
	return core.Run{}, nil
}
func (f *fakeChecks) CachedAllJobs(actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	return core.Page[core.Job]{}, false
}
func (f *fakeChecks) AllJobs(context.Context, actionssvc.JobsQuery) (core.Page[core.Job], error) {
	return core.Page[core.Job]{}, nil
}
func (f *fakeChecks) CachedLog(core.RepoRef, int64) (core.Log, bool) { return core.Log{}, false }
func (f *fakeChecks) Log(context.Context, core.RepoRef, int64) (core.Log, error) {
	return core.Log{}, nil
}
func (f *fakeChecks) CachedPartialLog(core.RepoRef, int64) (core.PartialLog, bool) {
	return core.PartialLog{}, false
}
func (f *fakeChecks) PartialLog(context.Context, core.RepoRef, int64) (core.PartialLog, error) {
	return core.PartialLog{}, core.ErrLogPending
}
func (f *fakeChecks) WatchLog(core.RepoRef, int64, int64) func() { return func() {} }
func (f *fakeChecks) CachedAnnotations(actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	return core.Page[core.Annotation]{}, false
}
func (f *fakeChecks) Annotations(context.Context, actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	return core.Page[core.Annotation]{}, nil
}
func (f *fakeChecks) Invalidate(core.RepoRef) {}
func (f *fakeChecks) RerunFailedJobs(core.RepoRef, int64) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
}

func modalText(h *host) string {
	return strings.Join(strings.Fields(ansi.Strip(h.modals[len(h.modals)-1].View())), " ")
}

func TestChecksKeyOnARowOpensTheChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(t, h, "C")
	m := h.modal()
	if m == nil || m.number != 142 || m.checks == nil {
		t.Fatalf("C opened %v, want #142 on its checks", m)
	}
	if v := modalText(h); !strings.Contains(v, "Checks ✗ 1 failing, ✓ 1 passed") || !strings.Contains(v, "codecov") {
		t.Errorf("the modal doesn't show the checks:\n%s", v)
	}
	// esc steps back to the detail, whose header counts them too.
	press(t, h, "esc")
	if m.checks != nil || h.modal() != m {
		t.Fatal("esc from the checks didn't step back to the detail")
	}
	if v := modalText(h); !strings.Contains(v, "CI ✗ 1 failing, ✓ 1 passed · C for details") {
		t.Errorf("the header doesn't count the checks:\n%s", v)
	}
	press(t, h, "C")
	if m.checks == nil {
		t.Fatal("C in the detail didn't open the checks")
	}
	press(t, h, "esc")
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc from the detail didn't close the modal")
	}
}

func TestOpenPullOnItsChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 135, Checks: true}))
	if m := h.modal(); m == nil || m.number != 135 || m.checks == nil {
		t.Fatalf("OpenPullMsg opened %v, want #135 on its checks", m)
	}
	// The step's messages go through the modal: enter shows the detail of
	// the failing check.
	press(t, h, "enter")
	if v := modalText(h); !strings.Contains(v, "Coverage fell") {
		t.Errorf("enter didn't show the check:\n%s", v)
	}
}

func TestChecksKeyWithoutChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30)
	press(t, h, "C")
	if h.modal() != nil {
		t.Error("C opened a modal without checks")
	}
	press(t, h, "enter")
	press(t, h, "C")
	if m := h.modal(); m == nil || m.checks != nil {
		t.Error("C opened checks in the detail without them")
	}
}

var _ checks.Service = (*fakeChecks)(nil)
