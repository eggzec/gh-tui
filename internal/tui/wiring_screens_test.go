package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	ownersvc "github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/owner"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/releases"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// brokenFiles is the files service of the keys app, but for a blob that
// fails to read when blobErr is set.
type brokenFiles struct {
	keyFiles
	blobErr error
}

func (b brokenFiles) CachedBlob(q filesvc.BlobQuery) (core.Blob, bool) {
	if b.blobErr != nil {
		return core.Blob{}, false
	}
	return b.keyFiles.CachedBlob(q)
}

func (b brokenFiles) Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error) {
	if b.blobErr != nil {
		return core.Blob{}, b.blobErr
	}
	return b.keyFiles.Blob(ctx, q)
}

// brokenIssues is the issues service of the keys app, but for an issue
// that fails to read when getErr is set.
type brokenIssues struct {
	keyIssues
	getErr error
}

func (b brokenIssues) CachedGet(r core.RepoRef, n int) (core.Issue, bool) {
	if b.getErr != nil {
		return core.Issue{}, false
	}
	return b.keyIssues.CachedGet(r, n)
}

func (b brokenIssues) CurrentGet(r core.RepoRef, n int) bool {
	return b.getErr == nil && b.keyIssues.CurrentGet(r, n)
}

func (b brokenIssues) Get(ctx context.Context, r core.RepoRef, n int) (core.Issue, error) {
	if b.getErr != nil {
		return core.Issue{}, b.getErr
	}
	return b.keyIssues.Get(ctx, r, n)
}

// brokenOwners is the owner service of the keys app, but for a README that
// is the given markdown, or fails when readmeErr is set.
type brokenOwners struct {
	keyOwners
	markdown  *string
	readmeErr error
}

func (b brokenOwners) FreshReadme(ownersvc.ReadmeQuery) bool {
	return b.readmeErr == nil && b.markdown == nil
}

func (b brokenOwners) Readme(ctx context.Context, q ownersvc.ReadmeQuery) (ownersvc.Readme, error) {
	switch {
	case b.readmeErr != nil:
		return ownersvc.Readme{}, b.readmeErr
	case b.markdown != nil:
		if *b.markdown == "" {
			return ownersvc.Readme{}, nil
		}
		return ownersvc.Readme{Markdown: *b.markdown, Source: core.RepoRef{Owner: q.Login, Name: ".github"}}, nil
	}
	return b.keyOwners.Readme(ctx, q)
}

// wiredApp is newKeysApp on testRepo over services that can fail.
func wiredApp(t *testing.T, fs brokenFiles, is brokenIssues, os brokenOwners) *Model {
	t.Helper()
	ctx, cfg := t.Context(), config.Default()
	acc := newFakeAccess(uitest.Classic("repo", "workflow", "notifications", "read:org", "gist"))
	v := ui.NewVoice(cfg.Keys, "")
	v.Token = ui.NewToken(acc, cfg.Keys)
	ic := ui.NewIcons(cfg.UI.Icons)
	layout := Layout{
		Files: files.New(ctx, fs, cfg.Keys, files.WithVoice(v), files.WithIcons(ic)),
		Pulls: pulls.New(ctx, keyPulls{}, cfg.Keys, pulls.WithVoice(v), pulls.WithIcons(ic),
			pulls.WithChecks(keyActions{}), pulls.WithCommitChecks(keyActions{}),
			pulls.WithReviewDrafts(keyReviewDrafts())),
		Issues:        issues.New(ctx, is, cfg.Keys, issues.WithVoice(v), issues.WithIcons(ic), issues.WithReferences(keyRefs{})),
		Notifications: notifications.New(ctx, keyInbox{}, cfg.Keys, notifications.WithVoice(v), notifications.WithIcons(ic)),
		Search:        searchpage.New(ctx, keySearch{}, cfg.Keys, searchpage.WithVoice(v), searchpage.WithIcons(ic)),
		Dashboard: dashboard.New(ctx, keyDash{}, cfg.Keys, dashboard.WithVoice(v), dashboard.WithIcons(ic),
			dashboard.WithInbox(keyInbox{}), dashboard.WithHere(testRepo, nil)),
		Owner: owner.New(ctx, os, cfg.Keys, owner.WithVoice(v), owner.WithIcons(ic)),
	}
	m := New(ctx, cfg, layout,
		WithVoice(v), WithAccess(acc),
		WithHistory(history.Opener(keyHistory{}, cfg.Keys, history.WithVoice(v))),
		WithCommit(history.CommitOpener(keyHistory{}, cfg.Keys, history.WithVoice(v))),
		WithRelease(releases.Opener(keyReleases{}, cfg.Keys, releases.WithVoice(v))),
		WithActions(actions.Opener(keyActions{}, cfg.Keys, actions.WithVoice(v))),
		WithStarrer(&fakeStar{known: true}),
		WithRepo(testRepo),
	)
	rankFakes(m)
	m.toast.SetDuration(0)
	m.accessChanges = nil
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	driveKeys(t, m, m.Init())
	return m
}

// wiredKeys presses the actions of the config on m, as keysAfter does.
func wiredKeys(t *testing.T, m *Model, steps ...string) {
	t.Helper()
	c := keyContext{name: "wiring", repo: true}
	for _, s := range steps {
		for _, name := range c.press(t, m.cfg.Keys, s) {
			msg, _ := keyPress(name)
			driveKeys(t, m, m.key(msg))
		}
	}
}

func screenText(m *Model) string { return ansi.Strip(m.View().Content) }

