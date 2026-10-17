package merge

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

func TestBlockerOrder(t *testing.T) {
	t.Parallel()
	rows, targets := Rows(everything(), opts(t, 100))
	got := ids(rows)
	if len(got) < 7 || got[0] != "checks" {
		t.Fatalf("ids = %q, want the checks node and its rows first", got)
	}
	want := []string{"review-required", "changes:carol", "changes:frank", "conflicts", "behind", "draft"}
	if tail := got[len(got)-len(want):]; !slices.Equal(tail, want) {
		t.Errorf("last ids = %q, want %q", tail, want)
	}
	for _, r := range rows[len(rows)-len(want):] {
		if !r.Action {
			t.Errorf("row %q has no action", r.ID)
		}
		if r.Fold || r.Skip {
			t.Errorf("row %q folds or is skipped: %+v", r.ID, r)
		}
	}
	if !rows[0].Fold || rows[0].Action {
		t.Errorf("checks node = %+v, want a fold that toggles", rows[0])
	}
	wantText := []string{
		"Review required: core team",
		"Changes requested: Carol Diaz",
		"Changes requested: Frank",
		"Conflicts with main",
		"Branch is 3 commits behind main",
		"Draft",
	}
	for i, l := range first(rows[len(rows)-len(want):]) {
		if !strings.HasSuffix(l, wantText[i]) {
			t.Errorf("row %q reads %q, want it to end with %q", want[i], l, wantText[i])
		}
	}
	if _, ok := targets["conflicts"]; ok {
		t.Error("conflicts has a target, want none")
	}
}

func TestCount(t *testing.T) {
	t.Parallel()
	few := detail()
	few.Draft = true
	few.Reviewers.Verdicts = []core.Verdict{
		{Author: user("a", ""), State: core.ReviewStateChangesRequested},
		{Author: user("b", ""), State: core.ReviewStateChangesRequested},
		{Author: user("c", ""), State: core.ReviewStateApproved},
	}
	tests := []struct {
		name string
		d    Data
		want int
	}{
		{"nothing", Data{Detail: detail(), Behind: -1}, 0},
		{"everything", everything(), 7},
		{"two reviewers, checks and a draft", Data{Detail: few, Checks: failing(), ChecksLoaded: true}, 4},
		{"checks not read", Data{Detail: detail(), Checks: failing()}, 0},
		{"passing checks", Data{Detail: detail(), Checks: commit([]core.CommitCheck{job(1, "a", core.ConclusionSuccess)}), ChecksLoaded: true}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Count(tt.d); got != tt.want {
				t.Errorf("Count = %d, want %d", got, tt.want)
			}
			if rows, _ := Rows(tt.d, opts(t, 100)); (len(rows) == 0) != (tt.want == 0) {
				t.Errorf("%d rows for %d blockers", len(rows), tt.want)
			}
		})
	}
}

func TestRowsOfNothing(t *testing.T) {
	t.Parallel()
	d := Data{Detail: detail(), Checks: commit([]core.CommitCheck{job(1, "a", core.ConclusionSuccess)}), ChecksLoaded: true}
	rows, targets := Rows(d, opts(t, 100))
	if len(rows) != 0 || len(targets) != 0 || Count(d) != 0 {
		t.Errorf("rows = %v, targets = %v, count = %d, want none", ids(rows), targets, Count(d))
	}
}

