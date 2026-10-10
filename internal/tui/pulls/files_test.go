package pulls

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// hunk returns a hunk of a patch that starts at old and new, whose header
// counts the lines it is given, each led by its sign.
func hunk(old, nw int, section string, lines ...string) string {
	var removed, added, context int
	for _, l := range lines {
		switch l[0] {
		case '-':
			removed++
		case '+':
			added++
		default:
			context++
		}
	}
	head := fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s\n", old, context+removed, nw, context+added, section)
	return head + strings.Join(lines, "\n") + "\n"
}

// mainPatch is the patch of cmd/gh-tui/main.go, with the lines extra, each
// led by its sign, after those it adds.
func mainPatch(extra ...string) string {
	return hunk(10, 10, "func main() {", slices.Concat([]string{
		" \tcfg := load()",
		"-\tsvc := cache.New()",
		"-\trun(svc)",
		"+\tdisk := cache.NewDisk(cfg.Dir)",
		"+\tsvc := cache.New(cache.WithDisk(disk))",
		"+\trun(svc)",
		"+\tdisk.Close()",
	}, extra, []string{
		" }",
		" ",
		" func run(svc *cache.Cache) {",
	})...)
}

// sampleFiles are the files the pull requests of the tests change, in the
// order GitHub lists them: several directories, and a file of each status.
func sampleFiles() []core.CommitFile {
	return []core.CommitFile{
		{Path: "cmd/gh-tui/main.go", Status: core.FileModified, Additions: 4, Deletions: 2, Patch: mainPatch()},
		{Path: "internal/cache/disk.go", Status: core.FileModified, Additions: 3, Deletions: 2, Patch: hunk(41, 41, "func (d *Disk) Get(key string) ([]byte, bool) {",
			" \td.mu.RLock()",
			"-\tb, ok := d.m[key]",
			"+\tb, ok, err := d.read(key)",
			"+\tif err != nil {",
			" \td.mu.RUnlock()",
			" \treturn b, ok",
			" }") + hunk(88, 90, "func (d *Disk) Put(key string, b []byte) error {",
			" \tif len(b) > d.max {",
			"-\t\treturn errTooBig",
			"+\t\treturn fmt.Errorf(\"put %s: %w\", key, errTooBig)",
			" \t}")},
		{Path: "internal/cache/disk_test.go", Status: core.FileAdded, Additions: 5, Patch: hunk(0, 1, "",
			"+package cache",
			"+",
			"+func TestDisk(t *testing.T) {",
			"+\tt.Skip()",
			"+}")},
		{Path: "internal/config/default.yaml", Status: core.FileModified, Additions: 2, Deletions: 1, Patch: hunk(3, 3, "cache:",
			" disk: true",
			"-ttl: 1h",
			"+ttl: 2h",
			"+dir: cache")},
		{Path: "internal/legacy.go", Status: core.FileRemoved, Deletions: 3, Patch: hunk(1, 0, "",
			"-package internal",
			"-",
			"-var Legacy = true")},
		{Path: "docs/design.md", PreviousPath: "docs/notes.md", Status: core.FileRenamed},
		{Path: "img/logo.png", Status: core.FileModified},
	}
}

// filed returns a section over svc, which serves the sample files, and the
// modal of its first pull request, opened on its conversation, at width by
// height. The pull request has a head, which the files are read at.
func filed(tb testing.TB, svc *fakeService, width, height int) (*host, *detailModal) {
	tb.Helper()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	if svc.changed == nil {
		svc.changed = sampleFiles()
	}
	h := started(tb, svc, width, height, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter opened no pull request")
	}
	return h, m
}

// plain returns the view of the modal without styles, as lines.
func plain(m *detailModal) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

