package issues

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

var _ PullCache = (*pulls.Service)(nil)

// fakePulls holds the pull request details cached, by number.
type fakePulls map[int]bool

func (f fakePulls) CachedGet(r core.RepoRef, number int) (core.PullRequestDetail, bool) {
	if r != repo || !f[number] {
		return core.PullRequestDetail{}, false
	}
	return core.PullRequestDetail{}, true
}

// kindAPI is a GitHub where 7 is an issue and 8 a pull request, 10 is a
// deleted issue, and no other number is either.
func kindAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{t: t, getIssueKind: func(number int) (core.NumberKind, core.Issue, github.Response, error) {
		res := github.Response{StatusCode: 200, ETag: fmt.Sprintf(`"e%d"`, number), URL: fmt.Sprintf("https://api.github.com/repos/octo-org/hello/issues/%d", number)}
		switch number {
		case 7:
			return core.KindIssue, issue(7), res, nil
		case 8:
			return core.KindPull, core.Issue{}, res, nil
		case 10:
			return "", core.Issue{}, github.Response{StatusCode: 410}, fmt.Errorf("GET issues/10: %w", &github.Error{StatusCode: 410, Message: "This issue was deleted"})
		}
		return "", core.Issue{}, github.Response{StatusCode: 404}, fmt.Errorf("GET issues/%d: %w", number, core.ErrNotFound)
	}}
}

func kind(t *testing.T, s *Service, number int) core.NumberKind {
	t.Helper()
	k, err := s.Kind(t.Context(), repo, number)
	if err != nil {
		t.Fatalf("Kind(%d): %v", number, err)
	}
	return k
}

func TestKindFromPullCache(t *testing.T) {
	api := &fakeAPI{t: t}
	s := New(api, WithPulls(fakePulls{8: true}))
	if k, ok := s.CachedKind(repo, 8); !ok || k != core.KindPull {
		t.Errorf("CachedKind(8) = %q, %v; want pull", k, ok)
	}
	if k := kind(t, s, 8); k != core.KindPull {
		t.Errorf("Kind(8) = %q, want pull", k)
	}
	if _, ok := s.CachedKind(repo, 7); ok {
		t.Error("CachedKind(7) = true, want false for a number not seen")
	}
	api.checkCalls(t)
}

// GetIssue returns the issue part of a pull request as well, so a cached
// issue doesn't tell that its number is an issue.
func TestKindDoesNotTrustCachedIssues(t *testing.T) {
	store := openStore(t)
	api := kindAPI(t)
	api.getIssue = func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
		return issue(number), github.Response{ETag: `"e8"`}, nil
	}
	s := New(api, WithStore(store))
	if _, err := s.Get(t.Context(), repo, 8); err != nil {
		t.Fatal(err)
	}
	if k, ok := s.CachedKind(repo, 8); ok {
		t.Errorf("CachedKind of pull request 8 read by Get = %q, want unknown", k)
	}
	if k := kind(t, s, 8); k != core.KindPull {
		t.Errorf("Kind of pull request 8 read by Get = %q, want pull", k)
	}
	api.checkCalls(t, "GetIssue", "GetIssueKind")

	// Nor does what an earlier session kept of it.
	next := kindAPI(t)
	if k := kind(t, New(next, WithStore(store)), 8); k != core.KindPull {
		t.Errorf("Kind in a new session = %q, want pull", k)
	}
	next.checkCalls(t)
	fresh := openStore(t)
	first := New(api, WithStore(fresh))
	if _, err := first.Get(t.Context(), repo, 8); err != nil {
		t.Fatal(err)
	}
	api.checkCalls(t, "GetIssue")
	later := kindAPI(t)
	if k := kind(t, New(later, WithStore(fresh)), 8); k != core.KindPull {
		t.Errorf("Kind with pull request 8 kept as an issue = %q, want pull", k)
	}
	later.checkCalls(t, "GetIssueKind")
}

