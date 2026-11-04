package owner

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestRepoCountForms(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	all := s.page.header.value.Profile.Repos

	forms := s.repoCount(all)
	if len(forms) < 2 {
		t.Fatalf("forms = %q, want a long and a bare form", forms)
	}
	if bare := forms[len(forms)-1]; strings.ContainsAny(bare, "[/+") {
		t.Errorf("narrowest form = %q, want the bare total", bare)
	}

	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:stars-desc"}))
	if forms = s.repoCount(all); len(forms) == 0 || !strings.Contains(forms[0], "/") {
		t.Errorf("filtered forms = %q, want matched/all first", forms)
	}
}

func TestRepoCountNoneWhileLoading(t *testing.T) {
	t.Parallel()
	s := New(t.Context(), newFake(), config.Default().Keys, WithNow(func() time.Time { return now }))
	setSize(s, 80, 24)
	s.Update(ui.OwnerMsg{Login: "octocat"})
	if forms := s.repoCount(40); len(forms) != 0 {
		t.Errorf("while loading, forms = %q", forms)
	}
}

// Where the long count doesn't fit, the tabs keep their titles and gaps and
// step down to the bare total before anything else shrinks.
func TestTabsStepDownToBareCount(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	tabs := tabsOf(s.page.header.value.Kind)
	forms := s.repoCount(s.page.header.value.Profile.Repos)

	wide := s.tabs(tabs, 400, 400)
	if !strings.Contains(ansi.Strip(wide), forms[0]) {
		t.Fatalf("wide tabs %q lack %q", ansi.Strip(wide), forms[0])
	}
	// Narrowing the row, the long count gives way to the bare total while the
	// gaps stay normal.
	bare := forms[len(forms)-1]
	for w := ansi.StringWidth(wide) - 1; w > 20; w-- {
		got := ansi.Strip(s.tabs(tabs, w, w))
		if !strings.Contains(got, forms[0]) && strings.Contains(got, " "+bare+"  Stars") {
			return
		}
	}
	t.Errorf("no width shows the bare total %q with the gaps kept", bare)
}

// The tabs line steps the count down for the language at its right, so a
// longer count never pushes the language out where the bare total leaves
// room for it.
func TestTabsLineKeepsLanguageOverLongCount(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	tabs := tabsOf(s.page.header.value.Kind)
	forms := s.repoCount(s.page.header.value.Profile.Repos)
	bare := forms[len(forms)-1]
	l, ok := s.page.list().(*repoList)
	if !ok {
		t.Skip("repositories list not on view")
	}
	lang := ansi.Strip(s.language(&l.tableTab))
	if lang == "" {
		t.Skip("no language under the cursor")
	}
	long := ansi.StringWidth(s.tabs(tabs, 400, 400))
	found := false
	for w := long; w > 20; w-- {
		line := ansi.Strip(s.tabsLine(w))
		if strings.Contains(line, lang) && strings.Contains(line, " "+bare+"  ") && !strings.Contains(line, forms[0]) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no width keeps %q with the bare total %q", lang, bare)
	}
}