func TestFilesTabLoadsOnShow(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	if n := svc.fileReadCount(); n != 0 || m.files != nil {
		t.Fatalf("the modal read %d pages of files before the tab showed, want none", n)
	}
	press(t, h, "]")
	press(t, h, "[")
	if n := svc.fileReadCount(); n != 0 || m.files != nil {
		t.Fatalf("other tabs read %d pages of files, want none", n)
	}
	press(t, h, "[")
	if m.tab != filesTab || m.files == nil {
		t.Fatalf("[ from the conversation showed tab %d with files %v, want the Files tab built", m.tab, m.files)
	}
	if n := svc.fileReadCount(); n != 1 {
		t.Fatalf("showing the tab read %d pages, want 1", n)
	}
	if q := svc.fileReads[0]; q.Head != "a1b2c3d" || q.Number != 142 || q.Cursor != "" {
		t.Errorf("read %+v, want the first page of #142 at its head", q)
	}
	if got := m.files.diff.Files(); got != len(sampleFiles()) {
		t.Errorf("the diff holds %d files, want %d", got, len(sampleFiles()))
	}
	// Leaving and coming back reads nothing more.
	press(t, h, "]")
	press(t, h, "[")
	if n := svc.fileReadCount(); n != 1 {
		t.Errorf("showing the tab again read %d pages, want it kept", n)
	}
}

func TestFilesTabWaitsForTheDetail(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	// A modal opened from the search has nothing until the detail arrives.
	m.loaded, m.detail = false, core.PullRequestDetail{}
	press(t, h, "[")
	if m.files != nil || svc.fileReadCount() != 0 {
		t.Fatalf("the tab read files with no head: %v, %d reads", m.files, svc.fileReadCount())
	}
	if v := plain(m); !strings.Contains(strings.Join(v, "\n"), "Loading") {
		t.Errorf("the tab shows no loading:\n%s", strings.Join(v, "\n"))
	}
	drain(t, h, m.Update(detailMsg{thread: m.thread.ID(), detail: svc.detail(142)}))
	if m.files == nil || svc.fileReadCount() != 1 {
		t.Errorf("the detail started no read of files: %v, %d reads", m.files, svc.fileReadCount())
	}
}

// pathOf returns the path of the node under the tree's cursor.
func pathOf(m *detailModal) string {
	n, _ := m.files.tree.Selected()
	return n.ID
}

func TestTreeFollowsDiff(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	if got := pathOf(m); got != "cmd/gh-tui/main.go" {
		t.Fatalf("the tree is on %q, want the first file", got)
	}
	press(t, h, "J")
	if got := pathOf(m); got != "internal/cache/disk.go" {
		t.Errorf("J left the tree on %q, want the second file of the diff", got)
	}
	press(t, h, "J")
	press(t, h, "J")
	if got := pathOf(m); got != "internal/config/default.yaml" {
		t.Errorf("three J left the tree on %q, want the fourth file", got)
	}
	press(t, h, "K")
	press(t, h, "K")
	if got := pathOf(m); got != "internal/cache/disk.go" {
		t.Errorf("K left the tree on %q, want the second file", got)
	}
	// Moving through the lines of a file into the next follows too.
	press(t, h, "G")
	if got := pathOf(m); got != "img/logo.png" {
		t.Errorf("G left the tree on %q, want the last file of the diff", got)
	}
}

func TestEnterOnAFileShowsItsDiff(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "1")
	if m.files.focus != treePane {
		t.Fatal("1 didn't focus the tree")
	}
	for _, k := range []string{"j", "j", "enter"} {
		press(t, h, k)
	}
	if m.files.focus != diffPane {
		t.Error("enter on a file didn't focus the diff")
	}
	if cur, _ := m.files.diff.CurrentFile(); cur.Path != "docs/design.md" || pathOf(m) != cur.Path {
		t.Errorf("the diff is in %q and the tree on %q, want both on docs/design.md", cur.Path, pathOf(m))
	}
	// l on a file does the same.
	press(t, h, "1")
	press(t, h, "G")
	press(t, h, "l")
	if m.files.focus != diffPane {
		t.Error("l on a file didn't focus the diff")
	}
	if cur, _ := m.files.diff.CurrentFile(); cur.Path != pathOf(m) {
		t.Errorf("the diff is in %q, the tree on %q", cur.Path, pathOf(m))
	}
}

func TestTreeFolds(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "1")
	rows := m.files.tree.Len()
	press(t, h, "g")
	press(t, h, "h")
	if m.files.tree.Len() >= rows {
		t.Errorf("h folded nothing: %d rows of %d", m.files.tree.Len(), rows)
	}
	press(t, h, "l")
	if got := m.files.tree.Len(); got != rows {
		t.Errorf("l left %d rows, want the %d it opened with", got, rows)
	}
	press(t, h, "*")
	if got := m.files.tree.Len(); got != 4 {
		t.Errorf("* left %d rows, want the 4 directories of the top", got)
	}
	press(t, h, "*")
	if got := m.files.tree.Len(); got != rows {
		t.Errorf("* again left %d rows, want all %d", got, rows)
	}
}

