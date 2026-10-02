package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cmdhist"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	accesssvc "github.com/eggzec/gh-tui/internal/service/access"
)

// loginDisk returns a disk cache config in a directory of the test's.
func loginDisk(t *testing.T) config.Disk {
	t.Helper()
	cfg := config.Default().Cache.Disk
	cfg.Dir = t.TempDir()
	return cfg
}

// testNow is when the tests' sessions start.
var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// envToken is a token from the environment, which names no login.
var envToken = accesssvc.Token{Value: fakeToken, Source: "GH_TOKEN"}

func TestStartLogin(t *testing.T) {
	t.Run("gh's own token", func(t *testing.T) {
		got := startLogin(loginDisk(t), "github.com", accesssvc.Token{Value: fakeToken, Source: "gh", Login: "octocat"}, testNow)
		if got.login != "octocat" || got.from != "gh" || got.path != "" {
			t.Errorf("startLogin = %+v, want gh's login and nothing kept", got)
		}
	})
	t.Run("first start with a token", func(t *testing.T) {
		got := startLogin(loginDisk(t), "github.com", envToken, testNow)
		if got.login != "" || got.from != "" || got.path == "" {
			t.Errorf("startLogin = %+v, want no login yet and a place to keep it", got)
		}
	})
	t.Run("a kept login", func(t *testing.T) {
		cfg := loginDisk(t)
		path := loginPath(cfg, "github.com", envToken.Value)
		if err := writeLogin(path, keptLogin{Login: "octocat", ETag: `"v1"`}); err != nil {
			t.Fatal(err)
		}
		got := startLogin(cfg, "github.com", envToken, testNow)
		if got.login != "octocat" || got.from != "kept" || got.kept.ETag != `"v1"` {
			t.Errorf("startLogin = %+v, want the kept login", got)
		}
		if other := startLogin(cfg, "ghe.corp", envToken, testNow); other.login != "" {
			t.Errorf("startLogin on another host = %+v, want no login", other)
		}
	})
	t.Run("disk cache off", func(t *testing.T) {
		cfg := loginDisk(t)
		cfg.Enabled = false
		if got := startLogin(cfg, "github.com", envToken, testNow); got != (sessionLogin{}) {
			t.Errorf("startLogin = %+v, want nothing", got)
		}
	})
}

