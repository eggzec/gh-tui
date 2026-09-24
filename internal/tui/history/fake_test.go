package history

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
)

var repo = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}

// testNow is the clock of the tests.
var testNow = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

var errBoom = errors.New("boom")

// fake is a Service over fixed branches and histories. Details it read are
// cached, as the service caches them. calls lists what it was asked, in
// order, as "branches", "commits ref@cursor", "commit sha", "files
// sha@cursor" and "compare head".
type fake struct {
	mu       sync.Mutex
	branches []core.Branch
	// histories are the commits of each branch, newest first, which pages
	// of commitPage commits list; details are by SHA.
	histories map[string][]core.Commit
	details   map[string]core.CommitDetail
	// morefiles are the later pages of files of a commit, by SHA.
	moreFiles map[string][][]core.CommitFile
	cached    map[string]bool
	compares  map[string]core.Compare
	// stale serves the first page of commits Stale once, as a page an
	// earlier session kept.
	stale bool
	errs  map[string]error
	calls []string
	// hold, if set, holds each read of a detail until it is closed or the
	// read is cancelled.
	hold      chan struct{}
	cancelled []string
}

func newFake() *fake {
	f := &fake{
		histories: map[string][]core.Commit{}, details: map[string]core.CommitDetail{},
		moreFiles: map[string][][]core.CommitFile{}, cached: map[string]bool{},
		compares: map[string]core.Compare{}, errs: map[string]error{},
	}
	f.branches = []core.Branch{{Name: "fix/tabs"}, {Name: "main"}, {Name: "v2-exp"}}
	main := history("main", 120)
	f.histories["main"] = main
	f.histories[""] = main
	f.histories["v2-exp"] = history("v2-exp", 5)
	f.histories["fix/tabs"] = history("fix/tabs", 3)
	for _, h := range f.histories {
		for i := range h {
			f.details[h[i].SHA] = detail(h[i])
		}
	}
	f.compares["fix/tabs"] = core.Compare{Status: core.CompareDiverged, AheadBy: 2, BehindBy: 5}
	return f
}

// sha returns the full SHA of commit n of branch.
func sha(branch string, n int) string {
	sum := sha1.Sum(fmt.Appendf(nil, "%s %d", branch, n))
	return hex.EncodeToString(sum[:])
}

// history returns n commits of branch, newest first, a day apart, with a
// merge at the third.
func history(branch string, n int) []core.Commit {
	out := make([]core.Commit, n)
	for i := range n {
		c := core.Commit{
			SHA:     sha(branch, i),
			Subject: fmt.Sprintf("%s: change %d", branch, i),
			Author: core.Signature{Name: "Kevin", Email: "kevin@example.com", Login: "kevin9327",
				Date: testNow.Add(-time.Duration(i+1) * 24 * time.Hour)},
			URL: "https://github.com/charmbracelet/bubbletea/commit/" + sha(branch, i),
		}
		c.Committer = c.Author
		c.Message = c.Subject
		if i+1 < n {
			c.Parents = []string{sha(branch, i+1)}
		}
		out[i] = c
	}
	if n > 4 {
		out[2].Parents = append(out[2].Parents, sha(branch, 4))
		out[2].Subject = "Merge pull request #1790"
	}
	// The newest commit has all a header can show: a body, trailers, a
	// signature, and a committer other than its author.
	top := &out[0]
	top.Body = "Sequence ran its commands at once, so their messages\ncould arrive out of order. Now each waits for the last."
	top.Trailers = []core.Trailer{
		{Key: "Co-authored-by", Value: "Ayman Bagabas <ayman@example.com>"},
		{Key: "Signed-off-by", Value: "Kevin <kevin@example.com>"},
	}
	top.Verification = core.Verification{Verified: true, Signed: true, Reason: "valid"}
	top.Committer = core.Signature{Name: "GitHub", Email: "noreply@github.com", Login: "web-flow", Date: top.Author.Date.Add(2 * time.Hour)}
	return out
}