func TestWho(t *testing.T) {
	t.Parallel()
	owner := core.ReviewRequest{Team: "acme/core", TeamName: "core team", CodeOwner: true}
	tests := []struct {
		name  string
		asked []core.ReviewRequest
		want  string
	}{
		{"code owners first", []core.ReviewRequest{{User: user("dave", "")}, owner, {User: user("eve", ""), CodeOwner: true}}, "core team, eve"},
		{"all asked when no owner", []core.ReviewRequest{{User: user("dave", "")}, {Team: "acme/infra"}, {Team: "acme/web", TeamName: "Web"}}, "dave, infra, Web"},
		{"unknown skipped", []core.ReviewRequest{{Unknown: true, CodeOwner: true}, {User: user("dave", "")}}, "dave"},
		{"only unknown", []core.ReviewRequest{{Unknown: true}}, ""},
		{"nobody asked", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := detail()
			d.ReviewDecision = core.ReviewRequired
			d.Reviewers.Requested = tt.asked
			rows, _ := Rows(Data{Detail: d}, opts(t, 100))
			want := "Review required"
			if tt.want != "" {
				want += ": " + tt.want
			}
			if got := first(rows)[0]; !strings.HasSuffix(got, want) {
				t.Errorf("row = %q, want it to end with %q", got, want)
			}
		})
	}
}

func TestBehind(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{
		-1: "Branch is behind main",
		0:  "Branch is behind main",
		1:  "Branch is 1 commit behind main",
		2:  "Branch is 2 commits behind main",
	} {
		d := detail()
		d.Merge.Status = core.MergeBehind
		rows, _ := Rows(Data{Detail: d, Behind: n}, opts(t, 100))
		if got := first(rows)[0]; !strings.HasSuffix(got, want) {
			t.Errorf("behind %d: row = %q, want it to end with %q", n, got, want)
		}
	}
	d := detail()
	if rows, _ := Rows(Data{Detail: d, Behind: 4}, opts(t, 100)); len(rows) != 0 {
		t.Errorf("rows of a branch that is not behind = %v", ids(rows))
	}
}

func TestChangesNameFallsBackToLogin(t *testing.T) {
	t.Parallel()
	d := detail()
	d.Reviewers.Verdicts = []core.Verdict{{Author: user("erin", ""), State: core.ReviewStateChangesRequested}}
	rows, _ := Rows(Data{Detail: d}, opts(t, 100))
	if got := first(rows)[0]; !strings.HasSuffix(got, "Changes requested: erin") {
		t.Errorf("row = %q", got)
	}
}

func TestBranch(t *testing.T) {
	t.Parallel()
	d := everything()
	if got, want := plain(Branch(d, opts(t, 100))), "feat/calc → main"; got != want {
		t.Errorf("Branch = %q, want %q", got, want)
	}
	if got := plain(Branch(d, opts(t, 8))); got != "feat/ca…" {
		t.Errorf("Branch at 8 = %q, want it cut with an ellipsis", got)
	}
}

