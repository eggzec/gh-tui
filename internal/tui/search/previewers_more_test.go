package search

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// heldSection returns a wide page on kind with the query tea
// submitted. The reads of the previews that finish are kept from the page and
// returned, for the test to give it when it chooses.
func heldSection(tb testing.TB, f *fakeService, kind core.SearchKind, opts ...Option) (s *Section, held []tea.Msg) {
	tb.Helper()
	p := config.Default().Prefetch
	p.Search.Preview = 0
	s = newSection(tb, f, 140, 40, append(opts, WithPrefetch(p))...)
	press(tb, s, "esc")
	press(tb, s, string(rune('1'+slices.Index(kinds, kind))))
	typeOnly(tb, s, "tea")
	_, held = drive(tb, s, s.Update(keyPress("enter")), func(m tea.Msg) bool {
		_, ok := m.(previewReadMsg)
		return ok
	})
	return s, held
}

func teaUser() *fakeService {
	f := newFake()
	f.users = []core.SearchUser{{Login: "teaman", Kind: core.OwnerUser}, {Login: "teapot", Kind: core.OwnerOrg}}
	return f
}

func teaCommit() *fakeService {
	f := newFake()
	f.commits = []core.SearchCommit{
		{Repo: core.RepoRef{Owner: "acme", Name: "tea"}, SHA: "0123456789abcdef0123456789abcdef01234567", Subject: "tea brewing fix", Author: "ana", Date: now},
		{Repo: core.RepoRef{Owner: "acme", Name: "tea"}, SHA: "fedcba9876543210fedcba9876543210fedcba98", Subject: "tea kettle", Author: "bo", Date: now},
	}
	return f
}

func sampleProfile() fakeUserPreviews {
	return fakeUserPreviews{p: core.UserProfile{Name: "Tea Man", Followers: 42, Bio: "Brews daily", CreatedAt: time.Date(2015, 6, 7, 0, 0, 0, 0, time.UTC)}}
}

func sampleCommit() fakeCommitPreviews {
	return fakeCommitPreviews{p: core.CommitPreview{
		Detail: core.CommitDetail{
			Commit: core.Commit{Body: "Brew longer.", Committer: core.Signature{Name: "Bo", Date: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)}},
			Stats:  core.CommitStats{Additions: 3, Deletions: 1},
			Files:  []core.CommitFile{{Path: "brew.go", Additions: 3, Deletions: 1}, {Path: "kettle/steep.go", Additions: 0, Deletions: 0}},
		},
		Patch: "diff --git a/brew.go b/brew.go\n@@ -1 +1 @@\n-cold\n+hot\n",
	}}
}

// byLoginProfiles gives each login a bio of its own.
type byLoginProfiles struct{}

func (byLoginProfiles) Profile(_ context.Context, login string) (core.UserProfile, error) {
	return core.UserProfile{Login: login, Bio: "Bio of " + login}, nil
}

// bySHACommits gives each commit a body of its own.
type bySHACommits struct{}

func (bySHACommits) Commit(_ context.Context, _ core.RepoRef, sha string) (core.CommitPreview, error) {
	return core.CommitPreview{Detail: core.CommitDetail{Commit: core.Commit{Body: "Body of " + sha[:4]}}}, nil
}

func TestViewUserAndCommitPreviews(t *testing.T) {
	t.Parallel()
	for _, w := range []struct{ cols, rows int }{{80, 22}, {140, 38}} {
		for _, k := range []struct {
			name string
			kind core.SearchKind
			f    *fakeService
			opt  Option
		}{
			{"user", core.SearchUsers, teaUser(), WithPreviewReader(NewPreviewReader(PreviewServices{Users: sampleProfile()}))},
			{"commit", core.SearchCommits, teaCommit(), WithPreviewReader(NewPreviewReader(PreviewServices{Commits: sampleCommit()}))},
		} {
			t.Run(k.name+" "+strconv.Itoa(w.cols)+" columns", func(t *testing.T) {
				t.Parallel()
				s := kindSectionAt(t, k.f, k.kind, w.cols, w.rows, k.opt)
				if w.cols < 100 {
					// A narrow page shows one pane at a time and opens on the results.
					press(t, s, "tab")
				}
				golden.RequireEqual(t, s.View())
			})
		}
	}
}

