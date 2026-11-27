package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/config"
)

// scriptAccounts returns Accounts whose gh is a shell script with body, and whose
// checks go to handler. The script's own arguments are in $@.
func scriptAccounts(t *testing.T, body string, handler http.HandlerFunc) *Accounts {
	t.Helper()
	script := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := config.ReadFromString("hosts:\n  github.com:\n    users:\n      a: {}\n      b: {}\n")
	return &Accounts{
		gh: ghLookup{config: func() (*config.Config, error) { return cfg, nil }},
		command: func(ctx context.Context, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, script, args...)
		},
		// A transport of its own keeps the client off gh's unix socket.
		opts: []Option{WithBaseURL(srv.URL), WithHTTPClient(&http.Client{Transport: http.DefaultTransport})},
	}
}

func userHandler(login string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"message":"bad"}`, http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"login":"` + login + `"}`))
	}
}

func TestResolveAccount(t *testing.T) {
	a := scriptAccounts(t, `echo tok`, userHandler("Alice"))
	tok, err := a.ResolveAccount(t.Context(), "github.com", "alice")
	if err != nil || tok != "tok" {
		t.Fatalf("ResolveAccount = %q, %v; want tok", tok, err)
	}
}

// TestResolveAccountCase checks that a login typed in another case is given
// to gh as gh stores it, since gh matches logins exactly.
func TestResolveAccountCase(t *testing.T) {
	a := scriptAccounts(t, `[ "$6" = a ] && echo tok || { echo "got $6" >&2; exit 1; }`, userHandler("a"))
	tok, err := a.ResolveAccount(t.Context(), "github.com", "A")
	if err != nil || tok != "tok" {
		t.Fatalf("ResolveAccount = %q, %v; want tok", tok, err)
	}
	if got := a.Canonical("github.com", "B"); got != "b" {
		t.Errorf("Canonical(B) = %q, want b", got)
	}
	if got := a.Canonical("github.com", "zed"); got != "zed" {
		t.Errorf("Canonical(zed) = %q, want zed", got)
	}
	_, err = a.Token(t.Context(), "github.com", "ZED")
	var ae *AccountError
	if !errors.As(err, &ae) || len(ae.Logins) != 2 || ae.User != "ZED" {
		t.Fatalf("Token(ZED) error = %v, want an AccountError listing the logins", err)
	}
}

func TestResolveAccountGHError(t *testing.T) {
	a := scriptAccounts(t, `echo "no oauth token found for a" >&2; exit 1`, userHandler("a"))
	_, err := a.ResolveAccount(t.Context(), "github.com", "c")
	var ae *AccountError
	if !errors.As(err, &ae) || ae.Stderr != "no oauth token found for a" {
		t.Fatalf("err = %v; want AccountError carrying gh's stderr", err)
	}
	for _, want := range []string{"no oauth token found for a", "logged in on github.com: a, b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q; want it to contain %q", err, want)
		}
	}
}

func TestResolveAccountEmptyOutput(t *testing.T) {
	a := scriptAccounts(t, `echo ""`, userHandler("a"))
	_, err := a.ResolveAccount(t.Context(), "github.com", "a")
	if _, ok := errors.AsType[*AccountError](err); !ok {
		t.Fatalf("err = %v; want AccountError", err)
	}
}

// The child inherits the environment, so a token gh finds only there
// still works, and it is told the host.
func TestAccountsChildEnv(t *testing.T) {
	t.Setenv("GH_TOKEN", "env-token")
	t.Setenv("GH_HOST", "elsewhere.example")
	a := scriptAccounts(t, `echo "$GH_TOKEN|$GH_HOST|$*"`, userHandler("a"))
	got, err := a.Token(t.Context(), "github.com", "a")
	want := "env-token|github.com|auth token --hostname github.com --user a"
	if err != nil || got != want {
		t.Fatalf("Token = %q, %v; want %q", got, err, want)
	}
}

func TestResolveAccountRateLimited(t *testing.T) {
	a := scriptAccounts(t, `echo tok`, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	})
	if _, err := a.ResolveAccount(t.Context(), "github.com", "a"); err == nil {
		t.Fatal("ResolveAccount succeeded on a failing check")
	}
}

func TestResolveAccountLoginMismatch(t *testing.T) {
	a := scriptAccounts(t, `echo tok`, userHandler("mallory"))
	_, err := a.ResolveAccount(t.Context(), "github.com", "a")
	if err == nil || !strings.Contains(err.Error(), "is for mallory") {
		t.Fatalf("err = %v; want a login mismatch", err)
	}
}
