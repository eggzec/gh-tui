package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestUnreachable(t *testing.T) {
	dial := &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("connection refused")}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"network", fmt.Errorf("list: %w", offline(t.Context(), dial)), true},
		{"untagged network", dial, false},
		{"server error", &Error{StatusCode: 502}, true},
		{"unavailable", &Error{StatusCode: 503}, true},
		{"not found", &Error{StatusCode: 404, err: core.ErrNotFound}, false},
		{"unauthorized", &Error{StatusCode: 401, err: core.ErrUnauthorized}, false},
		{"forbidden", &Error{StatusCode: 403}, false},
		{"gone", &Error{StatusCode: 410}, false},
		{"rate limited", &Error{StatusCode: 429, err: &core.RateLimitError{}}, false},
		{"other", errors.New("decode response: unexpected EOF"), false},
	}
	for _, tt := range tests {
		if got := Unreachable(t.Context(), tt.err); got != tt.want {
			t.Errorf("Unreachable(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestUnreachableCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Get", URL: "https://api.github.com/", Err: context.Canceled})
	if Unreachable(ctx, err) {
		t.Error("Unreachable after cancel = true, want false")
	}
}

func TestRefused(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not found", fmt.Errorf("get: %w", &Error{StatusCode: 404, err: core.ErrNotFound}), true},
		{"graphql not found", &GraphQLError{causes: []error{core.ErrNotFound}}, true},
		{"unauthorized", &Error{StatusCode: 401, err: core.ErrUnauthorized}, true},
		{"forbidden", &Error{StatusCode: 403}, true},
		{"missing scope", &Error{StatusCode: 403, err: core.ErrUnauthorized}, true},
		{"graphql forbidden", &GraphQLError{causes: []error{core.ErrForbidden}}, true},
		{"graphql scopes", &GraphQLError{causes: []error{core.ErrUnauthorized}}, true},
		{"bare not found", &Error{StatusCode: 404}, true},
		{"gone", &Error{StatusCode: 410}, true},
		{"rate limited", &Error{StatusCode: 403, err: &core.RateLimitError{}}, false},
		{"too many requests", &Error{StatusCode: 429, err: &core.RateLimitError{}}, false},
		{"server error", &Error{StatusCode: 502}, false},
		{"network", &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("refused")}, false},
	}
	for _, tt := range tests {
		if got := Refused(tt.err); got != tt.want {
			t.Errorf("Refused(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// ssoMessage is what GitHub says to a token that an organization with SAML
// SSO hasn't authorized.
const ssoMessage = "Resource protected by organization SAML enforcement. You must grant your Personal Access token access to this organization."

// reply answers with status, headers h and a JSON error body of msg.
func reply(status int, h map[string]string, msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range h {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if msg != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
		}
	}
}

// graphqlReply answers a GraphQL query with one error of typ.
func graphqlReply(typ, msg string, h map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range h {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":   nil,
			"errors": []map[string]any{{"type": typ, "message": msg, "path": []string{"repository"}}},
		})
	}
}