func TestKindFetchesOnceAndRemembers(t *testing.T) {
	for _, tt := range []struct {
		number int
		want   core.NumberKind
	}{{7, core.KindIssue}, {8, core.KindPull}} {
		t.Run(string(tt.want), func(t *testing.T) {
			api := kindAPI(t)
			s := New(api)
			for range 2 {
				if k := kind(t, s, tt.number); k != tt.want {
					t.Errorf("Kind = %q, want %q", k, tt.want)
				}
			}
			api.checkCalls(t, "GetIssueKind")
			if k, ok := s.CachedKind(repo, tt.number); !ok || k != tt.want {
				t.Errorf("CachedKind = %q, %v; want %q", k, ok, tt.want)
			}
		})
	}
}

// The response that says a number is an issue is the issue, so Get needs
// no request of its own; a pull request's is not an issue at all.
func TestKindWarmsOnlyIssues(t *testing.T) {
	store := openStore(t)
	api := kindAPI(t)
	s := New(api, WithStore(store))
	kind(t, s, 7)
	kind(t, s, 8)
	api.checkCalls(t, "GetIssueKind", "GetIssueKind")

	it, err := s.Get(t.Context(), repo, 7)
	if err != nil || it.Number != 7 {
		t.Fatalf("Get(7) = %+v, %v; want the issue Kind read", it, err)
	}
	api.checkCalls(t)
	kept := cache.NewShelf[core.Issue](store, kindIssue, schema)
	if e, ok := kept.Load(issueKey(repo, 7)); !ok || e.ETag != `"e7"` || e.Source == "" {
		t.Errorf("kept issue 7 = %+v, %v; want it with its validators", e, ok)
	}

	if _, ok := s.CachedGet(repo, 8); ok {
		t.Error("CachedGet(8) = true, want a pull request's issue part left out")
	}
	if _, ok := kept.Load(issueKey(repo, 8)); ok {
		t.Error("the store keeps pull request 8 as an issue")
	}
}

func TestKindWarmKeepsFreshIssue(t *testing.T) {
	api := kindAPI(t)
	api.getIssue = func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
		it := issue(number)
		it.Title = "Read by Get"
		return it, github.Response{}, nil
	}
	s := New(api)
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatal(err)
	}
	kind(t, s, 7)
	if it, _ := s.CachedGet(repo, 7); it.Title != "Read by Get" {
		t.Errorf("cached issue = %+v, want the one Get read", it)
	}
	api.checkCalls(t, "GetIssue", "GetIssueKind")
}

func TestKindKeptForANewSession(t *testing.T) {
	store := openStore(t)
	first := New(kindAPI(t), WithStore(store))
	kind(t, first, 7)
	kind(t, first, 8)

	// The next session asks GitHub nothing, and fails if it does.
	api := &fakeAPI{t: t}
	s := New(api, WithStore(store))
	if _, ok := s.CachedKind(repo, 8); ok {
		t.Error("CachedKind read the store")
	}
	for number, want := range map[int]core.NumberKind{7: core.KindIssue, 8: core.KindPull} {
		if k := kind(t, s, number); k != want {
			t.Errorf("Kind(%d) in a new session = %q, want %q", number, k, want)
		}
		if k, ok := s.CachedKind(repo, number); !ok || k != want {
			t.Errorf("CachedKind(%d) after Kind = %q, %v; want %q", number, k, ok, want)
		}
	}
	api.checkCalls(t)
}

// A kind needs no revalidating, so the revalidator isn't given it, only
// the issue that came with one.
func TestKindIsNotListedForRevalidation(t *testing.T) {
	store := openStore(t)
	s := New(kindAPI(t), WithStore(store))
	kind(t, s, 7)
	kind(t, s, 8)
	kept := s.Kept()
	ids := make([]string, 0, len(kept))
	for _, e := range kept {
		ids = append(ids, e.ID)
	}
	if len(ids) != 1 || !strings.Contains(ids[0], issueKey(repo, 7)) {
		t.Errorf("Kept = %q, want issue 7 alone", ids)
	}
}