// hasRow reports whether a line is the row of a directory in a tree: its
// mark and name alone.
func hasRow(lines []string, name string) bool {
	return slices.ContainsFunc(lines, func(l string) bool {
		return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "▌▾▸ ")) == name
	})
}

func TestNarrowBreadcrumbAndTab(t *testing.T) {
	h, m := filed(t, newFakeService(), 76, 20)
	press(t, h, "[")
	v := plain(m)
	if want := "1/7 › cmd/gh-tui/main.go  M +4 −2"; !strings.HasPrefix(v[0], want) {
		t.Errorf("breadcrumb = %q, want it to start with %q", v[0], want)
	}
	if hasRow(v, "docs") {
		t.Errorf("a narrow modal shows the tree beside the diff:\n%s", strings.Join(v, "\n"))
	}
	press(t, h, "J")
	if want := "2/7 › internal/cache/disk.go  M +3 −2"; !strings.HasPrefix(plain(m)[0], want) {
		t.Errorf("breadcrumb = %q after J, want %q", plain(m)[0], want)
	}
	press(t, h, "tab")
	v = plain(m)
	if m.files.focus != treePane || !hasRow(v[1:], "docs") || strings.Contains(strings.Join(v[1:], "\n"), "@@") {
		t.Errorf("tab didn't swap the diff for the tree:\n%s", strings.Join(v, "\n"))
	}
	if !strings.HasPrefix(v[0], "2/7") {
		t.Errorf("the breadcrumb lost the file with the tree shown: %q", v[0])
	}
	press(t, h, "tab")
	if m.files.focus != diffPane || !strings.Contains(strings.Join(plain(m), "\n"), "@@") {
		t.Error("a second tab didn't bring the diff back")
	}
	// z is for the panes that fit side by side.
	press(t, h, "z")
	if m.files.zoom {
		t.Error("z zoomed a pane of a narrow modal")
	}
	for i, l := range plain(m) {
		if w := ansi.StringWidth(l); w != 76 {
			t.Fatalf("line %d is %d cells wide, want 76", i, w)
		}
	}
}

func TestDigitsOnlyOnFiles(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	for _, k := range []string{"1", "2"} {
		press(t, h, k)
		if m.tab != conversationTab || m.files != nil {
			t.Fatalf("%s on the conversation did something: tab %d, files %v", k, m.tab, m.files)
		}
	}
	press(t, h, "]")
	for _, k := range []string{"1", "2"} {
		press(t, h, k)
		if m.tab != checksTab || m.files != nil {
			t.Fatalf("%s on the checks did something: tab %d, files %v", k, m.tab, m.files)
		}
	}
	press(t, h, "]")
	if m.files.focus != diffPane {
		t.Fatalf("the Files tab opened on pane %d, want the diff", m.files.focus)
	}
	press(t, h, "1")
	if m.files.focus != treePane {
		t.Error("1 didn't focus the tree on the Files tab")
	}
	press(t, h, "2")
	if m.files.focus != diffPane {
		t.Error("2 didn't focus the diff on the Files tab")
	}
	if !slices.Contains(uitest.Enabled(m.KeyLayers()), "focus pane") {
		t.Error("help doesn't offer the pane keys on the Files tab")
	}
	press(t, h, "]")
	press(t, h, "2")
	if m.files.focus != diffPane || m.tab != conversationTab {
		t.Error("2 on the conversation moved something")
	}
	if slices.Contains(uitest.Enabled(m.KeyLayers()), "focus pane") {
		t.Error("help offers the pane keys on the conversation")
	}
}

func TestZoomHidesTheTree(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "│") || !strings.Contains(v, "Files") {
		t.Fatalf("a wide modal shows no tree beside the diff:\n%s", v)
	}
	press(t, h, "2")
	press(t, h, "z")
	if !m.files.zoom {
		t.Fatal("z didn't zoom")
	}
	v := plain(m)
	if strings.Contains(strings.Join(v, "\n"), "│") || strings.HasPrefix(v[0], "Files") {
		t.Errorf("the tree shows with the diff zoomed:\n%s", strings.Join(v, "\n"))
	}
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc didn't close the modal from a zoomed pane")
	}
}

