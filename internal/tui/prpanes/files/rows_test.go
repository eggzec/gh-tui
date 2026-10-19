package files

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

var now = epoch.AddDate(0, 0, 20)

func theme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// opts returns the options of rows drawn in width cells with the icon set.
func opts(set string, width int) Options {
	return Options{Width: width, Styles: NewStyles(theme(), ui.NewIcons(set), set), Now: now}
}

// pull is a pull request of n commits, a day apart, by three authors, with
// the checks of some of them.
func pull(n int) core.PullCommits {
	authors := []string{"Ada Lovelace", "Grace Hopper", "Linus"}
	subjects := []string{
		"Add the cache",
		"Fix the off by one error in the pager when the last line is wrapped",
		"Rename the helpers",
		"Document the flags",
	}
	cs := make([]core.PullCommit, n)
	for i := range cs {
		cs[i] = core.PullCommit{
			OID:         fmt.Sprintf("%07x%033x", 0xa1b2c30+i*0x111, i),
			Headline:    subjects[i%len(subjects)],
			Author:      authors[i%len(authors)],
			CommittedAt: epoch.AddDate(0, 0, i),
		}
		switch i % 4 {
		case 0:
			cs[i].Checks = core.CheckPassing
		case 1:
			cs[i].Checks = core.CheckFailing
		case 2:
			cs[i].Checks = core.CheckPending
		}
	}
	return core.PullCommits{Commits: cs}
}

