package pulls

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// mergeTexts are what GitHub offers to merge #142 with.
var mergeTexts = core.MergeTexts{
	Squash: core.MergeMessage{Title: "Squash title (#142)", Body: "squash body"},
	Merge:  core.MergeMessage{Title: "Merge pull request #142", Body: "merge body"},
}

// memDrafts keeps drafts in memory and records the keys deleted.
type memDrafts struct {
	mu      sync.Mutex
	drafts  map[string]string
	deleted []string
}

func (d *memDrafts) Load(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.drafts[key], nil
}

func (d *memDrafts) Save(key, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.drafts == nil {
		d.drafts = map[string]string{}
	}
	d.drafts[key] = text
	return nil
}

func (d *memDrafts) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.drafts, key)
	d.deleted = append(d.deleted, key)
	return nil
}

func (d *memDrafts) get(key string) (text string, deleted []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.drafts[key], slices.Clone(d.deleted)
}

// merging opens the merge dialog of #142 over svc, with drafts kept in d.
func merging(t *testing.T, svc *fakeService, d *memDrafts) (*host, *mergeDialog) {
	t.Helper()
	svc.mergeTexts = map[int]core.MergeTexts{142: mergeTexts}
	h := started(t, svc, 120, 24, WithDrafts(d, "host/me"))
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
	press(t, h, "M")
	dlg := dialogOf(h)
	if dlg == nil || !dlg.loaded {
		t.Fatalf("M opened no loaded merge dialog:\n%s", question(h))
	}
	return h, dlg
}

func TestMergeDialogStepsTheMethodOnTheMethodRowOnly(t *testing.T) {
	t.Parallel()
	_, dlg := merging(t, newFakeService(), &memDrafts{})
	if got := dlg.current(); got != core.MergeSquash {
		t.Fatalf("the dialog starts on %q, want squash", got)
	}
	dlg.Update(keyMsg("right"))
	if got := dlg.current(); got != core.MergeRebase {
		t.Errorf("right on the method row picks %q, want rebase", got)
	}
	// On the title row the key moves the cursor of the field.
	dlg.Update(keyMsg("left"))
	dlg.move(1)
	if dlg.focus != focusTitle {
		t.Fatalf("the cursor is on row %d, want the title", dlg.focus)
	}
	dlg.Update(keyMsg("right"))
	if got := dlg.current(); got != core.MergeSquash {
		t.Errorf("right on the title row changed the method to %q, want it unchanged", got)
	}
}

func TestMergeDialogPrefillKeepsAnEditedTitle(t *testing.T) {
	t.Parallel()
	_, dlg := merging(t, newFakeService(), &memDrafts{})
	if got := dlg.title.Value(); got != mergeTexts.Squash.Title {
		t.Fatalf("the title starts as %q, want GitHub's", got)
	}
	dlg.title.SetValue("My title")
	// Squash, rebase, merge commit.
	dlg.Update(keyMsg("right"))
	dlg.Update(keyMsg("right"))
	if got := dlg.current(); got != core.MergeCommit {
		t.Fatalf("method = %q, want a merge commit", got)
	}
	if got := dlg.title.Value(); got != "My title" {
		t.Errorf("the title is %q after a method change, want the edited one", got)
	}
	if got := dlg.body.Value(); got != mergeTexts.Merge.Body {
		t.Errorf("the body is %q, want the one GitHub offers for the method", got)
	}
}

func TestMergeDialogSendsWhatItShows(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, dlg := merging(t, svc, &memDrafts{})
	dlg.title.SetValue("Add things")
	press(t, h, "y")
	got := svc.changes()
	if len(got) != 1 || !strings.HasPrefix(got[0], "merge squash ") || !strings.Contains(got[0], " title=Add things body=squash body") {
		t.Errorf("changes = %q, want a squash with the edited title and GitHub's body", got)
	}
}

func TestMergeDialogRebaseSendsNoMessage(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, dlg := merging(t, svc, &memDrafts{})
	dlg.title.SetValue("Add things")
	press(t, h, "right")
	press(t, h, "y")
	if got, want := svc.changes(), []string{"merge rebase 142"}; !slices.Equal(got, want) {
		t.Errorf("changes = %q, want %q, since a rebase keeps each commit's message", got, want)
	}
}

func TestMergeDialogDraftIsSavedRestoredAndDeleted(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	drafts := &memDrafts{}
	key := mergeDraftKey("host/me", repo, 142)
	h, dlg := merging(t, svc, drafts)

	// Closing an edited message keeps it.
	dlg.title.SetValue("Draft title")
	press(t, h, "n")
	if got, _ := drafts.get(key); got != "squash\nDraft title\n"+mergeTexts.Squash.Body {
		t.Fatalf("draft = %q, want the method and the edited message", got)
	}

	// The next dialog opens on it.
	press(t, h, "M")
	dlg = dialogOf(h)
	if dlg == nil || dlg.title.Value() != "Draft title" {
		t.Fatalf("the dialog reopened without the draft:\n%s", question(h))
	}

	// Merging with it sends it and drops it.
	press(t, h, "y")
	if got := svc.changes(); len(got) != 1 || !strings.Contains(got[0], " title=Draft title") {
		t.Errorf("changes = %q, want the draft's title sent", got)
	}
	if got, deleted := drafts.get(key); got != "" || !slices.Contains(deleted, key) {
		t.Errorf("draft = %q, deleted %v, want it deleted once merged", got, deleted)
	}
}