func TestResizeAcrossTheBreakKeepsFocus(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "1")
	m.SetSize(80, 30)
	if m.files.focus != treePane || !strings.HasPrefix(plain(m)[0], "1/7") {
		t.Errorf("narrow: focus %d, first line %q, want the tree with a breadcrumb", m.files.focus, plain(m)[0])
	}
	m.SetSize(100, 30)
	if m.files.focus != treePane || !strings.HasPrefix(plain(m)[0], "Files") {
		t.Errorf("wide again: focus %d, first line %q, want the tree and two panes", m.files.focus, plain(m)[0])
	}
}

func TestHeadMovedKeepsPos(t *testing.T) {
	svc := newFakeService()
	moved := sampleFiles()
	// The new head adds a line to the file above the one the cursor is in,
	// which moves the cursor's row down but not its line.
	moved[0].Patch = mainPatch("+\tlog.Print(\"started\")")
	moved[0].Additions++
	svc.changedAt = map[string][]core.CommitFile{"b2c3d4e": moved}
	h, m := filed(t, svc, 100, 30)
	press(t, h, "[")
	press(t, h, "J")
	press(t, h, "j")
	press(t, h, "j")
	want, ok := m.files.diff.Position()
	if !ok || want.Path != "internal/cache/disk.go" {
		t.Fatalf("the cursor is at %+v (%v), want a line of disk.go", want, ok)
	}
	row := m.files.diff.Cursor()
	press(t, h, "1")
	press(t, h, "z")
	reads := svc.fileReadCount()

	// A push moves the head; the sync that finds it reads the files again.
	svc.pulls[0].HeadSHA = "b2c3d4e"
	drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if svc.fileReadCount() != reads+1 || svc.fileReads[reads].Head != "b2c3d4e" {
		t.Fatalf("reads = %+v, want the first page again at the new head", svc.fileReads[reads:])
	}
	got, ok := m.files.diff.Position()
	if !ok || got != want {
		t.Errorf("the cursor is at %+v (%v) after the head moved, want %+v", got, ok, want)
	}
	if m.files.diff.Cursor() != row+1 {
		t.Errorf("the cursor is on row %d, want %d: the line moved down a row with the one added above", m.files.diff.Cursor(), row+1)
	}
	if got := pathOf(m); got != "internal/cache/disk.go" {
		t.Errorf("the tree is on %q after the head moved, want the file of the cursor", got)
	}
	if m.files.focus != treePane || !m.files.zoom {
		t.Errorf("the focus %d and the zoom %v changed with the head, want the tree zoomed", m.files.focus, m.files.zoom)
	}

	// Another sync with the same head reads nothing.
	drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if svc.fileReadCount() != reads+1 {
		t.Error("a sync that left the head where it was read the files again")
	}

	// A file that is gone leaves the cursor at the top.
	svc.changedAt["c3d4e5f"] = moved[2:]
	svc.pulls[0].HeadSHA = "c3d4e5f"
	drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if m.files.diff.Files() != len(moved)-2 {
		t.Fatalf("the diff holds %d files, want %d", m.files.diff.Files(), len(moved)-2)
	}
	if cur, _ := m.files.diff.CurrentFile(); cur.Path != "internal/cache/disk_test.go" {
		t.Errorf("the cursor is in %q, want the first file left", cur.Path)
	}
}

func TestHeadMovedWhileAnotherTabShows(t *testing.T) {
	svc := newFakeService()
	svc.changedAt = map[string][]core.CommitFile{"b2c3d4e": sampleFiles()[:3]}
	h, m := filed(t, svc, 100, 30)
	press(t, h, "[")
	reads := svc.fileReadCount()
	press(t, h, "]")
	svc.pulls[0].HeadSHA = "b2c3d4e"
	drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if svc.fileReadCount() != reads {
		t.Fatal("the files were read again while the conversation showed")
	}
	press(t, h, "[")
	if svc.fileReadCount() != reads+1 || m.files.diff.Files() != 3 {
		t.Errorf("showing the tab again: %d reads, %d files, want the new head's 3 files", svc.fileReadCount()-reads, m.files.diff.Files())
	}
}

