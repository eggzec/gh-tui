package pulls

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// onFilesWith returns the modal on its Files tab, 116 by 60, whose pull
// request has the threads ths, all on one page.
func onFilesWith(t *testing.T, ths ...core.Thread) (*host, *detailModal) {
	t.Helper()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: ths, Total: len(ths)}}
	h, m := threaded(t, svc, 116, 60, config.IconsUnicode)
	drain(t, h, m.switchTo(filesTab))
	if m.files == nil {
		t.Fatal("the Files tab was not built")
	}
	return h, m
}

// diffLines returns the lines of the diff, without styles.
func diffLines(m *detailModal) []string {
	return strings.Split(ansi.Strip(m.files.diff.View()), "\n")
}

// lineWith returns the index of the first line that holds want, or -1.
func lineWith(lines []string, want string) int {
	return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, want) })
}

func TestFileThreadsPlacement(t *testing.T) {
	left := review("tl", "cmd/gh-tui/main.go", 11, "octocat", "Why drop this?")
	left.Side = core.SideLeft
	outdated := review("t2", "internal/legacy.go", 0, "monalisa", "This is gone in the next commit.")
	outdated.Outdated, outdated.OriginalLine = true, 12
	file := review("t3", "docs/design.md", 0, "octocat", "Needs a section on the disk layout.")
	file.File = true
	file.Outdated = true
	done := review("t4", "internal/config/default.yaml", 4, "hubot", "Seconds would do.")
	done.Resolved = true
	_, m := onFilesWith(t,
		review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk.", reply("monalisa", "Fixed, it is returned now.")),
		left, outdated, file, done)
	lines := diffLines(m)
	at := func(text string) int {
		t.Helper()
		i := lineWith(lines, text)
		if i < 0 {
			t.Fatalf("the diff has no line with %q:\n%s", text, strings.Join(lines, "\n"))
		}
		return i
	}
	if i := at("Swallowing err"); !strings.Contains(lines[i-1], "return b, ok") {
		t.Errorf("a thread on line 45 follows %q, want the line", lines[i-1])
	}
	if i := at("1 reply"); i != at("Swallowing err")+1 {
		t.Errorf("the count of replies is on line %d, want right under the first comment", i)
	}
	if i := at("Why drop this?"); !strings.Contains(lines[i-1], "svc := cache.New()") || !strings.Contains(lines[i-1], "-") {
		t.Errorf("a thread on the deleted side follows %q, want the deleted line", lines[i-1])
	}
	if i := at("This is gone"); !strings.Contains(lines[i], "outdated") {
		t.Errorf("an outdated thread reads %q, want its tag", lines[i])
	}
	if i := at("Needs a section"); strings.Contains(lines[i], "outdated") {
		t.Errorf("a thread on a file reads %q, want no outdated tag", lines[i])
	}
	if i := at("Seconds would do."); !strings.Contains(lines[i], "resolved") || strings.Contains(lines[i+1], "reply") {
		t.Errorf("a resolved thread reads %q, want one line marked resolved", lines[i])
	}
	if got := m.files.diff.HeaderBlocks(); !slices.Equal(got, []string{"t2", "t3"}) {
		t.Errorf("the blocks under the headers are %v, want the outdated thread and the one on a file", got)
	}
}

func TestFileThreadsEnterExpands(t *testing.T) {
	h, m := onFilesWith(t, review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk.", reply("monalisa", "Fixed, it is returned now.")))
	if !m.files.diff.SeekBlock("t1") {
		t.Fatal("the thread is not in the diff")
	}
	if v := strings.Join(diffLines(m), "\n"); strings.Contains(v, "Fixed, it is returned now.") {
		t.Errorf("a folded thread shows its reply:\n%s", v)
	}
	press(t, h, "enter")
	if v := strings.Join(diffLines(m), "\n"); !strings.Contains(v, "Fixed, it is returned now.") {
		t.Errorf("enter on the thread showed no reply:\n%s", v)
	}
	press(t, h, "enter")
	if v := strings.Join(diffLines(m), "\n"); strings.Contains(v, "Fixed, it is returned now.") {
		t.Errorf("enter again left the reply shown:\n%s", v)
	}
}

func TestFileThreadsTreeCount(t *testing.T) {
	open := review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err.")
	done := review("t2", "internal/cache/disk.go", 3, "hubot", "Unused.")
	done.Resolved = true
	_, m := onFilesWith(t, open, done)
	f := m.files
	icon := func(path string) string {
		return f.icon(tree.Node{ID: path, Value: diff.File{Path: path, Status: diff.StatusModified}}, false)
	}
	dot := m.icons.Dot
	if got, want := icon("internal/cache/disk.go"), f.mark(diff.StatusModified)+" "+m.theme.Error.Render(dot+"2"); got != want {
		t.Errorf("a file with an unresolved thread has %q, want %q", got, want)
	}
	if got := ansi.Strip(icon("cmd/gh-tui/main.go")); got != "M" {
		t.Errorf("a file with no thread has %q, want only its letter", got)
	}
	f.counts["internal/cache/disk.go"] = threadCount{n: 2}
	if got, want := icon("internal/cache/disk.go"), f.mark(diff.StatusModified)+" "+m.theme.Subtle.Render(dot+"2"); got != want {
		t.Errorf("a file whose threads are resolved has %q, want %q", got, want)
	}
}

func TestFileThreadsOpenOnGitHub(t *testing.T) {
	th := review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err.")
	th.Comments[0].URL = "https://github.com/eggzec/gh-tui/pull/142#discussion_r1"
	h, m := onFilesWith(t, th)
	m.files.diff.SeekBlock("t1")
	var got string
	for _, msg := range press(t, h, "o") {
		if o, ok := msg.(ui.OpenMsg); ok {
			got = o.URL
		}
	}
	if got != th.Comments[0].URL {
		t.Errorf("o on a thread opened %q, want its comment", got)
	}
}

func TestFileThreadsReadEveryPage(t *testing.T) {
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage(), "p2": secondPage()}
	h, m := threaded(t, svc, 116, 60, config.IconsUnicode)
	drain(t, h, m.switchTo(filesTab))
	if n := svc.threadReadCount(); n != 2 {
		t.Errorf("the Files tab read %d pages of threads, want both", n)
	}
	if got := len(m.th.all); got != 5 {
		t.Errorf("%d threads were read, want 5, resolved ones too", got)
	}
	if got := len(m.th.list); got != 4 {
		t.Errorf("the conversation lists %d threads, want the 4 unresolved", got)
	}
}