func TestMergeDialogUneditedMessageLeavesNoDraft(t *testing.T) {
	t.Parallel()
	drafts := &memDrafts{}
	key := mergeDraftKey("host/me", repo, 142)
	h, _ := merging(t, newFakeService(), drafts)
	press(t, h, "n")
	if got, deleted := drafts.get(key); got != "" || !slices.Contains(deleted, key) {
		t.Errorf("draft = %q, deleted %v, want none kept for an unedited message", got, deleted)
	}
}

func TestMergeDialogKeepsTheMessageOfAMergeThatFailed(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.sendErr = errors.New("head moved")
	drafts := &memDrafts{}
	key := mergeDraftKey("host/me", repo, 142)
	h, dlg := merging(t, svc, drafts)
	dlg.title.SetValue("Long title")
	press(t, h, "y")
	if got, _ := drafts.get(key); got != "squash\nLong title\n"+mergeTexts.Squash.Body {
		t.Fatalf("draft = %q after a merge that failed, want the message kept", got)
	}
	press(t, h, "M")
	if dlg = dialogOf(h); dlg == nil || dlg.title.Value() != "Long title" {
		t.Fatalf("the dialog reopened without the message:\n%s", question(h))
	}
}

func TestMergeDialogReopensOnTheMethodOfTheDraft(t *testing.T) {
	t.Parallel()
	drafts := &memDrafts{}
	h, dlg := merging(t, newFakeService(), drafts)
	press(t, h, "right")
	press(t, h, "right")
	if got := dlg.current(); got != core.MergeCommit {
		t.Fatalf("method = %q, want a merge commit", got)
	}
	dlg.title.SetValue("Draft title")
	press(t, h, "n")
	press(t, h, "M")
	dlg = dialogOf(h)
	if dlg == nil || dlg.current() != core.MergeCommit || dlg.title.Value() != "Draft title" {
		t.Fatalf("the dialog reopened on %v, want the merge commit and the draft:\n%s", dlg, question(h))
	}
}

func TestMergeDialogAutoMergeTitle(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanAutoMerge: true, RepoAutoMerge: true}, core.ReviewApproved, core.ChecksPending)
	caps := writeCaps
	caps.AutoMerge = true
	h, _ := asking(t, svc, caps, "M")
	if got, want := dialogOf(h).Title(), "Merge #142 when ready"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestMergeDialogSaysTheBase(t *testing.T) {
	t.Parallel()
	t.Run("merge", func(t *testing.T) {
		t.Parallel()
		_, dlg := merging(t, newFakeService(), &memDrafts{})
		if got := dlg.View(); !strings.Contains(got, "Into main.") {
			t.Errorf("the dialog doesn't name the base:\n%s", got)
		}
	})
	t.Run("auto-merge", func(t *testing.T) {
		t.Parallel()
		svc := newFakeService()
		svc.mergeTexts = map[int]core.MergeTexts{142: mergeTexts}
		stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanAutoMerge: true, RepoAutoMerge: true}, core.ReviewApproved, core.ChecksPending)
		caps := writeCaps
		caps.AutoMerge = true
		h, _ := asking(t, svc, caps, "M")
		dlg := dialogOf(h)
		drain(t, h, dlg.Init())
		if got := dlg.View(); !strings.Contains(got, "Into main; merges when checks pass.") {
			t.Errorf("the dialog doesn't name the base and what it waits for:\n%s", got)
		}
	})
}

func TestMergeNote(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ base, when, want string }{
		{"main", "", "Into main."},
		{"main", "checks pass", "Into main; merges when checks pass."},
		{"main", "it can be merged", "Into main; merges when it can be merged."},
		{"", "checks pass", "Merges when checks pass."},
		{"", "", ""},
	} {
		if got := mergeNote(core.PullRequest{BaseRef: tt.base}, tt.when); got != tt.want {
			t.Errorf("mergeNote(%q, %q) = %q, want %q", tt.base, tt.when, got, tt.want)
		}
	}
}

func TestMergeDialogSaysWhyGitHubsMessageIsMissing(t *testing.T) {
	t.Parallel()
	_, dlg := merging(t, newFakeService(), &memDrafts{})
	dlg.fill(mergeTextMsg{dialog: dlg, err: core.ErrForbidden})
	if !strings.Contains(dlg.note, "Couldn't read GitHub's message: ") || !strings.HasSuffix(dlg.note, ". An empty title is GitHub's default.") || strings.Contains(dlg.note, "\n") {
		t.Errorf("note = %q, want a line with the cause", dlg.note)
	}
}
