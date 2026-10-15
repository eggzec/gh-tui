package pulls

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
)

// memDrafts keeps drafts in memory and records the keys deleted.
type memDrafts struct {
	mu      sync.Mutex
	drafts  map[string]string
	deleted []string
	loads   int
}

func (d *memDrafts) Load(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loads++
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

// commenting returns the modal of #142 on its Conversation tab, over a thread
// of one comment, with drafts kept in d.
func commenting(t *testing.T, svc *fakeService, d *memDrafts) (*host, *detailModal) {
	t.Helper()
	svc.thread = []core.Comment{{ID: "c1", Author: core.User{Login: "hubot"}, Body: "First.", CreatedAt: clock}}
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	var drafts composer.Drafts
	if d != nil {
		drafts = d
	}
	h := started(t, svc, 116, 40, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)), WithDrafts(drafts, "host/me"))
	h.SetTheme(theme(true))
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no pull request")
	}
	drain(t, h, m.switchTo(conversationTab))
	return h, m
}

// submit types a comment in the open composer and submits it with its key.
func submit(t *testing.T, h *host, m *detailModal) {
	t.Helper()
	m.comp.SetValue("Looks good.")
	drain(t, h, h.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}))
}

func TestCommentOpensOnTheConversationOnly(t *testing.T) {
	t.Parallel()
	h, m := commenting(t, newFakeService(), &memDrafts{})
	press(t, h, "c")
	if m.comp == nil {
		t.Fatal("c opened no composer on the Conversation tab")
	}
	if got := ansi.Strip(m.View()); !strings.Contains(got, "Comment on #142") {
		t.Errorf("the modal shows no composer:\n%s", got)
	}
	press(t, h, "esc")
	if m.comp != nil {
		t.Error("esc left the composer open")
	}
	if h.modal() != m {
		t.Error("esc closed the modal, not the composer")
	}

	drain(t, h, m.switchTo(filesTab))
	press(t, h, "c")
	if m.comp != nil {
		t.Error("c opened a composer on the Files tab")
	}
}

func TestCommentAsksBeforeItPosts(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	d := &memDrafts{}
	h, m := commenting(t, svc, d)
	press(t, h, "c")
	submit(t, h, m)
	if m.ask == nil || m.ask.Question != "Post this comment on #142?" {
		t.Fatalf("submit asked %+v, want the question to post", m.ask)
	}
	if got := svc.changes(); len(got) != 0 {
		t.Fatalf("submit alone sent %v, want nothing before the answer", got)
	}
	press(t, h, "n")
	if m.ask != nil || m.comp == nil {
		t.Fatalf("n left the question %+v and the composer %v, want no question and the composer open", m.ask, m.comp)
	}
	if got := m.comp.Value(); got != "Looks good." {
		t.Errorf("the composer holds %q after no, want the text kept", got)
	}
	if got := svc.changes(); len(got) != 0 {
		t.Errorf("n sent %v, want nothing", got)
	}
}

func TestCommentPostsAndDiscardsTheDraft(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	d := &memDrafts{}
	h, m := commenting(t, svc, d)
	press(t, h, "c")
	submit(t, h, m)
	key := m.draftKey()
	if text, _ := d.get(key); text != "Looks good." {
		t.Fatalf("the draft is %q after submit, want it saved while the comment is sent", text)
	}

	cmd := h.Update(keyMsg("y"))
	if m.comp != nil {
		t.Error("yes left the composer open")
	}
	// The command that reads the comments again runs with the send; read
	// them now, as it would first, to see the comment before the answer.
	drain(t, h, m.thread.Reload())
	if got := ansi.Strip(m.View()); !strings.Contains(got, "sending") {
		t.Errorf("the comment is not shown as sending:\n%s", got)
	}
	drain(t, h, cmd)
	if got := svc.sent(); !slices.Equal(got, []string{"comment Looks good."}) {
		t.Errorf("sent %v, want the comment", got)
	}
	if text, deleted := d.get(key); text != "" || !slices.Equal(deleted, []string{key}) {
		t.Errorf("the draft is %q and %v were deleted, want it deleted", text, deleted)
	}
	got := ansi.Strip(m.View())
	if strings.Contains(got, "sending") || !strings.Contains(got, "Looks good.") {
		t.Errorf("the confirmed comment shows wrong:\n%s", got)
	}
}

