package conv

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

var (
	alice = core.User{Login: "alice", Name: "Alice Chen"}
	bert  = core.User{Login: "bert", Name: "Bert Moss"}
	carol = core.User{Login: "carol", Name: "Carol Diaz"}
)

// ev is an event of who, mins minutes after the start of the tests' time.
func ev(kind core.TimelineKind, who core.User, mins int) core.TimelineItem {
	return core.TimelineItem{Kind: kind, Actor: who, At: at.Add(time.Duration(mins) * time.Minute)}
}

// labeled is the event of who adding the label name, mins minutes in.
func labeled(who core.User, name string, mins int) core.TimelineItem {
	it := ev(core.TimelineLabeled, who, mins)
	it.Label = core.Label{Name: name, Color: "d73a4a"}
	return it
}

// said returns what the system lines of d say, without the icon, the colors
// and the padding, in the order they are drawn.
func said(d Data, width int) []string { return saidWith(d, opts(config.IconsASCII, width)) }

// saidWith is said with the options o.
func saidWith(d Data, o Options) []string {
	var out []string
	for _, r := range Build(d, o) {
		if !strings.HasPrefix(r.ID, "e:") {
			continue
		}
		_, s, _ := strings.Cut(strings.TrimSpace(ansi.Strip(r.Lines[0])), " ")
		out = append(out, s)
	}
	return out
}

func TestEventTexts(t *testing.T) {
	t.Parallel()
	with := func(kind core.TimelineKind, f func(*core.TimelineItem)) core.TimelineItem {
		it := ev(kind, alice, 0)
		f(&it)
		return it
	}
	person := func(it *core.TimelineItem) { it.Subject.User = carol }
	team := func(it *core.TimelineItem) { it.Subject.Team = "eggzec/core" }
	none := func(*core.TimelineItem) {}
	for _, tc := range []struct {
		name string
		item core.TimelineItem
		want string
	}{
		{"labeled", with(core.TimelineLabeled, func(it *core.TimelineItem) { it.Label = core.Label{Name: "bug", Color: "d73a4a"} }), "Alice Chen added  bug"},
		{"unlabeled", with(core.TimelineUnlabeled, func(it *core.TimelineItem) { it.Label = core.Label{Name: "wip", Color: "d73a4a"} }), "Alice Chen removed  wip"},
		{"assigned", with(core.TimelineAssigned, person), "Alice Chen assigned Carol Diaz"},
		{"unassigned", with(core.TimelineUnassigned, person), "Alice Chen unassigned Carol Diaz"},
		{"requested", with(core.TimelineReviewRequested, person), "Alice Chen requested review from Carol Diaz"},
		{"request removed", with(core.TimelineReviewRequestRemoved, person), "Alice Chen removed the review request for Carol Diaz"},
		{"team requested", with(core.TimelineReviewRequested, team), "Alice Chen requested review from core team"},
		{"dismissed", with(core.TimelineReviewDismissed, none), "Alice Chen dismissed a review"},
		{"ready", with(core.TimelineReadyForReview, none), "Alice Chen marked this ready for review"},
		{"draft", with(core.TimelineConvertedToDraft, none), "Alice Chen converted this to a draft"},
		{"closed", with(core.TimelineClosed, none), "Alice Chen closed this"},
		{"reopened", with(core.TimelineReopened, none), "Alice Chen reopened this"},
		{"merged", with(core.TimelineMerged, func(it *core.TimelineItem) {
			it.Commit.OID, it.Ref = "0123456789abcdef", "main"
		}), "Alice Chen merged 0123456 into main"},
		{"locked", with(core.TimelineLocked, none), "Alice Chen locked the conversation"},
		{"locked as", with(core.TimelineLocked, func(it *core.TimelineItem) { it.Ref = "spam" }), "Alice Chen locked the conversation as spam"},
		{"unlocked", with(core.TimelineUnlocked, none), "Alice Chen unlocked the conversation"},
		{"renamed", with(core.TimelineRenamed, func(it *core.TimelineItem) { it.From, it.To = "Old", "New" }), "Alice Chen changed the title from Old to New"},
		{"base", with(core.TimelineBaseChanged, func(it *core.TimelineItem) { it.From, it.To = "develop", "main" }), "Alice Chen changed the base branch from develop to main"},
		{"head deleted", with(core.TimelineHeadDeleted, func(it *core.TimelineItem) { it.Ref = "feat/x" }), "Alice Chen deleted the branch feat/x"},
		{"referenced", with(core.TimelineCrossReferenced, func(it *core.TimelineItem) {
			it.Source.Number, it.Source.Title = 7, "Fix it"
		}), "Alice Chen referenced this in #7 Fix it"},
		{"milestoned", with(core.TimelineMilestoned, func(it *core.TimelineItem) { it.Ref = "v1" }), "Alice Chen added this to v1"},
		{"demilestoned", with(core.TimelineDemilestoned, func(it *core.TimelineItem) { it.Ref = "v1" }), "Alice Chen removed this from v1"},
		{"deleted actor", func() core.TimelineItem {
			it := ev(core.TimelineClosed, core.User{}, 0)
			return it
		}(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := said(Data{Items: []core.TimelineItem{tc.item}}, 160)
			if len(got) != 1 {
				t.Fatalf("%d lines, want 1", len(got))
			}
			if tc.want != "" && got[0] != tc.want {
				t.Errorf("line = %q, want %q", got[0], tc.want)
			}
			if tc.want == "" && !strings.HasSuffix(got[0], "closed this") {
				t.Errorf("line of a deleted account = %q, want it to end with the event", got[0])
			}
		})
	}
}

