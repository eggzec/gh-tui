package notifications

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var glyphSets = []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII}

// glyphSection returns a section that draws with the icon set named set and
// knows the states in held.
func glyphSection(tb testing.TB, set string, held map[core.SubjectRef]core.SubjectState) (*Section, ui.Theme) {
	tb.Helper()
	th := ui.NewTheme(defaultPalette(), true)
	s := New(tb.Context(), newFake(), config.Default().Keys,
		WithNow(func() time.Time { return now }),
		WithIcons(ui.NewIcons(set)))
	s.states = held
	s.SetTheme(th)
	s.SetSize(100, 10)
	return s, th
}

func glyphThread(unread bool, typ core.SubjectType) core.Notification {
	return core.Notification{
		ID: "1", Unread: unread, Reason: "mention", UpdatedAt: now.Add(-time.Hour),
		Repo:    core.RepoRef{Owner: "o", Name: "a"},
		Subject: core.Subject{Type: typ, Number: 7, Title: "title"},
	}
}

// Each kind and state draws its mark, in its color for an unread thread and
// dimmed for a read one, in every icon set.
func TestRowDrawsTheMarkOfTheSubject(t *testing.T) {
	t.Parallel()
	open, merged := core.SubjectState{State: core.StateOpen}, core.SubjectState{State: core.StateMerged}
	tests := []struct {
		name  string
		typ   core.SubjectType
		state *core.SubjectState
		want  ui.Mark
	}{
		{"open pull", core.SubjectPullRequest, &open, ui.Mark(ui.PullOpen)},
		{"draft pull", core.SubjectPullRequest, &core.SubjectState{State: core.StateOpen, Draft: true}, ui.Mark(ui.PullDraft)},
		{"merged pull", core.SubjectPullRequest, &merged, ui.Mark(ui.PullMerged)},
		{"closed pull", core.SubjectPullRequest, &core.SubjectState{State: core.StateClosed}, ui.Mark(ui.PullClosed)},
		{"pull before its state", core.SubjectPullRequest, nil, ui.MarkPullUnknown},
		{"open issue", core.SubjectIssue, &open, ui.Mark(ui.IssueOpen)},
		{"completed issue", core.SubjectIssue, &core.SubjectState{State: core.StateClosed}, ui.Mark(ui.IssueClosed)},
		{"issue not planned", core.SubjectIssue, &core.SubjectState{State: core.StateClosed, Reason: core.ReasonNotPlanned}, ui.Mark(ui.IssueNotPlanned)},
		{"issue before its state", core.SubjectIssue, nil, ui.MarkIssueUnknown},
		{"open discussion", core.SubjectDiscussion, &open, ui.MarkDiscussionOpen},
		{"answered discussion", core.SubjectDiscussion, &core.SubjectState{State: core.StateOpen, Answered: true}, ui.MarkDiscussionAnswered},
		{"closed discussion", core.SubjectDiscussion, &core.SubjectState{State: core.StateClosed}, ui.MarkDiscussionClosed},
		{"discussion before its state", core.SubjectDiscussion, nil, ui.MarkDiscussionUnknown},
		{"release", core.SubjectRelease, nil, ui.MarkRelease},
		{"commit", core.SubjectCommit, nil, ui.MarkCommit},
		{"check suite", core.SubjectCheckSuite, nil, ui.MarkCheckSuite},
		{"security alert", core.SubjectType("RepositoryVulnerabilityAlert"), nil, ui.MarkAlert},
	}
	for _, set := range glyphSets {
		for _, tt := range tests {
			t.Run(set+" "+tt.name, func(t *testing.T) {
				t.Parallel()
				held := map[core.SubjectRef]core.SubjectState{}
				n := glyphThread(true, tt.typ)
				if tt.state != nil {
					held[core.SubjectRef{Repo: n.Repo, Number: n.Subject.Number, Type: n.Subject.Type}] = *tt.state
				}
				s, th := glyphSection(t, set, held)
				ic := s.icons
				cell := ic.Mark(tt.want)
				if cell == "" {
					t.Fatalf("mark %d has no glyph in %s", tt.want, set)
				}
				cell += strings.Repeat(" ", kindWidth(ic)-ansi.StringWidth(cell))

				unread := s.render(n, false, 100)
				if want := newPaint(th.Mark(tt.want)).render(cell); !strings.Contains(unread, want) {
					t.Errorf("unread row lacks %q painted for mark %d: %q", cell, tt.want, unread)
				}
				n.Unread = false
				read := s.render(n, false, 100)
				if want := newPaint(th.Subtle).render(cell); !strings.Contains(read, want) {
					t.Errorf("read row lacks %q dimmed: %q", cell, read)
				}
			})
		}
	}
}

// A kind of thread without a mark of its own isn't shown as the separator:
// it has the generic glyph, or in the ASCII set its own name.
func TestRowOfAnUnknownKind(t *testing.T) {
	t.Parallel()
	for _, set := range glyphSets {
		s, _ := glyphSection(t, set, nil)
		row := ansi.Strip(s.render(glyphThread(true, core.SubjectType("RepositoryInvitation")), false, 100))
		fields := strings.Fields(row)
		if len(fields) < 3 {
			t.Fatalf("%s: row %q has too few cells", set, row)
		}
		want := s.icons.Mark(ui.MarkOther)
		if set == config.IconsASCII {
			want = "rep"
		}
		if fields[1] != want {
			t.Errorf("%s: kind cell = %q, want %q in %q", set, fields[1], want, row)
		}
		if sep := strings.TrimSpace(s.icons.Separator); fields[1] == sep {
			t.Errorf("%s: kind cell is the separator %q", set, sep)
		}
	}
}

// A glyph stays in a row too narrow for the ASCII tag.
func TestLayoutKeepsTheGlyph(t *testing.T) {
	t.Parallel()
	if l := newLayout(50, 4, glyphWidth); l.tag != glyphWidth {
		t.Errorf("newLayout(50, 4, glyphWidth) = %+v, want the glyph kept", l)
	}
	if l := newLayout(50, 4, tagWidth); l.tag != 0 {
		t.Errorf("newLayout(50, 4, tagWidth) = %+v, want the tag dropped", l)
	}
}

// The glyph takes one cell, so the layout keeps more room for the title
// than the ASCII tags do.
func TestKindWidth(t *testing.T) {
	t.Parallel()
	for set, want := range map[string]int{config.IconsNerd: 1, config.IconsUnicode: 1, config.IconsASCII: 3} {
		if got := kindWidth(ui.NewIcons(set)); got != want {
			t.Errorf("kindWidth(%s) = %d, want %d", set, got, want)
		}
	}
}
