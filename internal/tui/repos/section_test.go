package repos

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestUpdate(t *testing.T) {
	errDenied := errors.New("403 Forbidden")
	withURL := sampleRepos()
	withURL[0].URL = "https://ghe.example.com/eggzec/gh-tui"

	tests := []struct {
		name  string
		repos []core.Repo
		setup func(f *fakeService, s *Section)
		keys  []string
		send  []tea.Msg
		want  []tea.Msg
		check func(t *testing.T, f *fakeService, s *Section)
	}{
		{
			name: "select shows the pull requests of the repo",
			keys: []string{"down", "enter"},
			want: []tea.Msg{
				ui.RepoMsg{Repo: ref("charmbracelet/bubbletea")},
				ui.ShowMsg{Title: "Pull requests"},
			},
			check: func(t *testing.T, _ *fakeService, s *Section) {
				t.Helper()
				if s.current != ref("charmbracelet/bubbletea") {
					t.Errorf("current = %v, want the selected repo", s.current)
				}
			},
		},
		{
			name: "star an unstarred repo",
			keys: []string{"down", "down", "s"},
			want: []tea.Msg{ui.DoneMsg{What: "star eggzec/dotfiles"}},
			check: func(t *testing.T, f *fakeService, s *Section) {
				t.Helper()
				assertStarred(t, f, s, true)
				if got := f.sentOps(); !slices.Equal(got, []string{"star eggzec/dotfiles"}) {
					t.Errorf("sent %q, want one star", got)
				}
				// Init, the optimistic reload and the one after Done.
				if got := f.listCalls(); got != 3 {
					t.Errorf("List called %d times, want 3", got)
				}
			},
		},
		{
			name: "unstar a starred repo",
			keys: []string{"s"},
			want: []tea.Msg{ui.DoneMsg{What: "unstar eggzec/gh-tui"}},
			check: func(t *testing.T, f *fakeService, s *Section) {
				t.Helper()
				assertStarred(t, f, s, false)
			},
		},
		{
			name:  "a refused star is rolled back",
			setup: func(f *fakeService, _ *Section) { f.sendErr = errDenied },
			keys:  []string{"down", "down", "s"},
			want:  []tea.Msg{ui.DoneMsg{What: "star eggzec/dotfiles", Err: errDenied}},
			check: func(t *testing.T, f *fakeService, s *Section) {
				t.Helper()
				assertStarred(t, f, s, false)
			},
		},
		{
			name:  "open uses the repo's URL",
			repos: withURL,
			keys:  []string{"o"},
			want:  []tea.Msg{ui.OpenMsg{URL: "https://ghe.example.com/eggzec/gh-tui"}},
		},
		{
			name: "open falls back to github.com",
			keys: []string{"down", "o"},
			want: []tea.Msg{ui.OpenMsg{URL: "https://github.com/charmbracelet/bubbletea"}},
		},
		{
			name:  "no selection, no action",
			repos: []core.Repo{},
			keys:  []string{"enter", "s", "o"},
			check: func(t *testing.T, f *fakeService, _ *Section) {
				t.Helper()
				if got := f.sentOps(); len(got) > 0 {
					t.Errorf("sent %q, want nothing", got)
				}
			},
		},
		{
			name:  "blurred, keys do nothing",
			setup: func(_ *fakeService, s *Section) { s.Blur() },
			keys:  []string{"enter", "s", "o"},
			check: func(t *testing.T, f *fakeService, _ *Section) {
				t.Helper()
				if got := f.sentOps(); len(got) > 0 {
					t.Errorf("sent %q, want nothing", got)
				}
			},
		},
		{
			name: "refresh reloads",
			keys: []string{"r"},
			check: func(t *testing.T, f *fakeService, _ *Section) {
				t.Helper()
				if got := f.listCalls(); got != 2 {
					t.Errorf("List called %d times, want 2", got)
				}
			},
		},
		{
			name: "another section's change doesn't reload",
			send: []tea.Msg{ui.DoneMsg{What: "merge #42"}},
			check: func(t *testing.T, f *fakeService, _ *Section) {
				t.Helper()
				if got := f.listCalls(); got != 1 {
					t.Errorf("List called %d times, want 1", got)
				}
			},
		},
		{
			name: "a repo chosen elsewhere becomes current",
			send: []tea.Msg{ui.RepoMsg{Repo: ref("Torvalds/Linux")}},
			check: func(t *testing.T, _ *fakeService, s *Section) {
				t.Helper()
				if got := currentRows(s); !slices.Equal(got, []string{"torvalds/linux"}) {
					t.Errorf("current rows = %q, want torvalds/linux", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos := tt.repos
			if repos == nil {
				repos = sampleRepos()
			}
			f := newFake(repos...)
			s := newSection(t, f, 120, 10)
			if tt.setup != nil {
				tt.setup(f, s)
			}
			got := keys(s, tt.keys...)
			for _, msg := range tt.send {
				got = append(got, run(s, s.Update(msg))...)
			}
			if (len(got) > 0 || len(tt.want) > 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("messages = %#v\nwant %#v", got, tt.want)
			}
			if tt.check != nil {
				tt.check(t, f, s)
			}
		})
	}
}

func TestSelectRunsInOrder(t *testing.T) {
	s := newSection(t, newFake(sampleRepos()...), 80, 10)
	cmd := s.Update(press("enter"))
	cmds, ok := sequence(cmd())
	if !ok || len(cmds) != 2 {
		t.Fatalf("select returned %T, want a sequence of two commands", cmd())
	}
	if got := cmds[0](); got != (ui.RepoMsg{Repo: ref("eggzec/gh-tui")}) {
		t.Errorf("first = %#v, want RepoMsg", got)
	}
	if got := cmds[1](); got != (ui.ShowMsg{Title: "Pull requests"}) {
		t.Errorf("second = %#v, want ShowMsg", got)
	}
}

func TestWithCurrentMarksTheRow(t *testing.T) {
	s := newSection(t, newFake(sampleRepos()...), 80, 10, WithCurrent(ref("eggzec/cli")))
	if got := currentRows(s); !slices.Equal(got, []string{"eggzec/cli"}) {
		t.Errorf("current rows = %q, want eggzec/cli", got)
	}
}

func TestListError(t *testing.T) {
	f := newFake(sampleRepos()...)
	f.listErr = errors.New("API rate limit exceeded")
	s := newSection(t, f, 80, 5)
	if v := ansi.Strip(s.View()); !strings.Contains(v, "API rate limit exceeded · r to retry") {
		t.Errorf("view = %q, want the error with the refresh key", v)
	}
	f.listErr = nil
	keys(s, "r")
	if v := ansi.Strip(s.View()); !strings.Contains(v, "eggzec/gh-tui") {
		t.Errorf("after refresh, view = %q, want the repos", v)
	}
}

func TestHelp(t *testing.T) {
	s := newSection(t, newFake(), 80, 5)
	short := s.Help().ShortHelp()
	descs := make([]string, 0, len(short))
	for _, b := range short {
		descs = append(descs, b.Help().Desc)
	}
	want := []string{"up", "down", "pull requests", "star", "open", "refresh"}
	if !slices.Equal(descs, want) {
		t.Errorf("short help = %q, want %q", descs, want)
	}
	full := slices.Concat(s.Help().FullHelp()...)
	if !slices.ContainsFunc(full, func(b key.Binding) bool { return b.Help().Desc == "page down" }) {
		t.Error("full help lacks the list's navigation")
	}
}

func TestTitle(t *testing.T) {
	if got := New(t.Context(), newFake(), nil).Title(); got != "Repositories" {
		t.Errorf("Title() = %q", got)
	}
}

func TestCompact(t *testing.T) {
	tests := map[int]string{
		0: "0", 999: "999", 1_000: "1k", 1_234: "1.2k", 9_999: "9.9k", 12_345: "12k",
		999_999: "999k", 1_000_000: "1m", 1_250_000: "1.2m", 12_000_000: "12m",
	}
	for n, want := range tests {
		if got := compact(n); got != want {
			t.Errorf("compact(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestLayoutDropsColumns(t *testing.T) {
	tests := []struct {
		width                  int
		desc                   bool
		tag, lang, stars, ages bool
	}{
		{118, true, true, true, true, true},
		{78, true, true, true, true, true},
		{60, true, true, false, true, true},
		{50, true, true, false, true, false},
		{30, false, false, false, true, false},
		{16, false, false, false, false, false},
	}
	for _, tt := range tests {
		l := newLayout(tt.width, leadWidth)
		got := []bool{l.desc > 0, l.tag, l.lang, l.stars, l.ages}
		want := []bool{tt.desc, tt.tag, tt.lang, tt.stars, tt.ages}
		if !slices.Equal(got, want) {
			t.Errorf("newLayout(%d) desc, tag, lang, stars, age = %v, want %v", tt.width, got, want)
		}
		if used := l.lead + l.name + l.right() + descWidth(l); used > tt.width {
			t.Errorf("newLayout(%d) uses %d cells", tt.width, used)
		}
	}
}

func descWidth(l layout) int {
	if l.desc == 0 {
		return 0
	}
	return 2 + l.desc
}

func assertStarred(t *testing.T, f *fakeService, s *Section, want bool) {
	t.Helper()
	r, ok := s.feed.Selected()
	if !ok {
		t.Fatal("nothing selected")
	}
	if f.starred(r.Ref) != want || r.Starred != want {
		t.Errorf("%s starred: service %v, row %v; want %v", r.Ref, f.starred(r.Ref), r.Starred, want)
	}
	glyph := glyphUnstarred
	if want {
		glyph = glyphStarred
	}
	if row := ansi.Strip(s.render(r, true, 118)); !strings.Contains(row, glyph+" "+r.Ref.Owner) {
		t.Errorf("row %q lacks %s", row, glyph)
	}
}

// currentRows returns the repos whose rows carry the current marker.
func currentRows(s *Section) []string {
	var got []string
	for line := range strings.SplitSeq(ansi.Strip(s.View()), "\n") {
		if f := strings.Fields(line); len(f) > 2 && f[0] == glyphCurrent {
			got = append(got, strings.ToLower(f[2]))
		}
	}
	return got
}
