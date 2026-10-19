package involved

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
)

// previewIDs are the rows whose previews the goldens draw.
var previewIDs = []string{
	idAuthor, idReviewers, idAssignees, idCommenters,
	"reviewer:user:eve", "reviewer:user:rita", "reviewer:team:acme/platform",
	"assignee:bob", "commenter:zed", "commenter:xia",
}

func TestPreviewGoldens(t *testing.T) {
	t.Parallel()
	for _, theme := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		for _, sc := range []struct {
			name string
			d    Data
			ids  []string
		}{
			{"all", everything(), previewIDs},
			{"empty", Data{Author: user("ann", "Ann")}, []string{idReviewers, idAssignees, idCommenters, idEmptyReviewers, idEmptyAssignees, idEmptyCommenters}},
		} {
			t.Run(theme.name+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				var b strings.Builder
				for _, w := range []int{40, 80, 120} {
					fmt.Fprintf(&b, "== width %d ==\n", w)
					for _, id := range sc.ids {
						pv, ok := PreviewOf(sc.d, id, Options{Styles: styles(t, theme.dark), Width: w})
						if !ok {
							t.Fatalf("%s: no preview", id)
						}
						fmt.Fprintf(&b, "-- %s | %s | %s\n", id, pv.Title, pv.Info)
						for _, l := range pv.Head {
							fmt.Fprintln(&b, l)
						}
						if pv.Body != "" {
							fmt.Fprintln(&b, pv.Body)
						}
					}
				}
				golden.RequireEqual(t, b.String())
			})
		}
	}
}

func TestPreviewForms(t *testing.T) {
	t.Parallel()
	o := Options{Styles: styles(t, true), Width: 60}
	d := everything()
	tests := []struct {
		name, id string
		title    string
		info     string
		body     []string // each is in the plain text of the body
		head     []string // each is in the plain text of the head
	}{
		{"reviewers", idReviewers, "Reviewers", "", []string{"Rita", "requested", "Eve", "changes requested", "Platform infrastructure team", "and 1 more"}, nil},
		{"assignees", idAssignees, "Assignees", "", []string{"Bob  bob"}, nil},
		{"commenters", idCommenters, "Commenters", "", []string{"Zed  2 comments or reviews", "Xia  1 comment or review"}, nil},
		{"author", idAuthor, "Ann Author", "author · assignee", nil, nil},
		{"reviewer", "reviewer:user:eve", "Eve", "reviewer", nil, []string{"review: changes requested"}},
		{"team", "reviewer:team:acme/platform", "Platform infrastructure", "reviewer", nil, []string{"Team members:", "members hidden"}},
		{"assignee", "assignee:bob", "Bob", "assignee", nil, nil},
		{"commenter", "commenter:zed", "Zed", "", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pv, ok := PreviewOf(d, tt.id, o)
			if !ok {
				t.Fatal("no preview")
			}
			if pv.Title != tt.title || pv.Info != tt.info {
				t.Errorf("title %q info %q, want %q %q", pv.Title, pv.Info, tt.title, tt.info)
			}
			body, head := ansi.Strip(pv.Body), ansi.Strip(strings.Join(pv.Head, "\n"))
			for _, w := range tt.body {
				if !strings.Contains(body, w) {
					t.Errorf("body %q lacks %q", body, w)
				}
			}
			for _, w := range tt.head {
				if !strings.Contains(head, w) {
					t.Errorf("head %q lacks %q", head, w)
				}
			}
			if tt.head == nil && tt.body == nil && len(pv.Head) == 0 {
				t.Error("a person has a head with at least a rule")
			}
		})
	}
}

func TestPreviewEmpty(t *testing.T) {
	t.Parallel()
	o := Options{Styles: styles(t, true)}
	d := Data{Author: user("ann", "Ann")}
	tests := []struct{ id, want string }{
		{idReviewers, "nobody"},
		{idAssignees, "nobody"},
		{idCommenters, "nobody"},
		{idEmptyReviewers, "nobody yet\nenter requests a review"},
		{idEmptyAssignees, "nobody yet\nenter assigns"},
		{idEmptyCommenters, "nobody yet"},
	}
	for _, tt := range tests {
		pv, ok := PreviewOf(d, tt.id, o)
		if !ok || ansi.Strip(pv.Body) != tt.want {
			t.Errorf("%s: %q (ok %v), want %q", tt.id, ansi.Strip(pv.Body), ok, tt.want)
		}
	}
	if _, ok := PreviewOf(d, "reviewer:user:nobody", o); ok {
		t.Error("a row that doesn't exist has a preview")
	}
}

func TestPersonNarrow(t *testing.T) {
	t.Parallel()
	d := everything()
	pv, _ := PreviewOf(d, "commenter:zed", Options{Styles: styles(t, true)})
	if pv.Key != "person:zed" || !strings.Contains(ansi.Strip(pv.Nothing), "no comments or reviews from Zed in this pull request") {
		t.Errorf("key %q nothing %q", pv.Key, ansi.Strip(pv.Nothing))
	}
	team, _ := PreviewOf(d, "reviewer:team:acme/platform", Options{Styles: styles(t, true)})
	if team.Key != "" {
		t.Errorf("a team has no conversation, key %q", team.Key)
	}
}

func TestNarrowFor(t *testing.T) {
	t.Parallel()
	ann, bob := user("ann", "Ann"), user("bob", "Bob")
	d := Data{
		Items: []core.TimelineItem{
			{ID: "c1", Kind: core.TimelineComment, Actor: ann},
			{ID: "c2", Kind: core.TimelineComment, Actor: bob},
			{ID: "r1", Kind: core.TimelineReview, Actor: ann},
			{ID: "r2", Kind: core.TimelineReview, Actor: bob},
			{ID: "r3", Kind: core.TimelineReview, Actor: bob},
			{ID: "e1", Kind: core.TimelineReviewRequestRemoved, Actor: ann},
		},
		Threads: []core.Thread{
			{ID: "t1", ReviewID: "r2", Comments: []core.ThreadComment{{Author: bob}, {Author: ann}}},
			{ID: "t2", ReviewID: "r3", Comments: []core.ThreadComment{{Author: bob}}},
			{ID: "t3", ReviewID: "r1", Comments: []core.ThreadComment{{Author: ann, Pending: true}}},
		},
	}
	n := NarrowFor(d, "ann")
	var items, threads []string
	for _, it := range d.Items {
		if n.Item(it) {
			items = append(items, it.ID)
		}
	}
	for _, th := range d.Threads {
		if n.Thread(th) {
			threads = append(threads, th.ID)
		}
	}
	// ann's comment and review, and bob's review that holds a thread she wrote
	// in; not bob's comment, nor his other review, nor an event of hers.
	if got, want := strings.Join(items, " "), "c1 r1 r2"; got != want {
		t.Errorf("items %q, want %q", got, want)
	}
	if got, want := strings.Join(threads, " "), "t1"; got != want {
		t.Errorf("threads %q, want %q", got, want)
	}
	if got, want := Count(d, "ann"), 3; got != want {
		t.Errorf("count %d, want %d", got, want)
	}
}