func TestFilesEmptyAndError(t *testing.T) {
	t.Run("no files", func(t *testing.T) {
		svc := newFakeService()
		svc.changed = []core.CommitFile{}
		h, m := filed(t, svc, 100, 30)
		press(t, h, "[")
		v := strings.Join(plain(m), "\n")
		if strings.Count(v, "No files changed.") != 2 {
			t.Errorf("an empty pull request shows:\n%s", v)
		}
		for _, k := range []string{"J", "G", "1", "l", "z", "2"} {
			press(t, h, k)
		}
	})
	t.Run("error and retry", func(t *testing.T) {
		svc := newFakeService()
		svc.filesErr = errors.New("boom")
		h, m := filed(t, svc, 100, 30)
		press(t, h, "[")
		v := strings.Join(plain(m), "\n")
		if !strings.Contains(v, "Couldn't load files: boom") || !strings.Contains(v, "r to retry") {
			t.Fatalf("the failure isn't shown with how to retry:\n%s", v)
		}
		if strings.Contains(v, "No files changed.") {
			t.Errorf("the tree claims there are no files beside the failure:\n%s", v)
		}
		reads := svc.fileReadCount()
		svc.filesErr = nil
		press(t, h, "1")
		press(t, h, "r")
		if svc.fileReadCount() != reads+1 || m.files.diff.Files() != len(sampleFiles()) {
			t.Errorf("r read %d pages and left %d files, want one read and all files", svc.fileReadCount()-reads, m.files.diff.Files())
		}
		if v := strings.Join(plain(m), "\n"); strings.Contains(v, "Couldn't load") {
			t.Errorf("the failure stays after a retry:\n%s", v)
		}
	})
	t.Run("more than GitHub lists", func(t *testing.T) {
		svc := newFakeService()
		svc.pulls[0].ChangedFiles = core.MaxPullFiles + 200
		h, m := filed(t, svc, 100, 30)
		press(t, h, "[")
		press(t, h, "G")
		if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "first 3000 files") {
			t.Errorf("the end of the diff has no note about the files left out:\n%s", v)
		}
	})
	t.Run("paged", func(t *testing.T) {
		svc := newFakeService()
		svc.filePage = 3
		h, m := filed(t, svc, 100, 12)
		press(t, h, "[")
		press(t, h, "G")
		press(t, h, "G")
		if got := m.files.diff.Files(); got != len(sampleFiles()) {
			t.Errorf("the diff holds %d files after paging to the end, want %d", got, len(sampleFiles()))
		}
		if m.files.tree.Len() < 7 {
			t.Errorf("the tree holds %d rows, want the files of every page", m.files.tree.Len())
		}
	})
}

func TestOpenFileOnGitHub(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "J")
	opened := func() string {
		for _, msg := range press(t, h, "o") {
			if o, ok := msg.(ui.OpenMsg); ok {
				return o.URL
			}
		}
		return ""
	}
	want := "https://github.com/eggzec/gh-tui/pull/142/files#" + ui.DiffAnchor("internal/cache/disk.go")
	if got := opened(); got != want {
		t.Errorf("o opened %q, want %q", got, want)
	}
	// With the tree focused, the file of the tree's cursor.
	press(t, h, "1")
	press(t, h, "j")
	want = "https://github.com/eggzec/gh-tui/pull/142/files#" + ui.DiffAnchor("internal/cache/disk_test.go")
	if got := opened(); got != want {
		t.Errorf("o with the tree focused opened %q, want %q", got, want)
	}
	// A directory has no diff of its own.
	press(t, h, "g")
	press(t, h, "k")
	for pathOf(m) != "cmd" {
		press(t, h, "k")
	}
	if got := opened(); got != "https://github.com/eggzec/gh-tui/pull/142/files" {
		t.Errorf("o on a directory opened %q, want the page of the files", got)
	}
}

// filesView returns the view of the Files tab of a modal width by height,
// with the cursor on a line of the second file.
func filesView(width, height int, icons string) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()
		svc := newFakeService()
		svc.pulls[0].HeadSHA = "a1b2c3d"
		svc.changed = sampleFiles()
		h := started(t, svc, width, height, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(icons)))
		press(t, h, "enter")
		press(t, h, "[")
		press(t, h, "J")
		press(t, h, "j")
		press(t, h, "j")
		return h.modal().View()
	}
}