// TestErrorKinds sends requests to recorded answers of GitHub and checks
// what core.Explain makes of the errors, and what Unreachable and Refused
// say of them.
func TestErrorKinds(t *testing.T) {
	reset := time.Unix(1790000000, 0)
	now := time.Date(2026, 9, 27, 14, 0, 0, 0, time.UTC)
	rateHeaders := map[string]string{
		"X-RateLimit-Limit":     "5000",
		"X-RateLimit-Remaining": "0",
		"X-RateLimit-Reset":     strconv.FormatInt(reset.Unix(), 10),
		"X-RateLimit-Resource":  "core",
	}
	tests := []struct {
		name        string
		h           http.Handler
		graphql     bool
		kind        core.ProblemKind
		reason      string
		reset       time.Time // zero: any time, if kind is RateLimited
		unreachable bool
		refused     bool
	}{
		{
			name:    "403 SSO",
			h:       reply(403, map[string]string{"X-GitHub-SSO": "required; url=https://github.com/orgs/eggzec/sso?authorization_request=x"}, ssoMessage),
			kind:    core.Forbidden,
			reason:  ssoMessage,
			refused: true,
		},
		{
			name:    "403 no access",
			h:       reply(403, map[string]string{"X-OAuth-Scopes": "repo, read:org", "X-Accepted-OAuth-Scopes": "public_repo"}, "Must have admin rights to Repository."),
			kind:    core.Forbidden,
			reason:  "Must have admin rights to Repository.",
			refused: true,
		},
		{
			name:    "403 scopes",
			h:       reply(403, map[string]string{"X-OAuth-Scopes": "read:org, gist", "X-Accepted-OAuth-Scopes": "repo, workflow"}, "Resource not accessible by personal access token"),
			kind:    core.Auth,
			reason:  "Resource not accessible by personal access token",
			refused: true,
		},
		{
			name:    "403 scopes of another kind of token",
			h:       reply(403, map[string]string{"X-Accepted-OAuth-Scopes": "repo"}, "Resource not accessible by integration"),
			kind:    core.Forbidden,
			reason:  "Resource not accessible by integration",
			refused: true,
		},
		{
			name:   "403 rate limited",
			h:      reply(403, rateHeaders, "API rate limit exceeded for user ID 1."),
			kind:   core.RateLimited,
			reason: "API rate limit exceeded for user ID 1.",
			reset:  reset,
		},
		{
			name:   "403 secondary rate limit without a reset",
			h:      reply(403, nil, "You have exceeded a secondary rate limit. Please wait a few minutes before you try again."),
			kind:   core.RateLimited,
			reason: "You have exceeded a secondary rate limit. Please wait a few minutes before you try again.",
			reset:  now.Add(time.Minute),
		},
		{
			name: "429 with Retry-After",
			h:    reply(429, map[string]string{"Retry-After": "30"}, ""),
			kind: core.RateLimited,
		},
		{
			name:    "401",
			h:       reply(401, nil, "Bad credentials"),
			kind:    core.Auth,
			reason:  "Bad credentials",
			refused: true,
		},
		{
			name:    "404",
			h:       reply(404, nil, "Not Found"),
			kind:    core.NotFound,
			reason:  "Not Found",
			refused: true,
		},
		{
			name:    "410",
			h:       reply(410, nil, "Issues are disabled for this repo"),
			kind:    core.NotFound,
			reason:  "Issues are disabled for this repo",
			refused: true,
		},
		{
			name:   "422",
			h:      reply(422, nil, "Pull Request is not mergeable"),
			kind:   core.Rejected,
			reason: "Pull Request is not mergeable",
		},
		{
			name:        "502",
			h:           reply(502, nil, "Server Error"),
			kind:        core.Unavailable,
			reason:      "Server Error",
			unreachable: true,
		},
		{
			name: "503 without a body",
			h: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "<html>Unicorn!</html>")
			}),
			kind:        core.Unavailable,
			unreachable: true,
		},
		{
			name:    "graphql forbidden",
			h:       graphqlReply("FORBIDDEN", ssoMessage, nil),
			graphql: true,
			kind:    core.Forbidden,
			reason:  ssoMessage,
			refused: true,
		},
		{
			name:    "graphql insufficient scopes",
			h:       graphqlReply("INSUFFICIENT_SCOPES", "Your token has not been granted the required scopes to execute this query. The 'login' field requires one of the following scopes: ['read:org'], but your token has only been granted the: ['repo'] scopes.", nil),
			graphql: true,
			kind:    core.Auth,
			reason:  "Your token has not been granted the required scopes to execute this query. The 'login' field requires one of the following scopes: ['read:org'], but your token has only been granted the: ['repo'] scopes.",
			refused: true,
		},
		{
			name:    "graphql rate limited",
			h:       graphqlReply("RATE_LIMITED", "API rate limit exceeded for user ID 1.", rateHeaders),
			graphql: true,
			kind:    core.RateLimited,
			reason:  "API rate limit exceeded for user ID 1.",
			reset:   reset,
		},
		{
			name:    "graphql not found",
			h:       graphqlReply("NOT_FOUND", "Could not resolve to a Repository with the name 'eggzec/nope'.", nil),
			graphql: true,
			kind:    core.NotFound,
			reason:  "Could not resolve to a Repository with the name 'eggzec/nope'.",
			refused: true,
		},
		{
			name:        "graphql over a 502",
			h:           reply(502, nil, ""),
			graphql:     true,
			kind:        core.Unavailable,
			unreachable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.h)
			c.now = func() time.Time { return now }
			var err error
			if tt.graphql {
				err = c.Query(t.Context(), "query { viewer { login } }", nil, nil)
			} else {
				_, err = c.Get(t.Context(), "repos/eggzec/gh-tui", Conditional{}, nil)
			}
			err = fmt.Errorf("load it: %w", err)
			checkKind(t, err, tt.kind, tt.reason, tt.reset)
			if got := Unreachable(t.Context(), err); got != tt.unreachable {
				t.Errorf("Unreachable = %v, want %v", got, tt.unreachable)
			}
			if got := Refused(err); got != tt.refused {
				t.Errorf("Refused = %v, want %v", got, tt.refused)
			}
		})
	}
}

