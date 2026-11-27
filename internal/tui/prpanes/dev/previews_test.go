package dev

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestHeaderPreviewPerGroup(t *testing.T) {
	bad := issue(self, 9, "unreadable", core.StateOpen)
	bad.Problem = "gone"
	links := core.PullLinks{
		Closes: []core.LinkedIssue{{Issue: issue(self, 1, "closed one", core.StateOpen)}},
		Linked: []core.Reference{pull(2, "linked one", core.StateOpen, false)},
		// The mentions also name what the item closes and what closes it, and one
		// that could not be read: none of them is a row under Mentions.
		Mentions: []core.Reference{
			issue(self, 3, "mention one", core.StateOpen),
			issue(self, 1, "closed one", core.StateOpen),
			pull(2, "linked one", core.StateOpen, false),
			bad,
		},
		Mentioned: []core.Reference{issue(self, 4, "referenced one", core.StateOpen), bad},
	}
	rows := Build(links, Options{Repo: self, Styles: previewOpts("unicode").Styles})
	for group, want := range map[string]string{
		GroupCloses: "closed one", GroupLinked: "linked one",
		GroupMentions: "mention one", GroupReferencedBy: "referenced one",
	} {
		got := ansi.Strip(HeaderPreview(rows.Groups[group], previewOpts("unicode")))
		if !strings.Contains(got, want) || strings.Count(got, "\n") != 0 {
			t.Errorf("%s preview = %q, want only %q", group, got, want)
		}
	}
}

type fakeReader struct {
	cached bool
	err    error
	reads  int
	// pages are the comment pages by cursor, cached when warmPages is set.
	pages        map[string]core.Page[core.Comment]
	warmPages    bool
	commentErr   error
	commentReads []string
	// body is the description of the issues read.
	body string
}

func (f *fakeReader) CachedIssue(core.Target) (core.IssuePreview, bool) {
	return core.IssuePreview{Reference: issue(self, 1, "cached", core.StateOpen), Body: f.body}, f.cached
}

func (f *fakeReader) Issue(context.Context, core.Target) (core.IssuePreview, error) {
	f.reads++
	return core.IssuePreview{Reference: issue(self, 1, "read", core.StateOpen), Body: f.body}, f.err
}

func (f *fakeReader) CachedComments(_ core.Target, cursor string) (core.Page[core.Comment], bool) {
	p, ok := f.pages[cursor]
	return p, ok && f.warmPages
}

func (f *fakeReader) Comments(ctx context.Context, _ core.Target, cursor string) (core.Page[core.Comment], error) {
	f.commentReads = append(f.commentReads, cursor)
	if err := ctx.Err(); err != nil {
		return core.Page[core.Comment]{}, err
	}
	return f.pages[cursor], f.commentErr
}

func (f *fakeReader) CachedPull(core.Target) (core.PullPreview, bool) {
	return core.PullPreview{}, false
}

func (f *fakeReader) Pull(context.Context, core.Target) (core.PullPreview, error) {
	return core.PullPreview{}, nil
}

func TestPreviewsLoad(t *testing.T) {
	it := Item{Kind: Issue, Ref: issue(self, 1, "x", core.StateOpen)}
	o := previewOpts("unicode")

	f := &fakeReader{}
	p := NewPreviews(f)
	cmd := p.Load("a", it)
	if cmd == nil {
		t.Fatal("no read for an uncached preview")
	}
	if p.Load("a", it) != nil {
		t.Error("a read already out was asked for again")
	}
	if got := ansi.Strip(p.View(Row{Key: "a", Item: it}, o)); !strings.Contains(got, "loading") {
		t.Errorf("view before the read = %q", got)
	}
	if p.Handles(cmd()) != true || p.Handles(tea.KeyPressMsg{}) {
		t.Error("Handles is wrong")
	}
	p.Update(cmd())
	if got := ansi.Strip(p.View(Row{Key: "a", Item: it}, o)); !strings.Contains(got, "read") {
		t.Errorf("view after the read = %q", got)
	}
	if p.Load("a", it) != nil {
		t.Error("a held preview was read again")
	}

	c := NewPreviews(&fakeReader{cached: true})
	if c.Load("a", it) != nil || c.Version() != 1 {
		t.Error("a cached preview needed a read")
	}

	bad := NewPreviews(&fakeReader{err: errors.New("boom")})
	bad.Update(bad.Load("a", it)())
	if bad.Load("a", it) != nil {
		t.Error("a failed read was repeated before Retry")
	}
	bad.Retry()
	if bad.Load("a", it) == nil {
		t.Error("Retry did not allow another read")
	}

	skip := it
	skip.Ref.Problem = "gone"
	if NewPreviews(f).Load("a", skip) != nil {
		t.Error("an item with a problem was read")
	}
}