func TestFilesOnlineRetriesOnce(t *testing.T) {
	svc := newFakeService()
	svc.filesErr = core.ErrOffline
	h, m := filed(t, svc, 100, 30)
	press(t, h, "[")
	reads := svc.fileReadCount()
	svc.filesErr = nil
	drain(t, h, h.Update(ui.OnlineMsg{}))
	drain(t, h, h.Update(ui.OnlineMsg{}))
	if got := svc.fileReadCount() - reads; got != 1 || m.files.diff.Files() != len(sampleFiles()) {
		t.Errorf("two OnlineMsg read %d pages and left %d files, want one read and all the files", got, m.files.diff.Files())
	}
}

func TestFilesHostilePaths(t *testing.T) {
	// bidi reverses the text after it.
	bidi := string(rune(0x202e))
	svc := newFakeService()
	svc.changed = []core.CommitFile{
		{Path: "src/\x1b[31mred\nline.go", Status: core.FileModified, Additions: 1, Patch: "@@ -0,0 +1 @@\n+\x1b]0;title\x07x\n"},
		{Path: "src/\x1b[2Jclear.go", PreviousPath: "src/" + bidi + "old.go", Status: core.FileRenamed},
	}
	for _, width := range []int{80, 100} {
		h, m := filed(t, svc, width, 20)
		press(t, h, "[")
		for _, k := range []string{"J", "1", "j", "2"} {
			press(t, h, k)
			v := m.View()
			for _, bad := range []string{"\x1b[31mred", "\x1b[2J", "\x1b]0;", "\a", bidi} {
				if strings.Contains(v, bad) {
					t.Fatalf("at %d after %s the view holds %q", width, k, bad)
				}
			}
			for i, l := range strings.Split(v, "\n") {
				if w := ansi.StringWidth(l); w != width {
					t.Fatalf("at %d after %s line %d is %d cells wide", width, k, i, w)
				}
			}
		}
	}
}

func TestFilesTabWithoutTheDetail(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	// The detail failed to load, so there is no head to read the files at.
	m.loaded, m.detail, m.failed = false, core.PullRequestDetail{}, errors.New("boom")
	press(t, h, "[")
	if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "Couldn't load the pull request: boom") || !strings.Contains(v, "r to retry") {
		t.Errorf("the failure of the detail isn't shown with how to retry:\n%s", v)
	}
	// Retrying reads the detail again, which starts the files.
	press(t, h, "r")
	if m.files == nil || m.files.diff.Files() != len(sampleFiles()) {
		t.Errorf("r didn't read the detail and then the files: %v", m.files)
	}

	h, m = filed(t, newFakeService(), 100, 30)
	m.detail.HeadSHA = ""
	press(t, h, "[")
	if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "no head commit") || strings.Contains(v, "Loading") {
		t.Errorf("a pull request without a head shows:\n%s", v)
	}
}

func TestFilesKeptPagesReadAgainOnline(t *testing.T) {
	svc := newFakeService()
	svc.filesKept = true
	h, _ := filed(t, svc, 100, 30)
	press(t, h, "[")
	reads := svc.fileReadCount()
	svc.filesKept = false
	drain(t, h, h.Update(ui.OnlineMsg{}))
	if got := svc.fileReadCount() - reads; got != 1 {
		t.Fatalf("GitHub answering again read %d pages, want the kept one read again", got)
	}
	drain(t, h, h.Update(ui.OnlineMsg{}))
	if got := svc.fileReadCount() - reads; got != 1 {
		t.Errorf("a second OnlineMsg read again: %d reads", got)
	}
}

// A kept page is read again once the tab shows, when GitHub answered
// while another tab did.
func TestFilesKeptPagesWaitForTheTab(t *testing.T) {
	svc := newFakeService()
	svc.filesKept = true
	h, _ := filed(t, svc, 100, 30)
	press(t, h, "[")
	press(t, h, "]")
	svc.filesKept = false
	reads := svc.fileReadCount()
	drain(t, h, h.Update(ui.OnlineMsg{}))
	if svc.fileReadCount() != reads {
		t.Fatal("the files were read again while the conversation showed")
	}
	press(t, h, "[")
	if got := svc.fileReadCount() - reads; got != 1 {
		t.Errorf("showing the tab read %d pages, want the kept one read again", got)
	}
}