// kindSectionAt is a page on kind at width by height with the query tea
// submitted and its previews read.
func kindSectionAt(tb testing.TB, f *fakeService, kind core.SearchKind, width, height int, opts ...Option) *Section {
	tb.Helper()
	p := config.Default().Prefetch
	p.Search.Preview = 0
	s := newSection(tb, f, width, height, append(opts, WithPrefetch(p))...)
	press(tb, s, "esc")
	press(tb, s, string(rune('1'+slices.Index(kinds, kind))))
	typeText(tb, s, "tea")
	return s
}

func TestPreviewsShowLoadingWhileRead(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		kind core.SearchKind
		f    *fakeService
		opt  Option
		want string
	}{
		{"user", core.SearchUsers, teaUser(), WithPreviewReader(NewPreviewReader(PreviewServices{Users: sampleProfile()})), "Type: User"},
		{"commit", core.SearchCommits, teaCommit(), WithPreviewReader(NewPreviewReader(PreviewServices{Commits: sampleCommit()})), "tea brewing fix"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, held := heldSection(t, tt.f, tt.kind, tt.opt)
			if len(held) == 0 {
				t.Fatal("no read started")
			}
			got := s.View()
			if !strings.Contains(got, "Loading more") || !strings.Contains(got, tt.want) {
				t.Errorf("while the read is out the preview shows:\n%s", got)
			}
		})
	}
}

func TestStaleUserReadIsDropped(t *testing.T) {
	t.Parallel()
	opt := WithPreviewReader(NewPreviewReader(PreviewServices{Users: sampleProfile()}))
	t.Run("query changed", func(t *testing.T) {
		t.Parallel()
		s, held := heldSection(t, teaUser(), core.SearchUsers, opt)
		press(t, s, "i")
		press(t, s, "backspace")
		// The new query's own reads stay out, so only the old read can fill the preview.
		drive(t, s, s.Update(keyPress("enter")), func(m tea.Msg) bool { _, ok := m.(previewReadMsg); return ok })
		run(t, s, s.Update(held[0]))
		if got := s.View(); strings.Contains(got, "Brews daily") {
			t.Errorf("a read for the old query shows:\n%s", got)
		}
	})
	t.Run("cursor moved", func(t *testing.T) {
		t.Parallel()
		s, held := heldSection(t, teaUser(), core.SearchUsers, WithPreviewReader(NewPreviewReader(PreviewServices{Users: byLoginProfiles{}})))
		press(t, s, "down")
		s.Update(held[0])
		if got := s.View(); strings.Contains(got, "Bio of teaman") {
			t.Errorf("a read for the hit before shows under the next:\n%s", got)
		}
	})
}

func TestStaleCommitReadIsDropped(t *testing.T) {
	t.Parallel()
	opt := WithPreviewReader(NewPreviewReader(PreviewServices{Commits: sampleCommit()}))
	t.Run("query changed", func(t *testing.T) {
		t.Parallel()
		s, held := heldSection(t, teaCommit(), core.SearchCommits, opt)
		press(t, s, "i")
		press(t, s, "backspace")
		// The new query's own reads stay out, so only the old read can fill the preview.
		drive(t, s, s.Update(keyPress("enter")), func(m tea.Msg) bool { _, ok := m.(previewReadMsg); return ok })
		run(t, s, s.Update(held[0]))
		if got := s.View(); strings.Contains(got, "+hot") {
			t.Errorf("a read for the old query shows:\n%s", got)
		}
	})
	t.Run("cursor moved", func(t *testing.T) {
		t.Parallel()
		s, held := heldSection(t, teaCommit(), core.SearchCommits, WithPreviewReader(NewPreviewReader(PreviewServices{Commits: bySHACommits{}})))
		press(t, s, "down")
		s.Update(held[0])
		if got := s.View(); strings.Contains(got, "Body of 0123") {
			t.Errorf("a read for the hit before shows under the next:\n%s", got)
		}
	})
}