// checkKind checks what core.Explain makes of err.
func checkKind(t *testing.T, err error, kind core.ProblemKind, reason string, reset time.Time) {
	t.Helper()
	p := core.Explain("load it", err)
	if p.Kind != kind || p.Reason != reason {
		t.Errorf("Explain(%v) = %v %q, want %v %q", err, p.Kind, p.Reason, kind, reason)
	}
	switch {
	case kind != core.RateLimited:
	case reset.IsZero() && p.Reset.IsZero():
		t.Errorf("Explain(%v) has no reset", err)
	case !reset.IsZero() && !p.Reset.Equal(reset):
		t.Errorf("Explain(%v).Reset = %v, want %v", err, p.Reset, reset)
	}
}

func TestErrorKindsOffline(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	dial, err := New(WithBaseURL(downURL), WithToken("test-token"))
	if err != nil {
		t.Fatal(err)
	}
	// A client that doesn't trust the server's certificate.
	tlsSrv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(tlsSrv.Close)
	untrusted, err := New(WithBaseURL(tlsSrv.URL), WithToken("test-token"))
	if err != nil {
		t.Fatal(err)
	}
	// A server that never answers, and a client that gives up on it. The
	// timeout's error matches context.DeadlineExceeded, but the request's
	// context is live, so it is an outage.
	hang := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(hang.Close)
	impatient, err := New(WithBaseURL(hang.URL), WithToken("test-token"),
		WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*Client{"dial": dial, "tls": untrusted, "timeout": impatient} {
		t.Run(name, func(t *testing.T) {
			_, err := c.Get(t.Context(), "user", Conditional{}, nil)
			if !errors.Is(err, core.ErrOffline) {
				t.Fatalf("Get = %v, want core.ErrOffline", err)
			}
			if _, ok := errors.AsType[*url.Error](err); !ok {
				t.Errorf("Get = %v, want the *url.Error kept", err)
			}
			checkKind(t, err, core.Offline, "", time.Time{})
			if !Unreachable(t.Context(), err) || Refused(err) {
				t.Errorf("Unreachable, Refused = %v, %v, want true, false", Unreachable(t.Context(), err), Refused(err))
			}
		})
	}
}

func TestErrorKindsCanceled(t *testing.T) {
	arrived := make(chan struct{})
	c := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-arrived
		cancel()
	}()
	_, err := c.Get(ctx, "slow", Conditional{}, nil)
	if errors.Is(err, core.ErrOffline) {
		t.Errorf("Get = %v, want no core.ErrOffline after a cancel", err)
	}
	checkKind(t, err, core.Canceled, "", time.Time{})
	if Unreachable(ctx, err) || Refused(err) {
		t.Errorf("Unreachable, Refused = %v, %v, want false, false", Unreachable(ctx, err), Refused(err))
	}
}

func TestReason(t *testing.T) {
	e := &Error{StatusCode: 422, Message: "Validation Failed\n\x1b[31mbad\x1b[0m\tfield\x07 (x)"}
	if got, want := e.Reason(), "Validation Failed bad field (x)"; got != want {
		t.Errorf("Reason() = %q, want %q", got, want)
	}
	if got := (&Error{StatusCode: 502}).Reason(); got != "" {
		t.Errorf("Reason() without a message = %q, want empty", got)
	}
	g := &GraphQLError{Errors: []GraphQLErrorItem{{Message: "one\r\ntwo"}, {Message: ""}, {Message: "three"}}}
	if got, want := g.Reason(), "one two; three"; got != want {
		t.Errorf("GraphQLError.Reason() = %q, want %q", got, want)
	}
}