// The kept login names neither the token nor is it readable by others.
func TestLoginPath(t *testing.T) {
	cfg := loginDisk(t)
	path := loginPath(cfg, "github.com", fakeToken)
	if strings.Contains(path, fakeToken) || filepath.Base(path) != loginFile {
		t.Errorf("path = %q, want a hash of the token and %s", path, loginFile)
	}
	if err := writeLogin(path, keptLogin{Login: "octocat"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("stat = %v, %v; want mode 0600", fi, err)
	}
}

// fakeUser answers as GET /user would, and records the validators it got.
type fakeUser struct {
	login string
	etag  string
	err   error
	asked []github.Conditional
}

func (f *fakeUser) get(_ context.Context, cond github.Conditional) (string, github.Response, error) {
	f.asked = append(f.asked, cond)
	if f.err != nil {
		return "", github.Response{}, f.err
	}
	if cond.ETag != "" && cond.ETag == f.etag {
		return "", github.Response{StatusCode: 304, NotModified: true}, nil
	}
	return f.login, github.Response{StatusCode: 200, ETag: f.etag}, nil
}

// profiles is the profile of each login, as File.Resolve would pick it.
func profiles(byLogin map[string]string) func(string) string {
	return func(login string) string { return byLogin[strings.ToLower(login)] }
}

func TestLearnLogin(t *testing.T) {
	work := profiles(map[string]string{"octocat": "work"})
	learn := func(t *testing.T, s sessionLogin, u *fakeUser, profile func(string) string) (kept keptLogin, warned []string) {
		t.Helper()
		learnLogin(t.Context(), s, u.get, profile, func(w string) { warned = append(warned, w) }, func() time.Time { return testNow })
		return readLogin(s.path), warned
	}

	t.Run("first start keeps the login for the next", func(t *testing.T) {
		s := startLogin(loginDisk(t), "github.com", envToken, testNow)
		u := &fakeUser{login: "octocat", etag: `"v1"`}
		kept, warned := learn(t, s, u, work)
		if kept != (keptLogin{Login: "octocat", ETag: `"v1"`}) {
			t.Errorf("kept %+v, want octocat with its ETag", kept)
		}
		if len(warned) != 1 || warned[0] != "Profile work applies from the next start: the token is octocat's." {
			t.Errorf("warned %q, want that profile work applies from the next start", warned)
		}
	})
	t.Run("first start, no profile lists the login", func(t *testing.T) {
		s := startLogin(loginDisk(t), "github.com", envToken, testNow)
		kept, warned := learn(t, s, &fakeUser{login: "hubot"}, work)
		if kept.Login != "hubot" || len(warned) != 0 {
			t.Errorf("kept %+v, warned %q; want hubot kept and no warning, since nothing changes", kept, warned)
		}
	})
	t.Run("a kept login is asked about with its ETag", func(t *testing.T) {
		cfg := loginDisk(t)
		path := loginPath(cfg, "github.com", envToken.Value)
		if err := writeLogin(path, keptLogin{Login: "octocat", ETag: `"v1"`}); err != nil {
			t.Fatal(err)
		}
		u := &fakeUser{login: "octocat", etag: `"v1"`}
		kept, warned := learn(t, startLogin(cfg, "github.com", envToken, testNow), u, work)
		if len(u.asked) != 1 || u.asked[0].ETag != `"v1"` {
			t.Errorf("asked with %+v, want the kept ETag", u.asked)
		}
		if kept.Login != "octocat" || len(warned) != 0 {
			t.Errorf("kept %+v, warned %q; want it as it was", kept, warned)
		}
	})
	t.Run("a stale kept login is replaced", func(t *testing.T) {
		cfg := loginDisk(t)
		path := loginPath(cfg, "github.com", envToken.Value)
		if err := writeLogin(path, keptLogin{Login: "octocat", ETag: `"v1"`}); err != nil {
			t.Fatal(err)
		}
		kept, warned := learn(t, startLogin(cfg, "github.com", envToken, testNow), &fakeUser{login: "hubot", etag: `"v2"`}, work)
		if kept != (keptLogin{Login: "hubot", ETag: `"v2"`}) {
			t.Errorf("kept %+v, want hubot", kept)
		}
		if len(warned) != 1 || !strings.HasPrefix(warned[0], "No profile applies from the next start") {
			t.Errorf("warned %q, want that no profile applies from the next start", warned)
		}
	})
	t.Run("a failure keeps nothing", func(t *testing.T) {
		s := startLogin(loginDisk(t), "github.com", envToken, testNow)
		kept, warned := learn(t, s, &fakeUser{err: errors.New("offline")}, work)
		if kept != (keptLogin{}) || len(warned) != 0 {
			t.Errorf("kept %+v, warned %q; want nothing", kept, warned)
		}
		if _, err := os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s = %v, want no file", s.path, err)
		}
	})
	t.Run("gh's own token asks nothing", func(t *testing.T) {
		u := &fakeUser{login: "octocat"}
		learnLogin(t.Context(), startLogin(loginDisk(t), "github.com", accesssvc.Token{Value: fakeToken, Source: "gh", Login: "octocat"}, testNow),
			u.get, work, func(string) { t.Error("warned") }, time.Now)
		if len(u.asked) != 0 {
			t.Errorf("asked %d times, want none", len(u.asked))
		}
	})
}

