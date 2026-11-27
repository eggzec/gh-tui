package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Accounts finds the token of a named account on a host through the gh CLI,
// without changing which account gh has active, and checks with GitHub that
// the token is that account's.
type Accounts struct {
	gh ghLookup
	// command makes the gh command; tests replace it.
	command func(ctx context.Context, args ...string) *exec.Cmd
	// opts are added to the client that checks the token; tests point it at
	// a server.
	opts []Option
}

// NewAccounts returns the accounts of the gh CLI that is on the path.
func NewAccounts() *Accounts {
	return &Accounts{
		gh: ghDefaults(),
		command: func(ctx context.Context, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "gh", args...)
		},
	}
}

const (
	ghTimeout     = 5 * time.Second
	verifyTimeout = 10 * time.Second
)

// AccountError says why gh can't give the token of an account.
type AccountError struct {
	Host, User string
	// Stderr is what gh said, if it ran and failed.
	Stderr string
	// Logins are those gh is logged in as on Host.
	Logins []string
	Err    error
}

func (e *AccountError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "gh has no token for %s on %s", e.User, e.Host)
	switch {
	case e.Stderr != "":
		fmt.Fprintf(&b, ": %s", e.Stderr)
	case e.Err != nil:
		fmt.Fprintf(&b, ": %v", e.Err)
	}
	if len(e.Logins) > 0 {
		fmt.Fprintf(&b, "; logged in on %s: %s", e.Host, strings.Join(e.Logins, ", "))
	} else {
		fmt.Fprintf(&b, "; gh is not logged in to %s", e.Host)
	}
	return b.String()
}

func (e *AccountError) Unwrap() error { return e.Err }

// Canonical returns the login gh stores on host that is user in any letter
// case, since GitHub logins are case-insensitive but gh matches them
// exactly. It returns user as given when no stored login matches.
func (a *Accounts) Canonical(host, user string) string {
	for _, l := range a.gh.hostUsers(host) {
		if strings.EqualFold(l, user) {
			return l
		}
	}
	return user
}

// Token returns the token gh holds for user on host, by gh auth token. gh
// runs with the environment of this process and GH_HOST set to host.
func (a *Accounts) Token(ctx context.Context, host, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	user = a.Canonical(host, user)
	cmd := a.command(ctx, "auth", "token", "--hostname", host, "--user", user)
	cmd.Env = append(os.Environ(), "GH_HOST="+host)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	token := strings.TrimSpace(string(out))
	if err == nil && token != "" {
		return token, nil
	}
	if err == nil {
		err = errors.New("gh printed no token")
	}
	return "", &AccountError{
		Host: host, User: user,
		Stderr: strings.TrimSpace(stderr.String()),
		Logins: a.gh.hostUsers(host),
		Err:    err,
	}
}

// ResolveAccount returns the token of user on host, once GitHub has
// confirmed with one cheap request that it is that user's.
func (a *Accounts) ResolveAccount(ctx context.Context, host, user string) (string, error) {
	token, err := a.Token(ctx, host, user)
	if err != nil {
		return "", err
	}
	opts := append([]Option{WithHost(host), WithTokenSource(token, sourceKeyring)}, a.opts...)
	c, err := New(opts...)
	if err != nil {
		return "", fmt.Errorf("check %s on %s: %w", user, host, err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	login, _, err := c.UserLogin(ctx, Conditional{})
	if err != nil {
		return "", fmt.Errorf("check %s on %s: %w", user, host, err)
	}
	if !strings.EqualFold(login, user) {
		return "", fmt.Errorf("the token gh gives for %s on %s is for %s", user, host, login)
	}
	return token, nil
}
