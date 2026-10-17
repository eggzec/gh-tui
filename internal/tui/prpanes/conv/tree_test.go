package conv

import (
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

var at = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func comment(id string) core.TimelineItem {
	return core.TimelineItem{Kind: core.TimelineComment, ID: id, Actor: core.User{Login: "ann"}, At: at, Body: "body " + id}
}

func review(id string, st core.ReviewState) core.TimelineItem {
	return core.TimelineItem{Kind: core.TimelineReview, ID: id, Actor: core.User{Login: "bob"}, At: at, Review: st}
}

func event(kind core.TimelineKind) core.TimelineItem {
	return core.TimelineItem{Kind: kind, Actor: core.User{Login: "cy"}, At: at}
}

// thread is a thread of the review with n comments.
func thread(id, reviewID string, n int) core.Thread {
	t := core.Thread{ID: id, ReviewID: reviewID, Path: "a/b.go", Line: 7, TotalComments: n}
	for i := range n {
		t.Comments = append(t.Comments, core.ThreadComment{
			ID: id + "-" + string(rune('a'+i)), Author: core.User{Login: "dee"}, At: at, Body: "reply",
		})
	}
	return t
}

// shape writes the tree as its IDs, with each node's children in brackets.
func shape(ns []*node) string {
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		s := n.id
		if len(n.children) > 0 {
			s += "[" + shape(n.children) + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func TestTree(t *testing.T) {
	t.Parallel()
	unknown := event("unknown")
	for _, tc := range []struct {
		name    string
		items   []core.TimelineItem
		threads []core.Thread
		want    string
	}{
		{"none", nil, nil, ""},
		{"a comment alone stays top level", []core.TimelineItem{comment("1")}, nil, "c:1"},
		{
			"three comments make one run",
			[]core.TimelineItem{comment("1"), comment("2"), comment("3")}, nil,
			"run:1[rc:1 rc:2 rc:3]",
		},
		{
			"a review splits runs",
			[]core.TimelineItem{comment("1"), comment("2"), review("R", core.ReviewStateApproved), comment("3"), comment("4"), comment("5")}, nil,
			"run:1[rc:1 rc:2] r:R run:3[rc:3 rc:4 rc:5]",
		},
		{
			"a comment between reviews stays alone",
			[]core.TimelineItem{review("R", core.ReviewStateApproved), comment("1"), review("S", core.ReviewStateCommented)}, nil,
			"r:R c:1 r:S",
		},
		{
			"an event that isn't drawn doesn't split a run",
			[]core.TimelineItem{comment("1"), unknown, comment("2")}, nil,
			"run:1[rc:1 rc:2]",
		},
		{
			"a kind with no row leaves nothing",
			[]core.TimelineItem{unknown}, nil, "",
		},
		{
			"an event is a line and splits a run",
			[]core.TimelineItem{comment("1"), event(core.TimelineClosed), comment("2")}, nil,
			"c:1 e:closed|1767323045000000000|cy|||||||0 c:2",
		},
		{
			"threads attach to their review in order, replies flat",
			[]core.TimelineItem{review("R", core.ReviewStateChangesRequested), review("S", core.ReviewStateCommented)},
			[]core.Thread{thread("T1", "R", 3), thread("T2", "S", 1), thread("T3", "R", 1)},
			"r:R[t:T1[p:T1-b p:T1-c] t:T3] r:S[t:T2]",
		},
		{
			"a review with no thread is a row",
			[]core.TimelineItem{review("R", core.ReviewStateApproved)}, nil, "r:R",
		},
		{
			"a thread of an unknown review is left out",
			[]core.TimelineItem{review("R", core.ReviewStateApproved)},
			[]core.Thread{thread("T1", "X", 1), thread("T2", "", 1)},
			"r:R",
		},
		{
			"drafts make one pending review at the end",
			[]core.TimelineItem{review("R", core.ReviewStateApproved), review("P", core.ReviewStatePending), comment("1")},
			[]core.Thread{pend(thread("T1", "P", 1)), pend(thread("T2", "", 2))},
			"r:R c:1 r:P[t:T1 t:T2[p:T2-b]]",
		},
		{
			"a thread without comments is left out",
			[]core.TimelineItem{review("R", core.ReviewStateApproved)},
			[]core.Thread{{ID: "T1", ReviewID: "R"}},
			"r:R",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shape(tree(Data{Items: tc.items, Threads: tc.threads}, Options{})); got != tc.want {
				t.Errorf("tree = %q, want %q", got, tc.want)
			}
		})
	}
}

func pend(t core.Thread) core.Thread {
	t.Pending = true
	return t
}

func TestNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"name over login", Name(core.User{Login: "ann", Name: "Ann Lee"}), "Ann Lee"},
		{"login", Name(core.User{Login: "ann"}), "ann"},
		{"bot keeps its login", Name(core.User{Login: "dependabot", Bot: true}), "dependabot"},
		{"ghost", Name(core.User{}), "ghost"},
		{"blank name falls back", Name(core.User{Login: "ann", Name: "  "}), "ann"},
		{"team", SubjectName(core.TimelineSubject{Team: "eggzec/core"}), "core team"},
		{"team without org", SubjectName(core.TimelineSubject{Team: "core"}), "core team"},
		{"person", SubjectName(core.TimelineSubject{User: core.User{Login: "ann"}}), "ann"},
	} {
		if strings.Contains(tc.got, "@") {
			t.Errorf("%s: %q holds an @", tc.name, tc.got)
		}
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	for _, u := range []core.User{{Login: termtexttest.Hostile}, {Name: termtexttest.HostileLogin}} {
		if got := Name(u); strings.ContainsAny(got, "\x1b\a\n\u202e") {
			t.Errorf("Name(%+v) = %q, want it cleaned", u, got)
		}
	}
}