func TestActions(t *testing.T) {
	t.Parallel()
	d := everything()
	tests := []struct {
		name  string
		width int
		keys  Hints
		d     Data
		want  string
	}{
		{"long", 80, Hints{"M", "U"}, d, "Merge (squash) M · Update branch U"},
		{"exactly 54", 54, Hints{"M", "U"}, d, "Merge (squash) M · Update branch U"},
		{"short below 54", 53, Hints{"M", "U"}, d, "Merge M · Update U"},
		{"no limit", 0, Hints{"M", "U"}, d, "Merge (squash) M · Update branch U"},
		{"no method", 80, Hints{"M", "U"}, Data{Detail: d.Detail}, "Merge M · Update branch U"},
		{"merge unbound", 80, Hints{"", "U"}, d, "Update branch U"},
		{"update unbound", 80, Hints{"M", ""}, d, "Merge (squash) M"},
		{"update unbound, short", 40, Hints{"M", ""}, d, "Merge M"},
		{"both unbound", 80, Hints{}, d, ""},
		{"cut", 12, Hints{"M", "U"}, d, "Merge M · U…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := opts(t, tt.width)
			o.Keys = tt.keys
			if got := plain(Actions(tt.d, o)); got != tt.want {
				t.Errorf("Actions = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRule(t *testing.T) {
	t.Parallel()
	for width, want := range map[int]string{0: "─────", 40: "─────", 3: "───"} {
		if got := plain(Rule(opts(t, width))); got != want {
			t.Errorf("Rule at %d = %q, want %q", width, got, want)
		}
	}
}

func TestIDsSurviveARefresh(t *testing.T) {
	t.Parallel()
	a, _ := Rows(everything(), opts(t, 100))
	d := everything()
	d.Behind = 9
	d.Checks.Runs = append(d.Checks.Runs, job(9, "extra", core.ConclusionFailure))
	b, _ := Rows(d, opts(t, 100))
	for _, id := range []string{"checks", "review-required", "changes:carol", "conflicts", "behind", "draft"} {
		if !slices.Contains(ids(a), id) || !slices.Contains(ids(b), id) {
			t.Errorf("id %q is not in both refreshes: %q, %q", id, ids(a), ids(b))
		}
	}
	for _, rows := range [][]foldtree.Row{a, b} {
		seen := map[string]bool{}
		for _, r := range rows {
			if seen[r.ID] {
				t.Errorf("id %q twice", r.ID)
			}
			seen[r.ID] = true
		}
	}
}

// keys binds what the tests press.
var keys = foldtree.NewKeyMap(keymap.Func(func(action string) []string {
	switch action {
	case "down":
		return []string{"j"}
	case "global.select":
		return []string{"enter"}
	}
	return nil
}))

func newModel(tb testing.TB, d Data) Model {
	tb.Helper()
	m := New(foldtree.WithSize(100, 30), foldtree.WithKeyMap(keys), foldtree.WithFocused(true))
	rows, targets := Rows(d, opts(tb, 100))
	m.SetRows(rows, targets)
	return m
}

// enter presses enter on the row under the cursor, and returns the message
// that comes of it.
func enter(m Model) (after Model, msg tea.Msg) {
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return m, nil
	}
	m, cmd = m.Update(cmd())
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func TestEnter(t *testing.T) {
	t.Parallel()
	d := everything()
	rows, targets := Rows(d, opts(t, 100))
	want := map[string]tea.Msg{
		"review-required": RequestReviewMsg{},
		"changes:carol":   FocusReviewMsg{Login: "carol"},
		"changes:frank":   FocusReviewMsg{Login: "frank"},
		"conflicts":       nil,
		"behind":          UpdateBranchMsg{},
		"draft":           MarkReadyMsg{},
	}
	var jobs int
	for i, r := range rows {
		m := newModel(t, d)
		for range i {
			m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		}
		if id, _ := m.Selected(); id != r.ID {
			t.Fatalf("row %d: cursor on %q, want %q", i, id, r.ID)
		}
		m, got := enter(m)
		switch w, known := want[r.ID]; {
		case known:
			if got != w {
				t.Errorf("enter on %q = %v, want %v", r.ID, got, w)
			}
		case r.Fold:
			if got != nil || !m.Folded(r.ID) {
				t.Errorf("enter on fold %q = %v, folded = %v, want a toggle", r.ID, got, m.Folded(r.ID))
			}
		default:
			if got != targets[r.ID] {
				t.Errorf("enter on %q = %v, want %v", r.ID, got, targets[r.ID])
			}
			if _, ok := got.(checktree.OpenJobMsg); ok {
				jobs++
			}
		}
	}
	if jobs == 0 {
		t.Error("no row of a job gave an OpenJobMsg")
	}
}

func TestEnterOfAnotherTreeIsIgnored(t *testing.T) {
	t.Parallel()
	m := newModel(t, everything())
	if _, cmd := m.Update(foldtree.EnterMsg{Tree: m.ID() + 1000, ID: "draft"}); cmd != nil {
		t.Error("enter of another tree gave a command")
	}
}

func TestFoldsAndCursorSurviveARefresh(t *testing.T) {
	t.Parallel()
	d := everything()
	m := newModel(t, d)
	m, _ = enter(m)
	if !m.Folded("checks") {
		t.Fatal("enter on the checks node did not fold it")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	sel, _ := m.Selected()

	d.Behind = 7
	d.Checks.Runs = append(d.Checks.Runs, job(9, "extra", core.ConclusionFailure))
	rows, targets := Rows(d, opts(t, 100))
	m.SetRows(rows, targets)
	if !m.Folded("checks") {
		t.Error("the fold of the checks opened on a refresh")
	}
	if got, _ := m.Selected(); got != sel {
		t.Errorf("cursor on %q after a refresh, want %q", got, sel)
	}
}

func TestHostileInputStaysInsideTheWidth(t *testing.T) {
	t.Parallel()
	d := everything()
	d.Detail.HeadRef = "\x1b[2Jfeat/\x1b]0;pwned\x07" + strings.Repeat("x", 100)
	d.Detail.BaseRef = "main\x1b[31m"
	d.Method = "squash\x1b[2J"
	d.Detail.Reviewers.Requested = nil
	for i := range 200 {
		l := strings.Repeat("l", 100) + string(rune('a'+i%26))
		d.Detail.Reviewers.Requested = append(d.Detail.Reviewers.Requested,
			core.ReviewRequest{User: user(l, "")}, core.ReviewRequest{Team: "o/" + l, CodeOwner: i%2 == 0})
		d.Detail.Reviewers.Verdicts = append(d.Detail.Reviewers.Verdicts,
			core.Verdict{Author: user(l, "N\x1b[2Jame "+l), State: core.ReviewStateChangesRequested})
	}
	for _, width := range []int{20, 40, 54, 80} {
		o := opts(t, width)
		rows, _ := Rows(d, o)
		tree := foldtree.New(foldtree.WithSize(width, 50), foldtree.WithRows(rows))
		lines := append([]string{Branch(d, o), Actions(d, o), Rule(o)}, strings.Split(tree.View(), "\n")...)
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > width {
				t.Fatalf("width %d: line of %d cells: %q", width, w, plain(l))
			}
			// The raw line: the pane's styles only emit SGR, so any other
			// escape, and any bell, came from the input.
			if i := foreignControl(l); i >= 0 {
				t.Fatalf("width %d: line with a control sequence at %d: %q", width, i, l)
			}
		}
	}
}

// foreignControl returns the index of the first control character of s that
// is not part of a color sequence (ESC [ digits and semicolons m) or of a
// hyperlink of the checks (ESC ] 8 ; ... ESC \), or -1.
func foreignControl(s string) int {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 0x1b && strings.HasPrefix(s[i:], "\x1b]8;"):
			end := strings.Index(s[i:], "\x1b\\")
			if end < 0 {
				return i
			}
			i += end + 1
		case c == 0x1b:
			j := i + 1
			if j >= len(s) || s[j] != '[' {
				return i
			}
			j++
			for j < len(s) && (s[j] == ';' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			if j >= len(s) || s[j] != 'm' {
				return i
			}
			i = j
		case c < 0x20 && c != '\n', c == 0x7f:
			return i
		}
	}
	return -1
}

func TestDeletedReviewersGetOwnRows(t *testing.T) {
	t.Parallel()
	d := detail()
	d.Reviewers.Verdicts = []core.Verdict{
		{Author: user("", ""), State: core.ReviewStateChangesRequested},
		{Author: user("carol", ""), State: core.ReviewStateChangesRequested},
		{Author: user("", ""), State: core.ReviewStateChangesRequested},
	}
	rows, _ := Rows(Data{Detail: d, Behind: -1}, opts(t, 100))
	if got := ids(rows); !slices.Equal(got, []string{"changes:0", "changes:carol", "changes:2"}) {
		t.Fatalf("ids = %q, want one distinct id for each verdict", got)
	}
	for _, i := range []int{0, 2} {
		if l := first(rows)[i]; !strings.HasSuffix(l, "Changes requested: ghost") {
			t.Errorf("row %d reads %q, want it to end with the ghost", i, l)
		}
	}
}

func TestReadyStyle(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{true, false} {
		st := styles(t, dark)
		pass := plain(st.Checks.Glyphs.Pass)
		if got := plain(st.Ready); got != pass {
			t.Errorf("dark=%v: Ready strips to %q, want the pass glyph %q", dark, got, pass)
		}
		if st.Ready == pass {
			t.Errorf("dark=%v: Ready is not styled", dark)
		}
	}
}