func TestUserExtraOfAnEmptyProfile(t *testing.T) {
	t.Parallel()
	got := userExtra(core.UserProfile{})
	if got != "- Followers: 0\n" {
		t.Errorf("userExtra of nothing = %q, want only the followers", got)
	}
}

func TestCommitExtraOfAnEmptyCommit(t *testing.T) {
	t.Parallel()
	got := commitExtra(core.CommitPreview{}, "…")
	if got != "- Changes: +0 -0 in 0 files\n" {
		t.Errorf("commitExtra of nothing = %q, want only the changes line", got)
	}
}

func TestCommitExtraMarksATruncatedFileList(t *testing.T) {
	t.Parallel()
	files := []core.CommitFile{{Path: "a.go", Additions: 1}}
	more := commitExtra(core.CommitPreview{Detail: core.CommitDetail{Files: files, FilesNext: "c2"}}, "…")
	if !strings.Contains(more, "in 1+ files") || !strings.Contains(more, "- …\n") {
		t.Errorf("a list with more pages shows:\n%s", more)
	}
	cut := commitExtra(core.CommitPreview{Detail: core.CommitDetail{Files: files, FilesTruncated: true}}, "…")
	if !strings.Contains(cut, "in 1+ files") || !strings.Contains(cut, "- …\n") {
		t.Errorf("a cut list shows:\n%s", cut)
	}
	whole := commitExtra(core.CommitPreview{Detail: core.CommitDetail{Files: files}}, "…")
	if strings.Contains(whole, "…") {
		t.Errorf("a whole list shows an ellipsis:\n%s", whole)
	}
}

func TestCommitExtraKeepsABacktickPathInItsSpan(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"a`b.go":   "- `` a`b.go `` +1 -0\n",
		"`lead.go": "- `` `lead.go `` +1 -0\n",
		"x``y.go":  "- ``` x``y.go ``` +1 -0\n",
		"plain.go": "- `plain.go` +1 -0\n",
	} {
		got := commitExtra(core.CommitPreview{Detail: core.CommitDetail{Files: []core.CommitFile{{Path: path, Additions: 1}}}}, "…")
		if !strings.Contains(got, want) {
			t.Errorf("path %q shows %q, want %q", path, got, want)
		}
	}
}

func TestCommitPreviewFitsANarrowPage(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("deep/", 30) + "file.go"
	svc := fakeCommitPreviews{p: core.CommitPreview{
		Detail: core.CommitDetail{Files: []core.CommitFile{{Path: long, Additions: 1}}},
		Patch:  "@@ -1 +1 @@\n+" + strings.Repeat("word ", 80) + "\n",
	}}
	s := kindSectionAt(t, teaCommit(), core.SearchCommits, 60, 22, WithPreviewReader(NewPreviewReader(PreviewServices{Commits: svc})))
	press(t, s, "tab")
	view := s.View()
	if !strings.Contains(view, "Changes: +") {
		t.Errorf("the preview is not on screen:\n%s", view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("line %d is %d columns wide, want at most 60", i, w)
		}
	}
}

func commitWithPatchNearTheCap() fakeCommitPreviews {
	var b strings.Builder
	b.WriteString("@@ -1 +1 @@\n")
	for b.Len() < core.MaxCommitPatch-100 {
		b.WriteString("+a line of the patch that changes something\n")
	}
	return fakeCommitPreviews{p: core.CommitPreview{
		Detail: core.CommitDetail{Files: []core.CommitFile{{Path: "big.go", Additions: 5000}}},
		Patch:  b.String(),
	}}
}

// BenchmarkCommitPreview renders the preview of a commit whose patch is near
// the cap, at a new width each time.
func BenchmarkCommitPreview(b *testing.B) {
	s := kindSectionAt(b, teaCommit(), core.SearchCommits, 140, 38, WithPreviewReader(NewPreviewReader(PreviewServices{Commits: commitWithPatchNearTheCap()})))
	b.ReportAllocs()
	w := 140
	for b.Loop() {
		w = 279 - w
		s.SetSize(w, 38)
		_ = s.View()
	}
}
