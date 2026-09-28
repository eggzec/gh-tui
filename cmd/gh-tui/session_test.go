package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/obs"
	accesssvc "github.com/eggzec/gh-tui/internal/service/access"
)

// TestLogSession logs the session record as build does, and checks that
// it says who the session is without its token, and that every record
// after it names the host and the account.
func TestLogSession(t *testing.T) {
	restoreLogger(t)
	dir := t.TempDir()
	// The paths below the home directory are logged with it as ~, and
	// wherever the temporary directory is, it isn't below this home.
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("GH_CONFIG_DIR", dir)
	t.Setenv("GH_TOKEN", fakeToken)
	t.Setenv("GH_HOST", "ghe.corp")
	t.Setenv("GH_PATH", filepath.Join(dir, "home", "bin", "gh"))
	cfg := config.Default()
	cfg.Log.File = filepath.Join(dir, "gh-tui.log")
	cfg.Cache.Disk.Dir = filepath.Join(dir, "cache")
	closeLog, warning := openLog(cfg.Log)
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	slog.Info("before")

	st := start{Host: "ghe.corp", From: "GH_HOST", Here: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}}
	logHost(st.Host)
	client, err := github.New(github.WithHost(st.Host), github.WithToken(fakeToken),
		github.WithHTTPClient(&http.Client{Transport: http.DefaultTransport}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	info := newSessionInfo(st, accesssvc.Token{Value: fakeToken, Source: "GH_TOKEN"}, client, cfg.Cache.Disk)
	logSession(info)

	slog.Info("after")
	ctx, end := obs.Begin(context.Background(), "test.trace")
	slog.WarnContext(ctx, "traced")
	end(errors.New("boom"))
	closeLog()

	data, err := os.ReadFile(cfg.Log.File)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), fakeToken) {
		t.Fatalf("the log gives the token away: %s", data)
	}
	recs := readRecords(t, data)
	if len(recs) != 5 {
		t.Fatalf("records = %v, want 5", recs)
	}
	if _, ok := recs[0]["host"]; ok {
		t.Errorf("a record before the host was picked names one: %v", recs[0])
	}
	sess := recs[1]
	for key, want := range map[string]any{
		"msg": "session", "host": "ghe.corp", "host_from": "GH_HOST", "here": "eggzec/gh-tui", "web_host": "ghe.corp",
		"login": "", "token_source": "GH_TOKEN", "token_kind": "classic", "account": client.Account(),
		"cache_dir": filepath.Join(dir, "cache", "ghe.corp", "entry", client.Account()),
		"gh_path":   filepath.Join("~", "bin", "gh"),
		"span":      "app", "gh_hosts": 2.0, "gh_knows_host": true,
	} {
		if sess[key] != want {
			t.Errorf("session %s = %v, want %v", key, sess[key], want)
		}
	}
	// Neither key is written twice, as the logger's and the record's.
	line := strings.Split(string(data), "\n")[1]
	if strings.Count(line, `"host":`) != 1 || strings.Count(line, `"account":`) != 1 {
		t.Errorf("session record = %s, want host and account once", line)
	}
	for _, rec := range recs[2:] {
		if rec["host"] != "ghe.corp" || rec["account"] != client.Account() {
			t.Errorf("record %v doesn't name the host and account", rec)
		}
	}
}

func TestProxyHost(t *testing.T) {
	withCreds := func(*http.Request) (*url.URL, error) {
		return url.Parse("http://someone:" + fakeToken + "@proxy.corp:3128")
	}
	if got := proxyHost("https://api.github.com/", withCreds); got != "proxy.corp:3128" {
		t.Errorf("proxyHost = %q, want the host alone", got)
	}
	none := func(*http.Request) (*url.URL, error) { return nil, nil }
	if got := proxyHost("https://api.github.com/", none); got != "" {
		t.Errorf("proxyHost without a proxy = %q, want none", got)
	}
}