// Once the login of a token from the environment is kept, the next start
// names the account by it, and what the account kept under the token's
// name moves to the login's.
func TestKeptLoginMovesAccount(t *testing.T) {
	cfg := loginDisk(t)
	const host = "github.com"
	newClient := func(login string) *github.Client {
		t.Helper()
		c, err := github.New(github.WithHost(host), github.WithToken(envToken.Value), github.WithLogin(login))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		return c
	}
	first := newClient(startLogin(cfg, host, envToken, testNow).login)
	if first.Account() != first.TokenAccount() {
		t.Fatal("the first start names the account by its login")
	}
	entries := filepath.Join(cfg.Dir, hostDir(first.Host()), entryDir)
	if err := os.MkdirAll(filepath.Join(entries, first.Account()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entries, first.Account(), cmdhist.FileName), []byte("goto cli/cli\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeLogin(loginPath(cfg, host, envToken.Value), keptLogin{Login: "octocat"}); err != nil {
		t.Fatal(err)
	}

	if path := loginPath(cfg, host, envToken.Value); !strings.HasPrefix(path, filepath.Join(cfg.Dir, hostDir(first.Host()))+string(filepath.Separator)) {
		t.Errorf("the login is kept at %s, outside the disk cache of %s", path, first.Host())
	}
	next := newClient(startLogin(cfg, host, envToken, testNow).login)
	if next.Account() == next.TokenAccount() {
		t.Fatal("the next start doesn't name the account by its kept login")
	}
	moveAccount(cfg, next.Host(), next.TokenAccount(), next.Account())
	if b, err := os.ReadFile(filepath.Join(entries, next.Account(), cmdhist.FileName)); err != nil || string(b) != "goto cli/cli\n" {
		t.Errorf("history under the login = %q, %v; want the token's", b, err)
	}
}

// The login is kept below the disk cache of the API host, where openDisk
// keeps everything of the host, so the cache's sweep keeps it.
func TestLoginPathUnderAPIHost(t *testing.T) {
	cfg := loginDisk(t)
	for host, dir := range map[string]string{"github.com": "api.github.com", "ghe.corp.com": "ghe.corp.com"} {
		want := filepath.Join(cfg.Dir, dir, tokenDir) + string(filepath.Separator)
		if got := loginPath(cfg, host, fakeToken); !strings.HasPrefix(got, want) {
			t.Errorf("loginPath(%s) = %q, want it below %q", host, got, want)
		}
	}
}

// A kept file the app didn't write as it does is taken for no login: one
// whose login isn't one GitHub could have, a big one, or a link.
func TestReadLoginRefuses(t *testing.T) {
	cfg := loginDisk(t)
	write := func(t *testing.T, data string) string {
		t.Helper()
		path := loginPath(cfg, "github.com", t.Name())
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("escape sequences", func(t *testing.T) {
		if k := readLogin(write(t, `{"login":"octo\u001b[2Jcat"}`)); k != (keptLogin{}) {
			t.Errorf("readLogin = %+v, want none", k)
		}
	})
	t.Run("too big", func(t *testing.T) {
		if k := readLogin(write(t, `{"login":"octocat"}`+strings.Repeat(" ", maxLoginFile))); k != (keptLogin{}) {
			t.Errorf("readLogin = %+v, want none", k)
		}
	})
	t.Run("a link", func(t *testing.T) {
		target := write(t, `{"login":"octocat"}`)
		link := filepath.Join(filepath.Dir(target), "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skip("no symbolic links:", err)
		}
		if k := readLogin(link); k != (keptLogin{}) {
			t.Errorf("readLogin = %+v, want none", k)
		}
	})
	t.Run("as written", func(t *testing.T) {
		if k := readLogin(write(t, `{"login":"octocat","etag":"\"v1\""}`)); k.Login != "octocat" {
			t.Errorf("readLogin = %+v, want octocat", k)
		}
	})
}

// Without a kept login, GitHub is asked without the ETag, which would
// otherwise answer 304 and never name the login.
func TestLearnLoginETagOnlyWithLogin(t *testing.T) {
	cfg := loginDisk(t)
	path := loginPath(cfg, "github.com", envToken.Value)
	if err := writeLogin(path, keptLogin{ETag: `"v1"`}); err != nil {
		t.Fatal(err)
	}
	u := &fakeUser{login: "octocat", etag: `"v1"`}
	learnLogin(t.Context(), startLogin(cfg, "github.com", envToken, testNow), u.get, profiles(nil), func(string) {}, time.Now)
	if len(u.asked) != 1 || u.asked[0].ETag != "" {
		t.Errorf("asked with %+v, want no ETag", u.asked)
	}
	if k := readLogin(path); k.Login != "octocat" {
		t.Errorf("kept %+v, want octocat", k)
	}
}

// A token that may not read its user is asked about again only after a
// day, since GitHub refuses such tokens every time.
func TestLearnLoginRefused(t *testing.T) {
	cfg := loginDisk(t)
	u := &fakeUser{err: fmt.Errorf("read user: %w", core.ErrForbidden)}
	learn := func(at time.Time) {
		learnLogin(t.Context(), startLogin(cfg, "github.com", envToken, at), u.get, profiles(nil),
			func(w string) { t.Errorf("warned %q", w) }, func() time.Time { return at })
	}
	learn(testNow)
	if k := readLogin(loginPath(cfg, "github.com", envToken.Value)); !k.Refused.Equal(testNow) || k.Login != "" {
		t.Fatalf("kept %+v, want the refusal", k)
	}
	learn(testNow.Add(time.Hour))
	if len(u.asked) != 1 {
		t.Errorf("asked %d times within the day, want once", len(u.asked))
	}
	learn(testNow.Add(refusedFor))
	if len(u.asked) != 2 {
		t.Errorf("asked %d times, want again after a day", len(u.asked))
	}
}

// A lookup the app's quitting cancels keeps nothing.
func TestLearnLoginCanceled(t *testing.T) {
	s := startLogin(loginDisk(t), "github.com", envToken, testNow)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	get := func(ctx context.Context, _ github.Conditional) (string, github.Response, error) {
		return "", github.Response{}, ctx.Err()
	}
	learnLogin(ctx, s, get, profiles(nil), func(w string) { t.Errorf("warned %q", w) }, time.Now)
	if _, err := os.Lstat(s.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat = %v, want nothing kept", err)
	}
}

// age sets when path last changed to at.
func age(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// keptToken keeps a login for token in cfg's disk cache, last used at, and
// returns its directory.
func keptToken(t *testing.T, cfg config.Disk, token string, at time.Time) string {
	t.Helper()
	path := loginPath(cfg, "github.com", token)
	if err := writeLogin(path, keptLogin{Login: "octocat"}); err != nil {
		t.Fatal(err)
	}
	age(t, path, at)
	age(t, filepath.Dir(path), at)
	return filepath.Dir(path)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestRemoveUnused(t *testing.T) {
	cfg := loginDisk(t)
	cutoff := testNow.Add(-unusedFor)
	old := keptToken(t, cfg, "old-token", cutoff.Add(-time.Hour))
	recent := keptToken(t, cfg, "recent-token", cutoff.Add(time.Hour))
	// The session's own token is kept however old its directory is.
	current := keptToken(t, cfg, fakeToken, cutoff.Add(-48*time.Hour))
	dir := filepath.Dir(old)
	// What isn't a token's directory is left alone.
	other := filepath.Join(dir, "notes")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, other, cutoff.Add(-time.Hour))

	removed, err := removeUnused(dir, tokenKey("github.com", fakeToken), cutoff)
	if err != nil || removed != 1 {
		t.Errorf("removeUnused = %d, %v; want 1 removed", removed, err)
	}
	for path, want := range map[string]bool{old: false, recent: true, current: true, other: true} {
		if got := exists(path); got != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(path), got, want)
		}
	}
}

// A directory whose own time is old is still used when its login file
// was touched since.
func TestRemoveUnusedReadsTheLoginFile(t *testing.T) {
	cfg := loginDisk(t)
	cutoff := testNow.Add(-unusedFor)
	dir := keptToken(t, cfg, "old-token", cutoff.Add(-time.Hour))
	age(t, filepath.Join(dir, loginFile), cutoff.Add(time.Hour))
	if removed, err := removeUnused(filepath.Dir(dir), "", cutoff); err != nil || removed != 0 {
		t.Errorf("removeUnused = %d, %v; want none removed", removed, err)
	}
}

// waitGone waits for the background sweep to remove path.
func waitGone(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for exists(path) {
		if time.Now().After(deadline) {
			t.Fatalf("%s is still there after the sweep", filepath.Base(path))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// settle waits until the sweeps started so far are done, by sweeping for
// real a directory of its own, which the others, running alike, have
// by then passed over.
func settle(t *testing.T) {
	t.Helper()
	cfg := loginDisk(t)
	old := keptToken(t, cfg, "settle-token", testNow.Add(-2*unusedFor))
	sweepTokens(cfg, "github.com", accesssvc.Token{Value: "other-token"}, testNow)
	waitGone(t, old)
}

func TestSweepTokensRemovesUnused(t *testing.T) {
	cfg := loginDisk(t)
	old := keptToken(t, cfg, "old-token", testNow.Add(-2*unusedFor))
	recent := keptToken(t, cfg, "recent-token", testNow.Add(-unusedFor/2))
	sweepTokens(cfg, "github.com", envToken, testNow)
	waitGone(t, old)
	if !exists(recent) {
		t.Error("a directory used within the span was swept")
	}
}

// The directory that the sweep keeps is the one that loginPath names for
// the same host and token, however old it is.
func TestSweepTokensKeepsTheSessionsDirectory(t *testing.T) {
	cfg := loginDisk(t)
	own := keptToken(t, cfg, fakeToken, testNow.Add(-2*unusedFor))
	if want := filepath.Dir(loginPath(cfg, "github.com", fakeToken)); own != want {
		t.Fatalf("kept directory %s, want %s", own, want)
	}
	old := keptToken(t, cfg, "old-token", testNow.Add(-2*unusedFor))
	sweepTokens(cfg, "github.com", envToken, testNow)
	waitGone(t, old)
	if !exists(own) {
		t.Error("the directory of the session's own token was swept")
	}
}

func TestSweepTokensDisabled(t *testing.T) {
	cfg := loginDisk(t)
	old := keptToken(t, cfg, "old-token", testNow.Add(-2*unusedFor))
	cfg.Enabled = false
	sweepTokens(cfg, "github.com", envToken, testNow)
	settle(t)
	if !exists(old) {
		t.Error("a sweep with the disk cache off removed a directory")
	}
}

// A token with neither a value nor a login, as one still on its way is,
// sweeps nothing: it can't be told from the others.
func TestSweepTokensNoToken(t *testing.T) {
	cfg := loginDisk(t)
	old := keptToken(t, cfg, "old-token", testNow.Add(-2*unusedFor))
	sweepTokens(cfg, "github.com", accesssvc.Token{Source: "gh"}, testNow)
	settle(t)
	if !exists(old) {
		t.Error("a sweep without a token removed a directory")
	}
}

func TestRemoveUnusedNoDirectory(t *testing.T) {
	if removed, err := removeUnused(filepath.Join(t.TempDir(), "token"), "", testNow); err != nil || removed != 0 {
		t.Errorf("removeUnused = %d, %v; want nothing and no error", removed, err)
	}
}

// A start with a kept login marks its directory used, so a sweep a while
// later keeps it.
func TestStartLoginMarksUsed(t *testing.T) {
	cfg := loginDisk(t)
	dir := keptToken(t, cfg, fakeToken, testNow.Add(-2*unusedFor))
	startLogin(cfg, "github.com", envToken, testNow)
	later := testNow.Add(unusedFor / 2)
	if removed, err := removeUnused(filepath.Dir(dir), "", later.Add(-unusedFor)); err != nil || removed != 0 || !exists(dir) {
		t.Errorf("removeUnused = %d, %v; want the directory kept", removed, err)
	}
}

func TestIsTokenKey(t *testing.T) {
	for name, want := range map[string]bool{
		tokenKey("github.com", fakeToken): true,
		"notes":                           false,
		strings.Repeat("A", 32):           false,
		strings.Repeat("a", 31):           false,
		strings.Repeat("a", 34):           false,
	} {
		if got := isTokenKey(name); got != want {
			t.Errorf("isTokenKey(%q) = %v, want %v", name, got, want)
		}
	}
}
