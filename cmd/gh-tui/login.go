package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/obs"
	accesssvc "github.com/eggzec/gh-tui/internal/service/access"
)

// A token from the environment, such as GH_TOKEN, names no login, so the
// app asks GitHub whose it is, once, in the background, and keeps the
// answer by a hash of the host and the token. A later start with the same
// token knows the login at once, without waiting for GitHub, so the
// profile of that account applies and its cache is named by its login.
// The first start with a token doesn't wait for the answer either: it
// starts without a profile, and the login applies from the next start.
const (
	// tokenDir holds a directory for each token from the environment,
	// below the disk cache of its host.
	tokenDir = "token"
	// loginFile is the file in it that keeps the token's login. The disk
	// cache never trims it.
	loginFile = "login"
	// maxLoginFile is the most of a login file that is read: what it
	// keeps takes a few dozen bytes, so a bigger one isn't the app's.
	maxLoginFile = 4 << 10
	// tmpPrefix starts the name of a login file being written, as the
	// disk cache names its own, so that its sweep removes one left behind.
	tmpPrefix = ".tmp-"
	// refusedFor is how long a token that may not read its user, such as
	// an Actions or an app installation's token, isn't asked about again:
	// GitHub refuses those every time.
	refusedFor = 24 * time.Hour
	// unusedFor is how long the directory of a token from the environment
	// is kept after the last start with that token. A token that was
	// rotated is never used again, so its directory would stay for good;
	// one that comes back after this only starts once without a profile
	// while its login is asked about again.
	unusedFor = 30 * 24 * time.Hour
)

// keptLogin is the login of a token from the environment as GitHub last
// named it, with the ETag of that answer, so that asking again costs
// nothing against the rate limit while it holds. Refused is when GitHub
// last refused to name it, with no login.
type keptLogin struct {
	Login   string    `json:"login,omitempty"`
	ETag    string    `json:"etag,omitempty"`
	Refused time.Time `json:"refused,omitzero"`
}

// sessionLogin is the login a session starts with, and where it is from.
type sessionLogin struct {
	// login is the login of the token's account, or "" when it isn't known
	// yet.
	login string
	// from says where the login came from: "gh" for the account gh stores
	// the token for, "kept" for one an earlier start learned, or "" for
	// none.
	from string
	// path is where the login of a token from the environment is kept, or
	// "" when nothing is kept: for gh's own token, or with the disk cache
	// off.
	path string
	// kept is what path held.
	kept keptLogin
	// ask says whether to ask GitHub for the login: not for gh's own
	// token, nor for one GitHub refused within refusedFor.
	ask bool
}

// startLogin returns the login of token, the token of host, that the
// session starts at now with: the one gh stores it for, else the one an
// earlier start learned and kept in the disk cache, cfg, else none. It
// never asks GitHub, so it never holds up the start.
func startLogin(cfg config.Disk, host string, token accesssvc.Token, now time.Time) sessionLogin {
	if token.Login != "" {
		return sessionLogin{login: token.Login, from: "gh"}
	}
	path := loginPath(cfg, host, token.Value)
	if path == "" {
		return sessionLogin{}
	}
	s := sessionLogin{path: path, kept: readLogin(path)}
	if s.kept != (keptLogin{}) {
		// The start marks the token's directory used, so that the sweep
		// of unused ones keeps it.
		_ = os.Chtimes(path, now, now)
	}
	if s.kept.Login != "" {
		s.login, s.from = s.kept.Login, "kept"
	}
	s.ask = s.kept.Login != "" || s.kept.Refused.IsZero() || now.Sub(s.kept.Refused) >= refusedFor
	return s
}

// loginPath returns where the login of token, a token of host from the
// environment, is kept, or "" when the disk cache is off: below the disk
// cache of the API host, where the app keeps everything of host.
func loginPath(cfg config.Disk, host, token string) string {
	if !cfg.Enabled || token == "" {
		return ""
	}
	root, err := cfg.Path()
	if err != nil {
		return ""
	}
	return filepath.Join(tokensPath(root, host), tokenKey(host, token), loginFile)
}