func TestCommentDraftSurvivesSaveAndDiscard(t *testing.T) {
	t.Parallel()
	for name, drop := range map[string]func(m *detailModal) error{
		"save":    func(m *detailModal) error { return m.Save() },
		"discard": func(m *detailModal) error { m.Discard(); return nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := &memDrafts{}
			h, m := commenting(t, newFakeService(), d)
			press(t, h, "c")
			m.comp.SetValue("Half written")
			if err := drop(m); err != nil {
				t.Fatal(err)
			}
			if text, _ := d.get(m.draftKey()); text != "Half written" {
				t.Fatalf("the draft is %q, want the text of the open composer", text)
			}

			// A fresh modal on the same pull request starts from it.
			h2, m2 := commenting(t, newFakeService(), d)
			press(t, h2, "c")
			if got := m2.comp.Value(); got != "Half written" {
				t.Errorf("the reopened composer holds %q, want the draft", got)
			}
		})
	}
}

func TestCommentRefusedRollsBackAndKeepsTheDraft(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	d := &memDrafts{}
	h, m := commenting(t, svc, d)
	press(t, h, "c")
	submit(t, h, m)
	svc.sendErr = errors.New("boom")
	var failed bool
	for _, msg := range press(t, h, "y") {
		if done, ok := msg.(ui.DoneMsg); ok && done.Err != nil {
			failed = true
		}
	}
	if !failed {
		t.Error("a refused comment reported no failure to show")
	}
	if got := ansi.Strip(m.thread.View()); strings.Contains(got, "Looks good.") {
		t.Errorf("the refused comment is still shown:\n%s", got)
	}
	if text, deleted := d.get(m.draftKey()); text != "Looks good." || len(deleted) != 0 {
		t.Errorf("the draft is %q and %v were deleted, want the text kept", text, deleted)
	}
	if m.comp == nil || m.comp.Value() != "Looks good." {
		t.Errorf("the composer is %v after the refusal, want it open with the text", m.comp)
	}
}

func TestCommentRefusedWithoutDraftsKeepsTheText(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := commenting(t, svc, nil)
	press(t, h, "c")
	submit(t, h, m)
	svc.sendErr = errors.New("boom")
	press(t, h, "y")
	if m.comp == nil || m.comp.Value() != "Looks good." {
		t.Fatalf("the composer is %v after the refusal, want it open with the text", m.comp)
	}

	// With the composer away, the text waits for the next one.
	press(t, h, "esc")
	press(t, h, "c")
	if got := m.comp.Value(); got != "" {
		t.Fatalf("the composer holds %q, want the text given back once", got)
	}
}

func TestCommentRefusedAwayFromTheConversationWaitsForTheNextComposer(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := commenting(t, svc, nil)
	press(t, h, "c")
	submit(t, h, m)
	svc.sendErr = errors.New("boom")
	cmd := h.Update(keyMsg("y"))
	drain(t, h, m.switchTo(filesTab))
	drain(t, h, cmd)
	if m.comp != nil {
		t.Fatal("a composer opened on the Files tab")
	}
	drain(t, h, m.switchTo(conversationTab))
	press(t, h, "c")
	if m.comp == nil || m.comp.Value() != "Looks good." {
		t.Errorf("the composer is %v, want it to hold the refused text", m.comp)
	}
}

func TestCommentPostedKeepsADraftSavedMeanwhile(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	d := &memDrafts{}
	h, m := commenting(t, svc, d)
	press(t, h, "c")
	submit(t, h, m)
	key := m.draftKey()
	cmd := h.Update(keyMsg("y"))
	if err := d.Save(key, "Another one."); err != nil {
		t.Fatal(err)
	}
	drain(t, h, cmd)
	if text, deleted := d.get(key); text != "Another one." || len(deleted) != 0 {
		t.Errorf("the draft is %q and %v were deleted, want the newer draft kept", text, deleted)
	}
}

func TestCommentSaveLeavesTheDraftToABlankComposerNotYetLoaded(t *testing.T) {
	t.Parallel()
	d := &memDrafts{}
	_, m := commenting(t, newFakeService(), d)
	if err := d.Save(m.draftKey(), "Half written"); err != nil {
		t.Fatal(err)
	}
	// The composer opens, but the draft has not reached it yet.
	m.startComment()
	if m.comp == nil {
		t.Fatal("no composer opened")
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if text, deleted := d.get(m.draftKey()); text != "Half written" || len(deleted) != 0 {
		t.Errorf("the draft is %q and %v were deleted, want it kept", text, deleted)
	}
}
