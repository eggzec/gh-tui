package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestUpdatePullRequestBranch(t *testing.T) {
	tests := []struct {
		name string
		head string
		body string
	}{
		{"pinned", "abc123", `{"expected_head_sha":"abc123"}`},
		{"unpinned", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/repos/charmbracelet/bubbletea/pulls/7/update-branch" {
					t.Errorf("request = %s %s, want PUT .../pulls/7/update-branch", r.Method, r.URL.Path)
				}
				b, _ := io.ReadAll(r.Body)
				if string(b) != tt.body {
					t.Errorf("body = %q, want %q", b, tt.body)
				}
				if tt.body != "" && !json.Valid(b) {
					t.Errorf("body %q isn't JSON", b)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"message":"Updating pull request branch.","url":"https://github.com/repos/charmbracelet/bubbletea/pulls/7"}`))
			}))
			if err := c.UpdatePullRequestBranch(t.Context(), bubbletea, 7, tt.head); err != nil {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestUpdatePullRequestBranchRefused(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		header  map[string]string
		body    string
		want    string
		refused bool
		is      error
	}{
		{
			name: "conflict", status: http.StatusUnprocessableEntity,
			body: `{"message":"merge conflict between base and head"}`,
			want: "branch can't be updated: merge conflict between base and head", refused: true, is: core.ErrConflict,
		},
		{
			name: "head moved", status: http.StatusUnprocessableEntity,
			body: `{"message":"Expected head sha didn’t match current head ref."}`,
			want: "branch can't be updated: Expected head sha didn’t match current head ref.", refused: true,
		},
		{
			name: "no access", status: http.StatusForbidden,
			body: `{"message":"Resource not accessible by personal access token"}`,
			want: "branch can't be updated: Resource not accessible by personal access token", refused: true, is: core.ErrForbidden,
		},
		{name: "rate limited", status: http.StatusForbidden, header: map[string]string{"X-RateLimit-Remaining": "0"},
			body: `{"message":"API rate limit exceeded"}`},
		{name: "missing", status: http.StatusNotFound, body: `{"message":"Not Found"}`, is: core.ErrNotFound},
		{name: "outage", status: http.StatusBadGateway, body: `{}`, is: core.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			err := c.UpdatePullRequestBranch(t.Context(), bubbletea, 7, "abc123")
			if err == nil {
				t.Fatal("err = nil")
			}
			if _, refused := errors.AsType[*core.RefusedError](err); refused != tt.refused {
				t.Errorf("err = %v, refused %v; want %v", err, refused, tt.refused)
			}
			if tt.want != "" && err.Error() != tt.want {
				t.Errorf("err = %q, want %q", err, tt.want)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("err = %v, want it to match %v", err, tt.is)
			}
		})
	}
}