// The finder shows no preview of the file under its cursor until ctrl+o
// asks for one, and the preview follows the key again to hide.
func TestFinderPreviewKey(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.find_file")
		if got := screenText(m); !strings.Contains(got, "Find file") || strings.Contains(got, "Paragraph 1.") {
			t.Fatalf("the finder before ctrl+o:\n%s", got)
		}
		tap(t, m, "ctrl+o")
		if got := screenText(m); !strings.Contains(got, "Paragraph 1.") {
			t.Errorf("ctrl+o showed no preview of the file under the cursor:\n%s", got)
		}
		tap(t, m, "ctrl+o")
		if got := screenText(m); strings.Contains(got, "Paragraph 1.") {
			t.Errorf("a second ctrl+o left the preview on:\n%s", got)
		}
		tap(t, m, "esc")
		if m.modals.Top() != nil {
			t.Errorf("esc left %v open", topType(m))
		}
	})
}

// A file that can't be read shows why in the finder's preview and in the
// preview of the tree, and esc leaves each.
func TestFilePreviewReadFails(t *testing.T) {
	t.Parallel()
	want := "Something went wrong"
	synctest.Test(t, func(t *testing.T) {
		m := wiredApp(t, brokenFiles{blobErr: errors.New("boom")}, brokenIssues{}, brokenOwners{})
		wiredKeys(t, m, "global.find_file")
		tap(t, m, "ctrl+o")
		if got := screenText(m); !strings.Contains(got, want) {
			t.Errorf("the finder's preview of a file that fails to read:\n%s", got)
		}
		tap(t, m, "esc")
		wiredKeys(t, m, "files.down", "global.select")
		if got := screenText(m); !strings.Contains(got, want) {
			t.Errorf("the preview of a file that fails to read:\n%s", got)
		}
		tap(t, m, "esc")
		if m.modals.Top() != nil {
			t.Errorf("esc left %v open", topType(m))
		}
	})
}

// Typing a name that no file has leaves the finder empty, with the count.
func TestFinderNoMatch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.find_file")
		for _, k := range []string{"z", "z", "z"} {
			tap(t, m, k)
		}
		got := screenText(m)
		if !strings.Contains(got, "No files match") || !strings.Contains(got, "2 files · 0 matches") {
			t.Errorf("the finder after a query that nothing matches:\n%s", got)
		}
	})
}

// The issue opens over the list from its row, and shows it with the
// keys of the modal; esc closes it and leaves the list.
func TestIssueModalFromTheList(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_3", "global.select")
		got := screenText(m)
		for _, want := range []string{"#1", "Keys collide", "0 comments", "c comment", "X close issue"} {
			if !strings.Contains(got, want) {
				t.Errorf("the issue modal lacks %q:\n%s", want, got)
			}
		}
		wiredKeys(t, m, "issue_modal.close")
		if got := screenText(m); !strings.Contains(got, "Close issue #1?") {
			t.Errorf("the close key asked nothing:\n%s", got)
		}
		tap(t, m, "esc")
		if got := screenText(m); strings.Contains(got, "Close issue #1?") || !strings.Contains(got, "Keys collide") {
			t.Errorf("esc over the question should leave the issue open:\n%s", got)
		}
		tap(t, m, "esc")
		if m.modals.Top() != nil {
			t.Fatalf("esc left %v open", topType(m))
		}
		if got := screenText(m); !strings.Contains(got, "Keys collide") {
			t.Errorf("the list is gone after the modal closed:\n%s", got)
		}
	})
}

// An issue that can't be read says so in a toast, and the modal can still
// be closed.
func TestIssueModalLoadFails(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := wiredApp(t, brokenFiles{}, brokenIssues{getErr: errors.New("boom")}, brokenOwners{})
		wiredKeys(t, m, "global.pane_3", "global.select")
		if got := toasted(m); !strings.Contains(got, "#1") {
			t.Errorf("toasts after the issue failed to load: %s, want one naming #1", got)
		}
		for range 2 {
			if m.modals.Top() != nil {
				tap(t, m, "esc")
			}
		}
		if m.modals.Top() != nil {
			t.Errorf("esc left %v open", topType(m))
		}
	})
}

// The owner page shows the README of an organization once its pane is
// focused, scrolls it, and says so when it has none or can't be read.
func TestOwnerReadmeThroughGoto(t *testing.T) {
	t.Parallel()
	open := func(t *testing.T, os brokenOwners) *Model {
		t.Helper()
		m := wiredApp(t, brokenFiles{}, brokenIssues{}, os)
		runCommand(t, m, "goto @github")
		if m.screen != ownerScreen {
			t.Fatalf("goto @github showed screen %v, want the owner page", m.screen)
		}
		tap(t, m, "3")
		return m
	}
	t.Run("shown and scrolled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := open(t, brokenOwners{})
			before := screenText(m)
			if !strings.Contains(before, "README") || !strings.Contains(before, "Paragraph 1.") {
				t.Fatalf("the README pane:\n%s", before)
			}
			tap(t, m, "ctrl+d")
			if screenText(m) == before {
				t.Error("ctrl+d didn't scroll the README")
			}
		})
	})
	t.Run("failed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := open(t, brokenOwners{readmeErr: errors.New("boom")})
			if got := screenText(m); !strings.Contains(got, "Something went wrong · r to retry") {
				t.Errorf("the README pane after a failed read:\n%s", got)
			}
		})
	})
	t.Run("none", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			empty := ""
			m := open(t, brokenOwners{markdown: &empty})
			if got := screenText(m); !strings.Contains(got, "has no profile README") {
				t.Errorf("the README pane of an organization with none:\n%s", got)
			}
		})
	})
}
