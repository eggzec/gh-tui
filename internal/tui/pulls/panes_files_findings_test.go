package pulls

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func note(path string, l core.AnnotationLevel) core.Annotation {
	return core.Annotation{Path: path, StartLine: 1, EndLine: 1, Level: l, Message: "m"}
}

// sameBadges says whether two findings have the same counts and worst levels,
// whatever the annotations that they list.
func sameBadges(a, b map[string]files.Finding) bool {
	return maps.EqualFunc(a, b, func(x, y files.Finding) bool { return x.Count == y.Count && x.Level == y.Level })
}

// findingsChecks serves two check runs on the last commit: a job of GitHub
// Actions that brings its annotations, and an App's whose annotations are read
// from the run. The middle commit has one run with a notice on a file that the
// other has none on.
func findingsChecks() *fakeChecks {
	return &fakeChecks{
		runs: map[string][]core.CommitCheck{
			"ddd4444": {
				{ID: 5, JobID: 5, RunID: 9, Name: "build", Annotations: 2, Notes: []core.Annotation{
					note("a.go", core.AnnotationWarning), note("internal/b.go", core.AnnotationWarning),
				}},
				{ID: 7, Name: "lint", Annotations: 2},
			},
			"bbb2222": {
				{ID: 5, JobID: 5, RunID: 9, Name: "build", Annotations: 1, Notes: []core.Annotation{
					note("docs/c.md", core.AnnotationNotice),
				}},
			},
		},
		pages: map[int64][]core.Annotation{
			7: {note("a.go", core.AnnotationFailure), note("a.go", core.AnnotationNotice)},
		},
	}
}

func findingsPanes(t *testing.T, fc *fakeChecks) (*host, *panesModal) {
	t.Helper()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = changedFiles()
		svc.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: changedFiles()}}
	}, WithChecks(fc))
	return h, m
}

func TestFilesPaneFindingsOfTheLastCommit(t *testing.T) {
	t.Parallel()
	fc := findingsChecks()
	h, m := findingsPanes(t, fc)
	drain(t, h, m.readFindings())
	want := map[string]files.Finding{
		"a.go":          {Count: 3, Level: core.AnnotationFailure},
		"internal/b.go": {Count: 1, Level: core.AnnotationWarning},
	}
	got := m.filesData().Findings
	if !sameBadges(got, want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}
	if len(fc.noted) != 1 || fc.noted[0] != "ddd4444" {
		t.Errorf("read the annotations of %v, want only ddd4444 once", fc.noted)
	}
}

func TestFilesPaneFindingsFollowTheRange(t *testing.T) {
	t.Parallel()
	fc := findingsChecks()
	h, m := findingsPanes(t, fc)
	drain(t, h, m.readFindings())
	m.files.rng = files.Range{First: 1, Last: 1}
	cmd := m.readFindings()
	// The badges of the last commit go at once, before the new ones arrive.
	if got := m.filesData().Findings; len(got) != 0 {
		t.Errorf("findings = %v while the range's are read, want none", got)
	}
	drain(t, h, cmd)
	want := map[string]files.Finding{"docs/c.md": {Count: 1, Level: core.AnnotationNotice}}
	if got := m.filesData().Findings; !sameBadges(got, want) {
		t.Errorf("findings of bbb2222 = %v, want %v", got, want)
	}
	if want := []string{"ddd4444", "bbb2222"}; !slices.Equal(fc.noted, want) {
		t.Errorf("read the annotations of %v, want %v", fc.noted, want)
	}
	// Selecting the range again reads nothing.
	if cmd := m.readFindings(); cmd != nil {
		t.Error("read the findings of the same commit twice")
	}
}

func TestFilesPaneFindingsPendingAndFailed(t *testing.T) {
	t.Parallel()
	fc := findingsChecks()
	h, m := findingsPanes(t, fc)
	m.files.rng = files.Range{First: 1, Last: 1}
	cmd := m.readFindings()
	if cmd == nil {
		t.Fatal("no read for a commit that was not read")
	}
	if got := m.filesData().Findings; got != nil {
		t.Errorf("findings = %v before the read answered, want none", got)
	}
	// A failed read leaves the rows as they are, with no toast.
	fc.mu.Lock()
	fc.notedErr = errors.New("offline")
	fc.mu.Unlock()
	msgs := drain(t, h, cmd)
	if got := m.filesData().Findings; got != nil {
		t.Errorf("findings = %v after a failed read, want none", got)
	}
	for _, msg := range msgs {
		if _, ok := msg.(findingsMsg); !ok {
			continue
		}
		if msg.(findingsMsg).err == nil {
			t.Error("the read didn't fail")
		}
	}
}

// syncOf is the message of a poll of the checks of the commit sha that found
// a change.
func syncOf(m *panesModal, sha string) ui.SyncMsg {
	return ui.SyncMsg{Key: actionssvc.CommitChecksSyncKey(actionssvc.CommitChecksQuery{Repo: m.repo, SHA: sha})}
}

