package pulls

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// pickerCommits are three commits of the pull request, oldest first.
func pickerCommits() []core.Commit {
	at := func(d time.Duration) core.Signature {
		return core.Signature{Name: "Hubot", Date: time.Now().Add(-d)}
	}
	return []core.Commit{
		{SHA: "1111111aaaa", Subject: "first change", Author: at(72 * time.Hour)},
		{SHA: "2222222bbbb", Subject: "second change", Author: at(3 * time.Hour)},
		{SHA: "c0ffee0cccc", Subject: "third change", Author: at(5 * time.Minute)},
	}
}

// pickerRows returns the paths and details of the rows the picker lists.
func pickerRows(t *testing.T, m *detailModal) (paths, details []string) {
	t.Helper()
	if m.find == nil {
		t.Fatal("no picker is open")
	}
	for i := 0; ; i++ {
		it, ok := m.find.At(i)
		if !ok {
			return paths, details
		}
		paths, details = append(paths, it.Path), append(details, it.Detail)
	}
}

// pickerModal opens the Files tab, on scope s, of a pull request that has
// the commits of pickerCommits, and with changes since "old" if since is set.
func pickerModal(t *testing.T, s filesScope, since, cached bool) (*host, *detailModal, *fakeService) {
	t.Helper()
	var (
		h   *host
		m   *detailModal
		svc *fakeService
	)
	if since {
		h, m, svc = catching(t, lastSeen("old"), 100, aheadBy(2))
	} else {
		svc = newFakeService()
		h, m = filed(t, svc, 100, 30)
	}
	file := core.CommitFile{Path: "a.go", Status: "modified", SHA: "blob", Additions: 1, Deletions: 1, Patch: "@@ -1 +1 @@\n-a\n+b\n"}
	svc.ranges = map[string]core.CompareDetail{"old": {Files: []core.CommitFile{file}}}
	svc.commitFiles = map[string][]core.CommitFile{"c0ffee0cccc": {file}}
	svc.commits, svc.commitsCached = pickerCommits(), cached
	m.tab = filesTab
	drain(t, h, m.setFilesScope(s))
	return h, m, svc
}

func TestScopePickerListsTheChoices(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		since  bool
		cached bool
		want   []string
	}{
		{"without since", false, true, []string{"All changes", "c0ffee0 third change", "2222222 second change", "1111111 first change"}},
		{"with since", true, true, []string{"All changes", "Since you looked (2 commits)", "c0ffee0 third change", "2222222 second change", "1111111 first change"}},
		{"read when not cached", true, false, []string{"All changes", "Since you looked (2 commits)", "c0ffee0 third change", "2222222 second change", "1111111 first change"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m, svc := pickerModal(t, filesScope{}, tt.since, tt.cached)
			press(t, h, "w")
			paths, details := pickerRows(t, m)
			if fmt.Sprint(paths) != fmt.Sprint(tt.want) {
				t.Errorf("rows = %q, want %q", paths, tt.want)
			}
			if v := m.find.View(); !strings.Contains(v, "scopes") || strings.Contains(v, "file") {
				t.Errorf("the picker should count scopes, not files:\n%s", v)
			}
			if details[0] != "current" {
				t.Errorf("the detail of All changes is %q, want it marked as the current scope", details[0])
			}
			last := details[len(details)-2]
			if !strings.HasPrefix(last, "Hubot · ") || !strings.HasSuffix(last, "3h") {
				t.Errorf("the byline of the second commit is %q, want author and age", last)
			}
			if want := map[bool]int{true: 0, false: 1}[tt.cached]; svc.commitsReads != want {
				t.Errorf("commits were read %d times, want %d", svc.commitsReads, want)
			}
		})
	}
}