// detail returns what c changed: two files, one with a patch.
func detail(c core.Commit) core.CommitDetail {
	return core.CommitDetail{
		Commit: c,
		Stats:  core.CommitStats{Additions: 50, Deletions: 4, Total: 54},
		Files: []core.CommitFile{
			{Path: "commands.go", Status: core.FileModified, Additions: 42, Deletions: 3,
				Patch: "@@ -120,6 +120,12 @@ func Sequence(cmds ...Cmd) Cmd {\n \treturn func() Msg {\n-\t\treturn nil\n+\t\treturn sequenceMsg(cmds)\n \t}\n"},
			{Path: "tea.go", Status: core.FileModified, Additions: 8, Deletions: 1, Patch: "@@ -1 +1 @@\n-old\n+new\n"},
		},
	}
}

func (f *fake) record(call string) {
	f.calls = append(f.calls, call)
}

// took returns the calls made, and forgets them.
func (f *fake) took() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

func (f *fake) Branches(ctx context.Context, q historysvc.BranchesQuery) (core.Page[core.Branch], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("branches" + at(q.Cursor))
	if err := f.errs["branches"]; err != nil {
		return core.Page[core.Branch]{}, err
	}
	if err := ctx.Err(); err != nil {
		return core.Page[core.Branch]{}, err
	}
	return core.Page[core.Branch]{Items: slices.Clone(f.branches)}, nil
}

func at(cursor string) string {
	if cursor == "" {
		return ""
	}
	return "@" + cursor
}

// commitPage is how many commits the fake lists on a page, whatever the
// page size asked for.
const commitPage = 50

func (f *fake) Commits(ctx context.Context, q historysvc.CommitsQuery) (core.Page[core.Commit], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("commits " + q.Ref + at(q.Cursor))
	if err := f.errs["commits "+q.Ref]; err != nil {
		return core.Page[core.Commit]{}, err
	}
	if err := ctx.Err(); err != nil {
		return core.Page[core.Commit]{}, err
	}
	h := f.histories[q.Ref]
	start, _ := strconv.Atoi(q.Cursor)
	end := min(start+commitPage, len(h))
	p := core.Page[core.Commit]{Items: slices.Clone(h[start:end])}
	if end < len(h) {
		p.Next = strconv.Itoa(end)
	}
	if f.stale && q.Cursor == "" {
		f.stale, p.Stale = false, true
	}
	return p, nil
}

func (f *fake) CachedCommit(_ core.RepoRef, sha string) (core.CommitDetail, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cached[sha] {
		return core.CommitDetail{}, false
	}
	return f.details[sha], true
}

func (f *fake) Commit(ctx context.Context, _ core.RepoRef, sha string) (core.CommitDetail, error) {
	f.mu.Lock()
	f.record("commit " + short(sha))
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			f.mu.Lock()
			f.cancelled = append(f.cancelled, short(sha))
			f.mu.Unlock()
			return core.CommitDetail{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["commit "+short(sha)]; err != nil {
		return core.CommitDetail{}, err
	}
	d, ok := f.details[sha]
	if !ok {
		return core.CommitDetail{}, core.ErrNotFound
	}
	f.cached[sha] = true
	return d, nil
}

func (f *fake) CommitFiles(ctx context.Context, q historysvc.CommitFilesQuery) (core.Page[core.CommitFile], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("files " + short(q.SHA) + at(q.Cursor))
	if err := ctx.Err(); err != nil {
		return core.Page[core.CommitFile]{}, err
	}
	pages := f.moreFiles[q.SHA]
	i, _ := strconv.Atoi(q.Cursor)
	if i < 1 || i > len(pages) {
		return core.Page[core.CommitFile]{}, core.ErrNotFound
	}
	p := core.Page[core.CommitFile]{Items: pages[i-1]}
	if i < len(pages) {
		p.Next = strconv.Itoa(i + 1)
	}
	return p, nil
}

func (f *fake) CachedCompare(_ core.RepoRef, _, _ string) (core.Compare, bool) {
	return core.Compare{}, false
}

func (f *fake) Compare(ctx context.Context, _ core.RepoRef, base, head string) (core.Compare, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("compare " + head)
	if base != "main" {
		return core.Compare{}, fmt.Errorf("compare with %q, want main", base)
	}
	return f.compares[head], ctx.Err()
}