// A 404 is a number that is neither, or one the account may not see, and
// a 410 a deleted issue or a number of a repository with its issues
// turned off.
func TestKindNotFoundIsNotRemembered(t *testing.T) {
	for _, number := range []int{9, 10} {
		t.Run(strconv.Itoa(number), func(t *testing.T) {
			store := openStore(t)
			api := kindAPI(t)
			s := New(api, WithStore(store))
			for range 2 {
				_, err := s.Kind(t.Context(), repo, number)
				nf, ok := errors.AsType[*core.NoNumberError](err)
				if !ok || nf.Repo != repo || nf.Number != number {
					t.Fatalf("Kind error = %v, want a *core.NoNumberError for it", err)
				}
				if !github.Refused(err) || github.Unreachable(t.Context(), err) {
					t.Errorf("Kind error = %v, want it refused", err)
				}
			}
			api.checkCalls(t, "GetIssueKind", "GetIssueKind")
			if _, ok := s.CachedKind(repo, number); ok {
				t.Error("CachedKind = true, want nothing remembered")
			}
			next := kindAPI(t)
			if _, err := New(next, WithStore(store)).Kind(t.Context(), repo, number); err == nil {
				t.Error("Kind in a new session succeeded, want it asked again and missing")
			}
			next.checkCalls(t, "GetIssueKind")
		})
	}
}

func TestKindErrors(t *testing.T) {
	offline := fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Get", URL: "https://api.github.com", Err: errors.New("no route to host")})
	limited := fmt.Errorf("GET issues/7: %w", &core.RateLimitError{})
	for name, tt := range map[string]struct {
		err         error
		unreachable bool
	}{
		"offline":      {offline, true},
		"rate limited": {limited, false},
	} {
		t.Run(name, func(t *testing.T) {
			store := openStore(t)
			fail := &fakeAPI{t: t, getIssueKind: func(int) (core.NumberKind, core.Issue, github.Response, error) {
				return "", core.Issue{}, github.Response{}, tt.err
			}}
			s := New(fail, WithStore(store))
			_, err := s.Kind(t.Context(), repo, 7)
			if !errors.Is(err, tt.err) || github.Unreachable(t.Context(), err) != tt.unreachable || github.Refused(err) {
				t.Errorf("Kind error = %v, want %v, unreachable %v and not refused", err, tt.err, tt.unreachable)
			}
			if _, ok := s.CachedKind(repo, 7); ok {
				t.Error("CachedKind after a failure = true, want nothing remembered")
			}
			fail.checkCalls(t, "GetIssueKind")

			// A kind known before is served without GitHub.
			kind(t, New(kindAPI(t), WithStore(store)), 8)
			s = New(fail, WithStore(store))
			if k := kind(t, s, 8); k != core.KindPull {
				t.Errorf("Kind of a kept number without GitHub = %q, want pull", k)
			}
			fail.checkCalls(t)
		})
	}
}

func TestKindLogs(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelDebug, "s_test"))
	t.Cleanup(func() { slog.SetDefault(prev) })

	api := kindAPI(t)
	api.getIssueKind = func(int) (core.NumberKind, core.Issue, github.Response, error) {
		it := issue(7)
		it.Body = "secret body"
		return core.KindIssue, it, github.Response{ETag: `"e7"`}, nil
	}
	s := New(api)
	kind(t, s, 7)
	kind(t, s, 7)

	var found []string
	for line := range strings.Lines(buf.String()) {
		var r struct {
			Msg   string `json:"msg"`
			Span  string `json:"span"`
			Level string `json:"level"`
			Found string `json:"found"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if r.Span == span && r.Msg == "number kind" {
			if r.Level != "DEBUG" {
				t.Errorf("lookup logged at %s, want DEBUG", r.Level)
			}
			found = append(found, r.Found)
		}
	}
	if want := []string{"miss", "memory"}; !slices.Equal(found, want) {
		t.Errorf("lookups logged = %q, want %q", found, want)
	}
	if !strings.Contains(buf.String(), `"msg":"number resolved"`) {
		t.Error("no record of the number resolved")
	}
	if strings.Contains(buf.String(), "secret body") {
		t.Error("the log holds the issue's body")
	}
}