func TestScopePickerMarksTheCurrentScope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		scope filesScope
		row   int
	}{
		{"since", filesScope{kind: scopeSince, base: "old", commits: 2}, 1},
		{"commit", filesScope{kind: scopeCommit, sha: "2222222bbbb", subject: "second change"}, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m, _ := pickerModal(t, tt.scope, true, true)
			press(t, h, "w")
			_, details := pickerRows(t, m)
			for i, d := range details {
				if marked := strings.HasPrefix(d, "current"); marked != (i == tt.row) {
					t.Errorf("row %d has detail %q, want only row %d marked", i, d, tt.row)
				}
			}
		})
	}
}

func TestScopePickerChoosesTheScope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		keys []string
		want filesScope
	}{
		{"since", []string{"down", "enter"}, filesScope{kind: scopeSince, base: "old", commits: 2}},
		{"a commit", []string{"down", "down", "down", "enter"}, filesScope{kind: scopeCommit, sha: "2222222bbbb", subject: "second change"}},
		{"by typing", []string{"f", "i", "r", "s", "t", "enter"}, filesScope{kind: scopeCommit, sha: "1111111aaaa", subject: "first change"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m, _ := pickerModal(t, filesScope{}, true, true)
			press(t, h, "w")
			for _, k := range tt.keys {
				press(t, h, k)
			}
			if m.find != nil {
				t.Error("choosing left the picker open")
			}
			if m.files.scope.kind != tt.want.kind || m.files.scope.sha != tt.want.sha || m.files.scope.base != tt.want.base {
				t.Errorf("scope = %+v, want %+v", m.files.scope, tt.want)
			}
		})
	}
}

func TestScopePickerChoosesAllAgain(t *testing.T) {
	t.Parallel()
	h, m, _ := pickerModal(t, filesScope{kind: scopeSince, base: "old", commits: 2}, true, true)
	press(t, h, "w")
	press(t, h, "up")
	press(t, h, "enter")
	if m.files.scope.kind != scopeAll {
		t.Errorf("scope = %+v, want all", m.files.scope)
	}
}

func TestScopePickerEscKeepsTheScope(t *testing.T) {
	t.Parallel()
	h, m, svc := pickerModal(t, filesScope{}, true, true)
	press(t, h, "w")
	press(t, h, "down")
	press(t, h, "esc")
	if m.find != nil {
		t.Error("esc left the picker open")
	}
	if m.files.scope.kind != scopeAll || len(svc.rangeReads) != 0 {
		t.Errorf("scope = %+v after esc, with %d range reads, want all and none", m.files.scope, len(svc.rangeReads))
	}
	if h.modal() != m {
		t.Error("esc closed the modal with the picker")
	}
}

func TestScopeKeyInBothPanes(t *testing.T) {
	t.Parallel()
	for _, pane := range []filesPane{treePane, diffPane} {
		t.Run(fmt.Sprint(pane), func(t *testing.T) {
			t.Parallel()
			h, m, _ := pickerModal(t, filesScope{}, false, true)
			m.files.setFocus(pane)
			press(t, h, "w")
			if m.find == nil {
				t.Fatal("w opened no picker")
			}
		})
	}
}

func TestScopePickerTypesW(t *testing.T) {
	t.Parallel()
	h, m, _ := pickerModal(t, filesScope{}, false, true)
	press(t, h, "w")
	press(t, h, "w")
	if m.find == nil {
		t.Fatal("w inside the picker closed it")
	}
}

func TestScopePickerReadErrorToasts(t *testing.T) {
	t.Parallel()
	h, m, svc := pickerModal(t, filesScope{}, false, false)
	svc.commitsErr = errors.New("boom")
	msgs := press(t, h, "w")
	if m.find != nil {
		t.Error("the picker stays open after the commits failed")
	}
	if got := fmt.Sprint(msgs); !strings.Contains(got, "Couldn't read the commits") || !strings.Contains(got, "boom") {
		t.Errorf("msgs = %v, want a toast with the error", msgs)
	}
}

func TestScopePickerNeedsFiles(t *testing.T) {
	t.Parallel()
	h, m := filed(t, newFakeService(), 100, 30)
	m.tab = filesTab
	press(t, h, "w")
	if m.find != nil {
		t.Error("w opened a picker before the files were read")
	}
}