// tokensPath returns the directory that holds the directories of host's
// tokens from the environment, below the disk cache at root.
func tokensPath(root, host string) string {
	return filepath.Join(root, hostDir(github.APIHost(host)), tokenDir)
}

// sweepTokens removes, in the background, the directories of host's
// tokens from the environment that no start has used within unusedFor of
// now, but never the one of token, the session's own. It is only a
// cleanup, so a failure is only logged.
func sweepTokens(cfg config.Disk, host string, token accesssvc.Token, now time.Time) {
	if !cfg.Enabled {
		return
	}
	// A token from gh has no directory, and one still on its way can't be
	// told from the others yet.
	if token.Value == "" && token.Login == "" {
		return
	}
	root, err := cfg.Path()
	if err != nil {
		return
	}
	var keep string
	if token.Value != "" {
		keep = tokenKey(host, token.Value)
	}
	log := slog.Default()
	go func() {
		removed, err := removeUnused(tokensPath(root, host), keep, now.Add(-unusedFor))
		if removed > 0 {
			log.Info("token logins swept", "span", "login", "removed", removed)
		}
		if err != nil {
			log.Warn("token logins not swept", "span", "login", "err", err.Error())
		}
	}()
}

// removeUnused removes the directories of tokens in dir that were last
// used before cutoff, except keep, and returns how many it removed and
// why any of the others couldn't be. A directory is used when it or a
// file in it last changed: a start touches its login file. Only
// directories named as tokenKey names them are removed, so nothing else
// that may be in dir is. A start that overlaps another instance's sweep,
// or a session that runs for more than the span, can lose its cached
// login this way, which only costs one start without its profile.
func removeUnused(dir, keep string, cutoff time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var errs []error
	removed := 0
	for _, e := range entries {
		name := e.Name()
		// A symbolic link isn't followed: Type has no ModeDir for one.
		if name == keep || !e.Type().IsDir() || !isTokenKey(name) {
			continue
		}
		p := filepath.Join(dir, name)
		if lastUsed(p).After(cutoff) {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// lastUsed returns when dir or a file in it last changed.
func lastUsed(dir string) time.Time {
	var last time.Time
	if fi, err := os.Lstat(dir); err == nil {
		last = fi.ModTime()
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && fi.ModTime().After(last) {
			last = fi.ModTime()
		}
	}
	return last
}

// isTokenKey reports whether name is one tokenKey could return.
func isTokenKey(name string) bool {
	b, err := hex.DecodeString(name)
	return err == nil && len(b) == 16 && name == strings.ToLower(name)
}

// tokenKey names token, of host, without giving it away.
func tokenKey(host, token string) string {
	h := sha256.Sum256([]byte("gh-tui token login\x00" + host + "\x00" + token))
	return hex.EncodeToString(h[:16])
}

// readLogin returns the login kept at path, or none when there is none, it
// can't be read, or it isn't one the app wrote: not a regular file, too
// big, or not a login GitHub could have, since it is shown on screen.
func readLogin(path string) keptLogin {
	k, err := loadLogin(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("kept login not read", "span", "login", "err", err.Error())
		}
		return keptLogin{}
	}
	return k
}

// loadLogin reads the login kept at path.
func loadLogin(path string) (keptLogin, error) {
	// A symbolic link, or a pipe that would never end, isn't followed.
	fi, err := os.Lstat(path)
	if err != nil {
		return keptLogin{}, err
	}
	if !fi.Mode().IsRegular() {
		return keptLogin{}, fmt.Errorf("%s isn't a regular file", obs.ShortHome(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return keptLogin{}, err
	}
	defer f.Close()
	// What was opened must be what was looked at, not a file put there
	// since.
	if open, err := f.Stat(); err != nil || !os.SameFile(fi, open) {
		return keptLogin{}, fmt.Errorf("%s changed while it was read", obs.ShortHome(path))
	}
	data, err := io.ReadAll(io.LimitReader(f, maxLoginFile+1))
	if err != nil {
		return keptLogin{}, err
	}
	if len(data) > maxLoginFile {
		return keptLogin{}, fmt.Errorf("%s is bigger than %d bytes", obs.ShortHome(path), maxLoginFile)
	}
	var k keptLogin
	if err := json.Unmarshal(data, &k); err != nil {
		return keptLogin{}, err
	}
	if k.Login != "" && !core.ValidLogin(k.Login) {
		return keptLogin{}, fmt.Errorf("%s keeps no valid login", obs.ShortHome(path))
	}
	return k, nil
}

// writeLogin keeps k at path, for the user alone, replacing what was
// there at once, so that a start never reads half of it.
func writeLogin(path string, k keptLogin) error {
	data, err := json.Marshal(k)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// CreateTemp makes the file for the user alone.
	tmp, err := os.CreateTemp(dir, tmpPrefix+"login-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}

// userLogin asks GitHub for the login of the token, as Client.UserLogin
// does.
type userLogin func(ctx context.Context, cond github.Conditional) (string, github.Response, error)

// learnLogin asks GitHub, through get, whose token s is for, and keeps the
// answer at s.path for the next start. profile returns the profile of a
// login, "" for none. When the profile the answer selects isn't the one
// this session applies, warn is told that it applies from the next start,
// since a session never changes its config halfway. A failure only
// leaves the session as it started, without a login it didn't have. A
// refusal is kept, with the time now returns, so that the next starts
// don't ask again for a while.
func learnLogin(ctx context.Context, s sessionLogin, get userLogin, profile func(login string) string, warn func(string), now func() time.Time) {
	if !s.ask {
		return
	}
	// The ETag vouches for a kept login only: without one, a 304 would
	// never tell it.
	var cond github.Conditional
	if s.kept.Login != "" {
		cond.ETag = s.kept.ETag
	}
	ctx, end := obs.Begin(ctx, "login.learn")
	login, resp, err := get(ctx, cond)
	end(err, "span", "login")
	switch {
	case err == nil:
	case ctx.Err() != nil:
		slog.DebugContext(ctx, "login not learned: the app quit", "span", "login")
		return
	case errors.Is(err, core.ErrForbidden):
		slog.InfoContext(ctx, "login refused: the token may not read its user", "span", "login", "again_in", refusedFor.String())
		if s.kept.Login == "" {
			if err := writeLogin(s.path, keptLogin{Refused: now()}); err != nil {
				slog.WarnContext(ctx, "login not kept", "span", "login", "err", err.Error())
			}
		}
		return
	default:
		slog.WarnContext(ctx, "login not learned", "span", "login", "kept", s.kept.Login != "", "err", err.Error())
		return
	}
	if resp.NotModified {
		slog.DebugContext(ctx, "login unchanged", "span", "login")
		return
	}
	if err := writeLogin(s.path, keptLogin{Login: login, ETag: resp.ETag}); err != nil {
		slog.WarnContext(ctx, "login not kept", "span", "login", "err", err.Error())
		return
	}
	if strings.EqualFold(login, s.kept.Login) {
		return
	}
	slog.InfoContext(ctx, "login learned", "span", "login", "login", login, "was", s.kept.Login)
	if was, next := profile(s.kept.Login), profile(login); was != next {
		warn(profileChangeText(login, next))
	}
}

// profileChangeText tells that profile, "" for none, applies from the
// next start, since the token is login's.
func profileChangeText(login, profile string) string {
	if profile == "" {
		return "No profile applies from the next start: the token is " + login + "'s, which none lists."
	}
	return "Profile " + profile + " applies from the next start: the token is " + login + "'s."
}