func TestQuitAndBackOnFiles(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "backspace")
	if h.modal() != m || m.tab != filesTab {
		t.Error("backspace did something on the Files tab")
	}
	if _, ok := m.Act(ui.ActBack); ok {
		t.Error("the Files tab took the back key from the app")
	}
	if _, ok := m.Act(ui.ActQuit); !ok || !m.closed {
		t.Error("q didn't close the modal from the Files tab")
	}
}

func TestRefreshReadsTheFilesAgain(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	press(t, h, "[")
	press(t, h, "J")
	reads, want := svc.fileReadCount(), mustPos(t, m)
	press(t, h, "r")
	if len(svc.invalidated) == 0 || svc.fileReadCount() != reads+1 {
		t.Errorf("r invalidated %d times and read %d pages, want it to read the files again", len(svc.invalidated), svc.fileReadCount()-reads)
	}
	if got := mustPos(t, m); got != want {
		t.Errorf("the cursor is at %+v after r, want %+v", got, want)
	}
}

// mustPos returns the file the cursor of the diff is in.
func mustPos(t *testing.T, m *detailModal) string {
	t.Helper()
	cur, ok := m.files.diff.CurrentFile()
	if !ok {
		t.Fatal("the cursor is in no file")
	}
	return cur.Path
}

// / searches the lines of the diff, and what is typed there stays out of
// the modal's keys; n and N go through the matches, and esc clears the
// search before it closes the modal.
func TestSearchTheDiff(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "2")
	press(t, h, "/")
	if !m.files.diff.Capturing() {
		t.Fatal("/ didn't open the search")
	}
	layers := m.KeyLayers()
	if len(layers) != 1 || !layers[0].Typing || layers[0].Context != "search_prompt" {
		t.Fatalf("the layers while typing are %+v, want the prompt's alone", layers)
	}
	// C, M, ] and q are keys of the modal when nothing is typed.
	for _, k := range []string{"C", "M", "]", "q", "X"} {
		press(t, h, k)
	}
	if m.tab != filesTab || m.ask != nil || m.closed {
		t.Fatalf("a typed key did something: tab %v, ask %v, closed %v", m.tab, m.ask, m.closed)
	}
	press(t, h, "esc")
	if m.files.diff.Capturing() || h.modal() != m {
		t.Fatal("esc did not close the input alone")
	}

	press(t, h, "/")
	for _, k := range []string{"d", "i", "s", "k"} {
		press(t, h, k)
	}
	press(t, h, "enter")
	d := &m.files.diff
	if d.Query() != "disk" || d.Matches() < 3 {
		t.Fatalf("query %q found %d matches, want several across files", d.Query(), d.Matches())
	}
	first, _ := d.CurrentFile()
	var last diff.File
	for range d.Matches() {
		press(t, h, "n")
		last, _ = d.CurrentFile()
		if last.Path != first.Path {
			break
		}
	}
	if last.Path == first.Path {
		t.Error("n stayed in one file")
	}
	press(t, h, "N")
	if cur, _ := d.CurrentFile(); cur.Path != first.Path {
		t.Errorf("N went to %q, want back in %q", cur.Path, first.Path)
	}
	if !strings.Contains(strings.Join(plain(m), "\n"), "match ") {
		t.Error("the view doesn't say which match it is on")
	}

	press(t, h, "esc")
	if d.Query() != "" || h.modal() != m || m.closed {
		t.Fatalf("esc: query %q, modal open %v; want the search cleared and the modal open", d.Query(), h.modal() == m)
	}
	press(t, h, "esc")
	if h.modal() == m {
		t.Error("a second esc did not close the modal")
	}
}

// With the tree focused, esc still clears a search of the diff first.
func TestSearchClearedFromTheTree(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "2")
	for _, k := range []string{"/", "d", "i", "s", "k", "enter", "1", "esc"} {
		press(t, h, k)
	}
	if m.files.diff.Query() != "" || h.modal() != m {
		t.Error("esc did not clear the search first")
	}
}