func TestMissingScope(t *testing.T) {
	tests := []struct {
		granted  *string
		accepted string
		want     bool
	}{
		{nil, "repo", false},
		{new(""), "", false},
		{new(""), "repo", true},
		{new("repo"), "repo", false},
		{new("repo"), "public_repo", false},
		{new("repo"), "repo:status", false},
		{new("public_repo"), "repo", true},
		{new("admin:org"), "read:org", false},
		{new("write:org"), "read:org", false},
		{new("read:org"), "write:org", true},
		{new("user"), "user:email", false},
		{new("repo"), "repo_deployment", false},
		{new("repo"), "admin:repo_hook", false},
		{new("repo"), "read:repo_hook", false},
		{new("public_repo"), "admin:repo_hook", true},
		{new("gist, read:org"), "repo, workflow", true},
		{new("gist, workflow"), "repo, workflow", false},
	}
	for _, tt := range tests {
		h := http.Header{}
		if tt.granted != nil {
			h.Set("X-OAuth-Scopes", *tt.granted)
		}
		h.Set("X-Accepted-OAuth-Scopes", tt.accepted)
		if got := missingScope(h); got != tt.want {
			t.Errorf("missingScope(%v, %q) = %v, want %v", tt.granted, tt.accepted, got, tt.want)
		}
	}
}

// orgRestricted is what GitHub says of a search result in an organization
// that restricts OAuth apps, as the gh CLI sees in its own searches.
const orgRestricted = "Although you appear to have the correct authorization credentials, the `acme` organization has enabled OAuth App access restrictions, meaning that data access to third-parties is limited."

func TestGraphQLPartialRefusal(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		refused bool
	}{
		{
			name:    "one search node",
			body:    `{"data":{"search":{"nodes":[{"id":"a"},null]}},"errors":[{"type":"FORBIDDEN","path":["search","nodes",1],"message":"` + orgRestricted + `"}]}`,
			refused: false,
		},
		{
			name:    "scopes of one node",
			body:    `{"data":{"search":{"nodes":[{"id":"a"},null]}},"errors":[{"type":"INSUFFICIENT_SCOPES","path":["search","nodes",1],"message":"missing read:org"}]}`,
			refused: false,
		},
		{
			name:    "the root field",
			body:    `{"data":{"repository":null},"errors":[{"type":"FORBIDDEN","path":["repository"],"message":"` + ssoMessage + `"}]}`,
			refused: true,
		},
		{
			name:    "no data",
			body:    `{"data":null,"errors":[{"type":"FORBIDDEN","path":["search","nodes",1],"message":"` + orgRestricted + `"}]}`,
			refused: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.body)
			}))
			var data struct {
				Search struct {
					Nodes []*struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"search"`
			}
			err := c.Query(t.Context(), "query { search { nodes { id } } }", nil, &data)
			if _, ok := errors.AsType[*GraphQLError](err); !ok {
				t.Fatalf("Query = %v, want a *GraphQLError", err)
			}
			if got := Refused(err); got != tt.refused {
				t.Errorf("Refused(%v) = %v, want %v", err, got, tt.refused)
			}
			if Unreachable(t.Context(), err) {
				t.Errorf("Unreachable(%v) = true, want false", err)
			}
			if !tt.refused && (len(data.Search.Nodes) != 2 || data.Search.Nodes[0].ID != "a") {
				t.Errorf("partial data = %+v, want the node that was allowed", data.Search.Nodes)
			}
		})
	}
}

// TestSharedFetchOffline checks that a read shared by several callers still
// ends as an outage for those who wait when one of them gives up: the
// request's context outlives that caller, so the client tags the failure.
func TestSharedFetchOffline(t *testing.T) {
	arrived, drop := make(chan struct{}), make(chan struct{})
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(arrived)
		<-drop
		conn, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	mem := cache.New[int]()
	fn := func(ctx context.Context, _ cache.Entry[int], _ bool) (cache.Entry[int], error) {
		if _, err := c.Get(ctx, "user", Conditional{}, nil); err != nil {
			return cache.Entry[int]{}, err
		}
		return cache.Entry[int]{Value: 1}, nil
	}

	stays := make(chan error, 1)
	go func() {
		_, err := mem.Fetch(t.Context(), "k", fn)
		stays <- err
	}()
	<-arrived
	ctx, cancel := context.WithCancel(t.Context())
	leaves := make(chan error, 1)
	go func() {
		_, err := mem.Fetch(ctx, "k", fn)
		leaves <- err
	}()
	cancel()
	if err := <-leaves; !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch of the caller that left = %v, want context.Canceled", err)
	}
	close(drop)
	err := <-stays
	if !errors.Is(err, core.ErrOffline) || !Unreachable(t.Context(), err) {
		t.Errorf("Fetch of the caller that stayed = %v, want an outage", err)
	}
	checkKind(t, err, core.Offline, "", time.Time{})
}