func ids(rows []foldtree.Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// text draws rows in a tree of width cells, without colors, with the Commits
// section opened when open is set.
func text(rows []foldtree.Row, width int, open bool) string {
	m := foldtree.New(foldtree.WithSize(width, 60), foldtree.WithRows(rows))
	if open {
		m.SetFolded(IDCommits, false)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func TestRowsGolden(t *testing.T) {
	t.Parallel()
	review := func(pc core.PullCommits, oid int) core.PullCommits {
		pc.LastReview = &core.ViewerReview{Commit: pc.Commits[oid].OID, SubmittedAt: epoch.AddDate(0, 0, oid)}
		return pc
	}
	long := pull(3)
	long.Commits[0].Author = "Bartholomew Maximilian Featherstonehaugh-Smythe"
	long.Commits[1].Author = ""
	none := pull(4)
	for i := range none.Commits {
		none.Commits[i].Checks = core.CheckNone
	}
	capped := pull(5)
	capped.Capped = true

	for _, tc := range []struct {
		name   string
		d      Data
		folded bool
	}{
		{"all-selected", Data{Commits: pull(6), Range: All(6)}, false},
		{"folded-all", Data{Commits: pull(6), Range: All(6)}, true},
		{"folded-range", Data{Commits: pull(6), Range: Range{1, 3}}, true},
		{"sub-range", Data{Commits: pull(6), Range: Range{1, 3}}, false},
		{"single", Data{Commits: pull(6), Range: Range{4, 4}}, false},
		{"divider", Data{Commits: review(pull(6), 2), Range: Range{3, 5}}, false},
		{"capped", Data{Commits: capped, Range: All(5)}, false},
		{"no-checks", Data{Commits: none, Range: All(4)}, false},
		{"long-author", Data{Commits: long, Range: All(3)}, false},
		{"empty", Data{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sb strings.Builder
			for _, w := range []int{80, 50, 30} {
				o := opts(config.IconsUnicode, w)
				o.Folded = tc.folded
				fmt.Fprintf(&sb, "width %d\n", w)
				sb.WriteString(text(Rows(tc.d, o), w, !tc.folded))
				sb.WriteString("\n")
			}
			golden.RequireEqual(t, sb.String())
		})
	}
}

// changed are the files of a range, not in the order of their paths.
func changed() []core.CommitFile {
	return []core.CommitFile{
		{Path: "internal/tui/pulls/panes.go", Additions: 120, Deletions: 7},
		{Path: "README.md", Additions: 3},
		{Path: "cmd/gh-tui/main.go", Deletions: 14},
		{Path: "internal/a-very-long-directory-name/with/more/levels/than/fit/in/the/pane/file.go", Additions: 1, Deletions: 1},
		{Path: "assets/logo.png"},
	}
}

// viewed marks some of changed(): one viewed, one that changed since.
func viewed(supported bool) core.PullViewed {
	return core.PullViewed{
		Supported: supported,
		States: map[string]core.ViewedState{
			"README.md":                   core.Viewed,
			"cmd/gh-tui/main.go":          core.ViewedChanged,
			"internal/tui/pulls/panes.go": core.Viewed,
		},
	}
}

func TestRowsFilesGolden(t *testing.T) {
	t.Parallel()
	pc := pull(2)
	for _, tc := range []struct {
		name string
		d    Data
	}{
		{"rows-files-tree", Data{Commits: pc, Range: All(2), Files: changed()}},
		{"rows-files-viewed", Data{Commits: pc, Range: All(2), Files: changed(), Viewed: viewed(true)}},
		{"rows-files-findings", Data{Commits: pc, Range: All(2), Files: changed(), Viewed: viewed(true), Findings: findings()}},
		{"rows-files-viewed-unsupported", Data{Commits: pc, Range: All(2), Files: changed(), Viewed: viewed(false)}},
		{"rows-files-truncated", Data{Commits: pc, Range: All(2), Files: changed(), Truncated: "first 300 files of this range"}},
		{"rows-files-error", Data{Commits: pc, Range: All(2), Files: changed(), FilesErr: "these commits are no longer on the branch"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sb strings.Builder
			for _, w := range []int{80, 40} {
				o := opts(config.IconsUnicode, w)
				o.Folded = true
				fmt.Fprintf(&sb, "width %d\n", w)
				sb.WriteString(text(Rows(tc.d, o), w, false))
				sb.WriteString("\n")
			}
			golden.RequireEqual(t, sb.String())
		})
	}
}

// findings are what the checks found in some of changed(): a viewed file with
// a failure among its notes, one that changed since viewed with a warning, a
// viewed one with a notice, and a file that isn't viewed with several failures.
func findings() map[string]Finding {
	return Findings([]core.Annotation{
		{Path: "internal/tui/pulls/panes.go", Level: core.AnnotationNotice},
		{Path: "internal/tui/pulls/panes.go", Level: core.AnnotationFailure},
		{Path: "internal/tui/pulls/panes.go", Level: core.AnnotationWarning},
		{Path: "cmd/gh-tui/main.go", Level: core.AnnotationWarning},
		{Path: "README.md", Level: core.AnnotationNotice},
		{Path: "README.md", Level: core.AnnotationNotice},
		{Path: "internal/a-very-long-directory-name/with/more/levels/than/fit/in/the/pane/file.go", Level: core.AnnotationFailure},
	})
}

func TestFindings(t *testing.T) {
	t.Parallel()
	got := findings()
	for path, want := range map[string]Finding{
		"internal/tui/pulls/panes.go": {Count: 3, Level: core.AnnotationFailure},
		"cmd/gh-tui/main.go":          {Count: 1, Level: core.AnnotationWarning},
		"README.md":                   {Count: 2, Level: core.AnnotationNotice},
	} {
		if got[path] != want {
			t.Errorf("finding of %s = %+v, want %+v", path, got[path], want)
		}
	}
	if _, ok := got["assets/logo.png"]; ok {
		t.Error("a file with no annotations has a finding")
	}
}

func TestFindingsTrimsDotSlash(t *testing.T) {
	t.Parallel()
	got := Findings([]core.Annotation{
		{Path: "./a.go", Level: core.AnnotationWarning},
		{Path: "a.go", Level: core.AnnotationFailure},
	})
	if want := (Finding{Count: 2, Level: core.AnnotationFailure}); got["a.go"] != want || len(got) != 1 {
		t.Errorf("findings = %+v, want a.go %+v only", got, want)
	}
}

// TestFindingsBadge checks the order and the colors of the badge: on a file
// that isn't viewed it goes before the note of a change since, and on a viewed
// one it stays colored and the check follows.
func TestFindingsBadge(t *testing.T) {
	t.Parallel()
	o := opts(config.IconsUnicode, 80)
	rows := Rows(Data{Commits: pull(2), Range: All(2), Files: changed(), Viewed: viewed(true), Findings: findings()}, o)
	line := func(p string) string {
		for _, r := range rows {
			if r.ID == FileID(p) {
				return r.Lines[0]
			}
		}
		t.Fatalf("no row for %s", p)
		return ""
	}
	st := &o.Styles
	failure := st.failure + " " + st.dim.Render("3")
	if got := line("internal/tui/pulls/panes.go"); !strings.HasSuffix(got, failure+" "+st.check) {
		t.Errorf("viewed row = %q, want it to end in the colored badge and the check", got)
	}
	main := line("cmd/gh-tui/main.go")
	badge := st.warning + " " + st.dim.Render("1")
	i, j := strings.Index(main, badge), strings.Index(main, st.rerequested)
	if i < 0 || j < 0 || i > j {
		t.Errorf("changed row = %q, want the badge before the note of the change", main)
	}
	for _, r := range Rows(Data{Commits: pull(2), Range: All(2), Files: changed(), Viewed: viewed(true)}, o) {
		if r.ID == FileID("assets/logo.png") && r.Lines[0] != line("assets/logo.png") {
			t.Errorf("a file without findings changed: %q, want %q", line("assets/logo.png"), r.Lines[0])
		}
	}
}

func TestCommitChecksFolds(t *testing.T) {
	t.Parallel()
	d := Data{Commits: pull(4), Range: All(4)}
	for _, r := range Rows(d, opts(config.IconsNerd, 80)) {
		if r.Fold && r.ID != IDCommits && r.ID != IDFiles {
			t.Errorf("commit row %q is a fold with no way to read its checks", r.ID)
		}
	}
	var depths []int
	d.Children = func(oid string, depth int) []foldtree.Row {
		depths = append(depths, depth)
		return []foldtree.Row{{ID: "child/" + oid, Depth: depth}}
	}
	got := ids(Rows(d, opts(config.IconsNerd, 80)))
	var folds, kids int
	for i, r := range d.Commits.Commits {
		if r.Checks == core.CheckNone {
			continue
		}
		folds++
		at := slices.Index(got, CommitID(r.OID))
		if at < 0 || at+1 >= len(got) || got[at+1] != "child/"+r.OID {
			t.Errorf("commit %d has no children after it in %v", i, got)
		}
		kids++
	}
	if len(depths) != folds || folds == 0 || kids != folds {
		t.Errorf("children asked for %d commits, want %d", len(depths), folds)
	}
	for _, depth := range depths {
		if depth != commitDepth+1 {
			t.Errorf("children at depth %d, want %d", depth, commitDepth+1)
		}
	}
}

func TestFileRows(t *testing.T) {
	t.Parallel()
	rows := Rows(Data{Commits: pull(1), Range: All(1), Files: changed(), Truncated: "cut"}, opts(config.IconsNerd, 80))
	long := "internal/a-very-long-directory-name/with/more/levels/than/fit/in/the/pane/file.go"
	want := []string{
		DirID("assets"), FileID("assets/logo.png"),
		DirID("cmd"), DirID("cmd/gh-tui"), FileID("cmd/gh-tui/main.go"),
		DirID("internal"),
		DirID("internal/a-very-long-directory-name"),
		DirID("internal/a-very-long-directory-name/with"),
		DirID("internal/a-very-long-directory-name/with/more"),
		DirID("internal/a-very-long-directory-name/with/more/levels"),
		DirID("internal/a-very-long-directory-name/with/more/levels/than"),
		DirID("internal/a-very-long-directory-name/with/more/levels/than/fit"),
		DirID("internal/a-very-long-directory-name/with/more/levels/than/fit/in"),
		DirID("internal/a-very-long-directory-name/with/more/levels/than/fit/in/the"),
		DirID("internal/a-very-long-directory-name/with/more/levels/than/fit/in/the/pane"),
		FileID(long),
		DirID("internal/tui"), DirID("internal/tui/pulls"), FileID("internal/tui/pulls/panes.go"),
		FileID("README.md"), idTruncated,
	}
	got := ids(rows)[len(rows)-len(want):]
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for _, r := range rows[len(rows)-len(want) : len(rows)-1] {
		switch p, isFile := PathOf(r.ID); {
		case isFile:
			if r.Kind != KindFile || r.Skip || r.Fold || r.Action || r.Depth != fileDepth+strings.Count(p, "/") {
				t.Errorf("file row %+v", r)
			}
		default:
			dir, ok := DirOf(r.ID)
			if !ok || r.Kind == KindFile || r.Skip || !r.Fold || r.Closed || !r.Arrow || r.Action || r.Depth != fileDepth+strings.Count(dir, "/") {
				t.Errorf("folder row %+v", r)
			}
		}
	}
	if last := rows[len(rows)-1]; !last.Skip {
		t.Errorf("the note %+v must be skipped by the cursor", last)
	}
}

func TestViewedRows(t *testing.T) {
	t.Parallel()
	d := Data{Commits: pull(1), Range: All(1), Files: changed(), Viewed: viewed(true)}
	byID := map[string]foldtree.Row{}
	for _, r := range Rows(d, opts(config.IconsUnicode, 80)) {
		byID[r.ID] = r
	}
	line := func(path string) string {
		r := byID[FileID(path)]
		return ansi.Strip(r.Lines[0]) + "|" + ansi.Strip(r.Right)
	}
	for path, want := range map[string]string{
		"README.md":                   "README.md ✓|+3",
		"cmd/gh-tui/main.go":          "main.go ↻ changed since viewed|−14",
		"assets/logo.png":             "logo.png|",
		"internal/tui/pulls/panes.go": "panes.go ✓|+120 −7",
	} {
		if got := line(path); !strings.HasSuffix(got, want) {
			t.Errorf("%s: row %q, want it to end %q", path, got, want)
		}
	}
	plain := byID[FileID("assets/logo.png")]
	if strings.Contains(plain.Lines[0], "✓") {
		t.Errorf("unviewed row %q has a check", plain.Lines[0])
	}
}

func TestDirOf(t *testing.T) {
	t.Parallel()
	if p, ok := DirOf(DirID("a/b")); !ok || p != "a/b" {
		t.Errorf("DirOf(DirID) = %q, %v", p, ok)
	}
	if p, ok := DirOf(FileID("a/b.go")); ok {
		t.Errorf("DirOf(file) = %q, true", p)
	}
	if p, ok := PathOf(DirID("a/b")); ok {
		t.Errorf("PathOf(dir) = %q, true", p)
	}
}

func TestPathOf(t *testing.T) {
	t.Parallel()
	if p, ok := PathOf(FileID("a/b.go")); !ok || p != "a/b.go" {
		t.Errorf("PathOf(FileID) = %q, %v", p, ok)
	}
	if p, ok := PathOf(CommitID("abc")); ok {
		t.Errorf("PathOf(commit) = %q, true", p)
	}
}

func TestTotals(t *testing.T) {
	t.Parallel()
	if n, a, d := Totals(changed()); n != 5 || a != 124 || d != 22 {
		t.Errorf("Totals = %d, %d, %d; want 5, 124, 22", n, a, d)
	}
	if n, a, d := Totals(nil); n != 0 || a != 0 || d != 0 {
		t.Errorf("Totals(nil) = %d, %d, %d; want zeros", n, a, d)
	}
}

func TestInfoForms(t *testing.T) {
	t.Parallel()
	st := NewStyles(theme(), ui.NewIcons(config.IconsUnicode), config.IconsUnicode)
	for _, tc := range []struct {
		n, add, del int
		want        []string
	}{
		{0, 0, 0, nil},
		{1, 3, 0, []string{"1 file · +3 −0", "1 · +3 −0"}},
		{12, 340, 25, []string{"12 files · +340 −25", "12 · +340 −25"}},
	} {
		forms := Info(tc.n, tc.add, tc.del, st)
		got := make([]string, 0, len(forms))
		for _, f := range forms {
			got = append(got, ansi.Strip(f))
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("Info(%d, %d, %d) = %q, want %q", tc.n, tc.add, tc.del, got, tc.want)
		}
	}
}

func TestRowsShape(t *testing.T) {
	t.Parallel()
	pc := pull(4)
	pc.Capped = true
	pc.LastReview = &core.ViewerReview{Commit: pc.Commits[1].OID}
	rows := Rows(Data{Commits: pc, Range: All(4)}, opts(config.IconsNerd, 80))
	want := []string{
		IDCommits,
		CommitID(pc.Commits[0].OID), CommitID(pc.Commits[1].OID),
		idDivider,
		CommitID(pc.Commits[2].OID), CommitID(pc.Commits[3].OID),
		idCapped, idRule, IDFiles,
	}
	if got := ids(rows); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for _, r := range rows {
		switch r.ID {
		case IDCommits:
			if !r.Fold || !r.Closed || r.Arrow {
				t.Errorf("Commits: %+v, want a closed fold with no arrow", r)
			}
		case IDFiles:
			if !r.Fold || r.Closed || r.Arrow {
				t.Errorf("Files: %+v, want an open fold with no arrow", r)
			}
		case idDivider, idCapped, idRule:
			if !r.Skip {
				t.Errorf("%s must be skipped by the cursor", r.ID)
			}
		default:
			if r.Skip || r.Kind != KindCommit || r.Depth != 1 || r.Fold {
				t.Errorf("commit row %+v", r)
			}
		}
	}
}

func BenchmarkRows(b *testing.B) {
	d := Data{Commits: pull(250), Range: Range{40, 200}}
	o := opts(config.IconsNerd, 160)
	b.ReportAllocs()
	for b.Loop() {
		Rows(d, o)
	}
}
