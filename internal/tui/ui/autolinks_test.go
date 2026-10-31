package ui

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/autolinks"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// fakeLinks answers from known and records what was asked.
type fakeLinks struct {
	known    map[string]autolinks.Result
	cached   int
	verified [][]autolinks.Candidate
}

func (f *fakeLinks) Cached(_ string, cands []autolinks.Candidate) []autolinks.Result {
	f.cached++
	out := make([]autolinks.Result, len(cands))
	for i, c := range cands {
		out[i] = f.known[c.Ref]
	}
	return out
}

func (f *fakeLinks) Verify(_ context.Context, _ string, cands []autolinks.Candidate) []autolinks.Result {
	f.verified = append(f.verified, cands)
	return nil
}

func TestAutolinksFuncReadsTheCacheOnly(t *testing.T) {
	t.Parallel()
	f := &fakeLinks{known: map[string]autolinks.Result{
		"1": {Known: true, Exists: true, URL: "https://github.com/o/r/issues/1"},
		"2": {Known: true},
	}}
	fn := NewAutolinks(f, "github.com").Func("o/r")
	if url, ok := fn(markdown.RefIssue, "1"); !ok || url != "https://github.com/o/r/issues/1" {
		t.Errorf("confirmed ref = %q, %v", url, ok)
	}
	if _, ok := fn(markdown.RefIssue, "2"); ok {
		t.Error("a ref that doesn't exist is linked")
	}
	if _, ok := fn(markdown.RefUser, "a"); ok {
		t.Error("an unanswered ref is linked")
	}
	if len(f.verified) != 0 {
		t.Errorf("Func called Verify: %v", f.verified)
	}
}

func TestAutolinksNilLinksNothing(t *testing.T) {
	t.Parallel()
	var a *Autolinks
	if a.Func("o/r") != nil || a.Verify(context.Background(), "o/r", "1") != nil {
		t.Error("a nil Autolinks links or asks")
	}
	if NewAutolinks(nil, "github.com").Func("o/r") != nil {
		t.Error("no service still links")
	}
}

// Serial: it switches the process-wide links setting.
func TestSeenNotesAsksOnceAndNotWithLinksOff(t *testing.T) {
	f := &fakeLinks{}
	s := NewAutolinks(f, "github.com").Seen("o/r")
	fn := s.Func()
	fn(markdown.RefIssue, "#1")
	fn(markdown.RefUser, "@a")
	fn(markdown.RefIssue, "#1")
	if len(f.verified) != 0 {
		t.Fatalf("rendering called Verify: %v", f.verified)
	}
	cmd := s.Ask(context.Background())
	if cmd == nil {
		t.Fatal("no command for the references seen")
	}
	cmd()
	if len(f.verified) != 1 || len(f.verified[0]) != 2 {
		t.Fatalf("asked %v, want one call of two candidates", f.verified)
	}
	fn(markdown.RefIssue, "#1")
	if s.Ask(context.Background()) != nil {
		t.Error("a reference was asked about twice")
	}

	termtext.SetLinks(false)
	defer termtext.SetLinks(true)
	fn(markdown.RefIssue, "#9")
	if s.Ask(context.Background()) != nil {
		t.Error("links off still asks")
	}
	var none *Seen
	if none.Func() != nil || none.Ask(context.Background()) != nil {
		t.Error("a nil Seen links or asks")
	}
}

// Serial: it switches the process-wide links setting.
func TestAutolinksVerifyAsksOncePerBodyUntilSynced(t *testing.T) {
	f := &fakeLinks{}
	a := NewAutolinks(f, "github.com")
	body := "#1 @a abc1234 and `#2` in code"
	cmd := a.Verify(context.Background(), "o/r", body)
	if cmd == nil {
		t.Fatal("no command for a body with references")
	}
	cmd()
	if len(f.verified) != 1 || len(f.verified[0]) != 3 {
		t.Fatalf("asked %v, want one call of three candidates", f.verified)
	}
	if f.verified[0][1].Kind != core.VerifiedUser {
		t.Errorf("second candidate = %v, want a user", f.verified[0][1])
	}
	if a.Verify(context.Background(), "o/r", body) != nil {
		t.Error("the same body was asked again")
	}
	if a.Synced(SyncMsg{Key: "other"}) {
		t.Error("an unrelated key counts as synced")
	}
	if !a.Synced(SyncMsg{Key: autolinks.SyncKey("github.com")}) {
		t.Fatal("the autolinks key isn't synced")
	}
	if a.Verify(context.Background(), "o/r", body) == nil {
		t.Error("the body isn't asked again after the sync")
	}

	termtext.SetLinks(false)
	defer termtext.SetLinks(true)
	if a.Verify(context.Background(), "o/r", "9") != nil {
		t.Error("links off still asks")
	}
}
