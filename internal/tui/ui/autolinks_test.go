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

func TestAutolinksSynced(t *testing.T) {
	t.Parallel()
	const host = "github.com"
	key := autolinks.SyncKey(host)
	tests := []struct {
		name    string
		msg     SyncMsg
		change  bool // whether the answer of #1 moves before the sync
		want    bool
		cleared bool
	}{
		{name: "unchanged answers", msg: SyncMsg{Key: key}},
		{name: "changed answer", msg: SyncMsg{Key: key}, change: true, want: true, cleared: true},
		{name: "another host", msg: SyncMsg{Key: autolinks.SyncKey("other")}, change: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeLinks{known: map[string]autolinks.Result{"1": {Known: true}}}
			a := NewAutolinks(f, host)
			fn := a.Func("o/r")
			fn(markdown.RefIssue, "1")
			a.asked[7] = true
			if tt.change {
				f.known["1"] = autolinks.Result{Known: true, Exists: true, URL: "u"}
			}
			var drew uint64
			if got := a.Synced(tt.msg, &drew); got != tt.want {
				t.Errorf("Synced = %v, want %v", got, tt.want)
			}
			if cleared := !a.asked[7]; cleared != tt.cleared {
				t.Errorf("asked cleared = %v, want %v", cleared, tt.cleared)
			}
			if tt.want && a.Synced(tt.msg, &drew) {
				t.Error("the same sync twice changed the links twice")
			}
		})
	}
	var nilLinks *Autolinks
	if nilLinks.Synced(SyncMsg{Key: key}, new(uint64)) {
		t.Error("a nil Autolinks synced")
	}
}

func TestAutolinksSyncedReachesEveryView(t *testing.T) {
	t.Parallel()
	f := &fakeLinks{known: map[string]autolinks.Result{"1": {Known: true}}}
	a := NewAutolinks(f, "github.com")
	a.Func("o/r")(markdown.RefIssue, "1")
	f.known["1"] = autolinks.Result{Known: true, Exists: true, URL: "u"}
	msg := SyncMsg{Key: autolinks.SyncKey("github.com")}
	var one, two uint64
	if !a.Synced(msg, &one) || !a.Synced(msg, &two) {
		t.Error("a view missed the sync that another one saw")
	}
	if a.Synced(msg, &one) || a.Synced(msg, &two) {
		t.Error("a view drew again with nothing new")
	}
}

func TestAutolinksSyncedSeesAnAnswerLearnedBetweenDraws(t *testing.T) {
	t.Parallel()
	f := &fakeLinks{known: map[string]autolinks.Result{"1": {Known: true}}}
	a := NewAutolinks(f, "github.com")
	fn := a.Func("o/r")
	fn(markdown.RefIssue, "1")
	f.known["1"] = autolinks.Result{Known: true, Exists: true, URL: "u"}
	fn(markdown.RefIssue, "1")
	if !a.Synced(SyncMsg{Key: autolinks.SyncKey("github.com")}, new(uint64)) {
		t.Error("a draw between the answer and the sync hid the change")
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
	var drew uint64
	if a.Synced(SyncMsg{Key: "other"}, &drew) {
		t.Error("an unrelated key counts as synced")
	}
	a.Func("o/r")(markdown.RefIssue, "1")
	f.known = map[string]autolinks.Result{"1": {Known: true, Exists: true, URL: "u"}}
	if !a.Synced(SyncMsg{Key: autolinks.SyncKey("github.com")}, &drew) {
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