// A run that finishes with annotations after the findings were read shows its
// badge when the poll of the commit reports a change, with no change of range.
func TestFilesPaneFindingsRefreshWhenThePollChanges(t *testing.T) {
	t.Parallel()
	fc := findingsChecks()
	// The lint of the commit is still running: it has no annotations yet.
	lint := fc.runs["ddd4444"][1]
	lint.Status, lint.Annotations = core.RunInProgress, 0
	fc.runs["ddd4444"] = []core.CommitCheck{fc.runs["ddd4444"][0], lint}
	h, m := findingsPanes(t, fc)
	drain(t, h, m.readFindings())
	if got, want := m.filesData().Findings, map[string]files.Finding{
		"a.go":          {Count: 1, Level: core.AnnotationWarning},
		"internal/b.go": {Count: 1, Level: core.AnnotationWarning},
	}; !sameBadges(got, want) {
		t.Fatalf("findings before = %v, want %v", got, want)
	}

	// A change of another commit's checks reads nothing.
	if cmd := m.refreshFindings(syncOf(m, "bbb2222")); cmd != nil {
		t.Error("read the findings again for the checks of another commit")
	}
	if cmd := m.refreshFindings(ui.SyncMsg{Key: syncOf(m, "ddd4444").Key, Err: errors.New("offline")}); cmd != nil {
		t.Error("read the findings again for a failed poll")
	}

	// The lint finishes with annotations, and the poll reports it.
	fc.mu.Lock()
	fc.runs["ddd4444"] = findingsChecks().runs["ddd4444"]
	fc.mu.Unlock()
	cmd := m.Update(syncOf(m, "ddd4444"))
	// The badges stay while the new answer is on its way.
	if len(m.filesData().Findings) != 2 {
		t.Errorf("findings = %v while the new ones are read, want the old", m.filesData().Findings)
	}
	drain(t, h, cmd)
	want := map[string]files.Finding{
		"a.go":          {Count: 3, Level: core.AnnotationFailure},
		"internal/b.go": {Count: 1, Level: core.AnnotationWarning},
	}
	if got := m.filesData().Findings; !sameBadges(got, want) {
		t.Errorf("findings after = %v, want %v", got, want)
	}
	if !slices.Equal(fc.again, []string{"ddd4444"}) {
		t.Errorf("asked GitHub again for %v, want ddd4444 once", fc.again)
	}
}

// A failed read again keeps the badges that are shown.
func TestFilesPaneFindingsRefreshFailureKeepsTheBadges(t *testing.T) {
	t.Parallel()
	fc := findingsChecks()
	h, m := findingsPanes(t, fc)
	drain(t, h, m.readFindings())
	want := maps.Clone(m.filesData().Findings)
	fc.mu.Lock()
	fc.notedErr = errors.New("offline")
	fc.mu.Unlock()
	drain(t, h, m.Update(syncOf(m, "ddd4444")))
	if got := m.filesData().Findings; !sameBadges(got, want) {
		t.Errorf("findings = %v after a failed read again, want %v", got, want)
	}
}

// The answer to a read that a newer one replaced, of the same commit or of
// another, never overwrites what the newer one brought.
func TestFilesPaneFindingsDropsOlderAnswers(t *testing.T) {
	t.Parallel()
	fc := findingsChecks()
	_, m := findingsPanes(t, fc)
	all := m.files.rng
	m.files.rng = files.Range{First: 1, Last: 1}
	first := m.readFindings()
	late := first().(findingsMsg)

	// Another commit: the range moves back before the first answer comes.
	m.files.rng = all
	second := m.readFindings()
	m.findingsRecv(late)
	if got := m.filesData().Findings; got != nil {
		t.Errorf("findings = %v after the answer of bbb2222 for ddd4444, want none", got)
	}
	msg := second().(findingsMsg)
	if msg.err != nil || msg.sha != "ddd4444" {
		t.Fatalf("answer = %+v, want the findings of ddd4444", msg)
	}
	m.findingsRecv(msg)
	want := map[string]files.Finding{
		"a.go":          {Count: 3, Level: core.AnnotationFailure},
		"internal/b.go": {Count: 1, Level: core.AnnotationWarning},
	}
	if got := m.filesData().Findings; !sameBadges(got, want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}

	// The same commit: a read again replaces the one before it.
	older := m.refreshFindings(syncOf(m, "ddd4444"))
	lateAgain := older().(findingsMsg)
	newer := m.refreshFindings(syncOf(m, "ddd4444"))
	fc.mu.Lock()
	fc.runs["ddd4444"] = nil
	fc.mu.Unlock()
	m.findingsRecv(newer().(findingsMsg))
	if got := m.filesData().Findings; len(got) != 0 {
		t.Fatalf("findings = %v, want none from the newer answer", got)
	}
	m.findingsRecv(lateAgain)
	if got := m.filesData().Findings; len(got) != 0 {
		t.Errorf("findings = %v after an older answer, want the newer, none", got)
	}
}

func TestFilesPaneFindingsWithoutService(t *testing.T) {
	t.Parallel()
	_, m, _ := rangePanes(t, sampleCommits(nil), nil)
	if cmd := m.readFindings(); cmd != nil {
		t.Error("read findings with no checks service")
	}
}