func TestEventCentred(t *testing.T) {
	t.Parallel()
	closed := ev(core.TimelineClosed, alice, 0)
	long := ev(core.TimelineRenamed, alice, 0)
	long.From, long.To = strings.Repeat("a", 60), strings.Repeat("b", 60)
	shifted := ev(core.TimelineHeadDeleted, alice, 0)
	shifted.Ref = "ab"

	lead := func(r foldtree.Row) (pad, tw int, right string) {
		s := ansi.Strip(r.Lines[0])
		trimmed := strings.TrimLeft(s, " ")
		return len(s) - len(trimmed), ansi.StringWidth(strings.TrimRight(trimmed, " ")), ansi.Strip(r.Right)
	}
	row := func(it core.TimelineItem, w int) foldtree.Row {
		for _, r := range Build(Data{Items: []core.TimelineItem{it}}, opts(config.IconsASCII, w)) {
			if strings.HasPrefix(r.ID, "e:") {
				return r
			}
		}
		t.Fatal("no system line")
		return foldtree.Row{}
	}

	for _, w := range []int{80, 160} {
		pad, tw, right := lead(row(closed, w))
		if want := (w - 1 - tw) / 2; pad != want || pad == 0 || right == "" {
			t.Errorf("width %d: pad %d, age %q, want pad %d and an age", w, pad, right, want)
		}
	}

	// Near the age the line moves left to keep two cells from it.
	pad, tw, right := lead(row(shifted, 40))
	room := 39 - ansi.StringWidth(right) - 2
	if pad != room-tw || pad >= (39-tw)/2 {
		t.Errorf("width 40: pad %d, text %d, want %d, short of the middle %d", pad, tw, room-tw, (39-tw)/2)
	}

	// What does not fit is cut at the age.
	pad, tw, right = lead(row(long, 40))
	if want := 39 - ansi.StringWidth(right) - 2; pad != 0 || tw != want {
		t.Errorf("width 40: pad %d, text %d, want 0 and %d", pad, tw, want)
	}
}

func TestEventBreaksRun(t *testing.T) {
	t.Parallel()
	d := Data{Items: []core.TimelineItem{comment("1"), labeled(alice, "bug", 1), comment("2")}}
	kinds := func(o Options) string {
		ns := tree(d, o)
		ks := make([]string, 0, len(ns))
		for _, n := range ns {
			ks = append(ks, n.kind)
		}
		return strings.Join(ks, " ")
	}
	if got, want := kinds(Options{}), fmt.Sprint(KindComment, " ", KindEvent, " ", KindComment); got != want {
		t.Errorf("nodes = %q, want %q", got, want)
	}
	var o Options
	o.hide[GroupLabels] = true
	if got := kinds(o); got != KindRun {
		t.Errorf("with labels hidden, nodes = %q, want one run", got)
	}
}

func TestEventIDsStable(t *testing.T) {
	t.Parallel()
	closed := ev(core.TimelineClosed, alice, 0)
	items := []core.TimelineItem{comment("1"), closed, labeled(bert, "bug", 5)}
	eventIDs := func(items []core.TimelineItem) []string {
		var out []string
		for _, n := range tree(Data{Items: items}, Options{}) {
			if n.kind == KindEvent {
				out = append(out, n.id)
			}
		}
		return out
	}
	before := eventIDs(items)
	after := eventIDs(slices.Concat(items, []core.TimelineItem{comment("2")}))
	if len(before) != 2 || strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("IDs = %q, then %q after a comment, want the same two", before, after)
	}
	twice := eventIDs([]core.TimelineItem{closed, closed})
	if len(twice) != 2 || twice[0] == twice[1] || !strings.HasPrefix(twice[1], twice[0]) {
		t.Errorf("IDs of identical events = %q, want two that differ", twice)
	}
}

func TestPreviewHasNoEvents(t *testing.T) {
	t.Parallel()
	d := Data{Items: []core.TimelineItem{comment("1"), ev(core.TimelineClosed, alice, 1), comment("2")}}
	ns := tree(d, Options{preview: true})
	if got := shape(ns); got != "c:1 c:2" {
		t.Errorf("preview = %q, want the comments alone", got)
	}
}

func TestEventsGolden(t *testing.T) {
	t.Parallel()
	d := eventsFixture()
	for _, w := range []int{80, 120, 160} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			golden.RequireEqual(t, text(Build(d, opts(config.IconsNerd, w)), w, false))
		})
	}
}

// eventsFixture is a conversation of comments, a review and system lines.
func eventsFixture() Data {
	ago := func(h int) time.Time { return now.Add(-time.Duration(h) * time.Hour) }
	at := func(it core.TimelineItem, h int) core.TimelineItem {
		it.At = ago(h)
		return it
	}
	unlabeled := ev(core.TimelineUnlabeled, alice, 0)
	unlabeled.Label = core.Label{Name: "wip", Color: "d73a4a"}
	team := ev(core.TimelineReviewRequested, alice, 0)
	team.Subject.Team = "eggzec/core"
	base := ev(core.TimelineBaseChanged, bert, 0)
	base.From, base.To = "develop", "main"
	merged := ev(core.TimelineMerged, carol, 0)
	merged.Commit.OID, merged.Ref = "0123456789abcdef", "main"
	rev := ev(core.TimelineReview, bert, 0)
	rev.ID, rev.Review, rev.Body = "R1", core.ReviewStateApproved, "Looks good."
	first := comment("c1")
	first.Actor = alice
	first.Body = "Opening this for review."
	last := comment("c2")
	last.Actor = carol
	last.Body = "Merging."
	return Data{Items: []core.TimelineItem{
		at(first, 70),
		at(labeled(alice, "bug", 0), 69),
		at(labeled(alice, "api", 0), 69),
		at(unlabeled, 69),
		at(team, 68),
		at(rev, 60),
		at(base, 50),
		at(last, 5),
		at(merged, 4),
	}}
}
