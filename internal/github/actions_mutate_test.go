package github

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestRunActions(t *testing.T) {
	tests := []struct {
		name   string
		do     func(ctx context.Context, c *Client) error
		path   string
		status int
	}{
		{"rerun", func(ctx context.Context, c *Client) error { return c.RerunRun(ctx, bubbletea, 42) }, "/repos/charmbracelet/bubbletea/actions/runs/42/rerun", http.StatusCreated},
		{"rerun failed", func(ctx context.Context, c *Client) error { return c.RerunFailedJobs(ctx, bubbletea, 42) }, "/repos/charmbracelet/bubbletea/actions/runs/42/rerun-failed-jobs", http.StatusCreated},
		{"rerun job", func(ctx context.Context, c *Client) error { return c.RerunJob(ctx, bubbletea, 7) }, "/repos/charmbracelet/bubbletea/actions/jobs/7/rerun", http.StatusCreated},
		{"cancel", func(ctx context.Context, c *Client) error { return c.CancelRun(ctx, bubbletea, 42) }, "/repos/charmbracelet/bubbletea/actions/runs/42/cancel", http.StatusAccepted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tt.path {
					t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, tt.path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("{}"))
			}))
			if err := tt.do(t.Context(), c); err != nil {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestRunActionsRefused(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		do      func(ctx context.Context, c *Client) error
		want    string
		refused bool
		is      error
	}{
		{
			name: "too old", status: http.StatusForbidden,
			body: `{"message":"Unable to retry this workflow run because it was created over a month ago"}`,
			do:   func(ctx context.Context, c *Client) error { return c.RerunRun(ctx, bubbletea, 42) },
			want: "run can't be re-run: Unable to retry this workflow run because it was created over a month ago", refused: true,
		},
		{
			name: "completed", status: http.StatusConflict,
			body: `{"message":"Cannot cancel a workflow run that is completed."}`,
			do:   func(ctx context.Context, c *Client) error { return c.CancelRun(ctx, bubbletea, 42) },
			want: "run can't be cancelled: Cannot cancel a workflow run that is completed.", refused: true, is: core.ErrConflict,
		},
		{
			name: "no reason", status: http.StatusForbidden, body: `{}`,
			do:   func(ctx context.Context, c *Client) error { return c.RerunJob(ctx, bubbletea, 7) },
			want: "job can't be re-run", refused: true,
		},
		{
			name: "rate limited", status: http.StatusForbidden, body: `{"message":"API rate limit exceeded"}`,
			headers: map[string]string{"X-RateLimit-Limit": "5000", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1790325043"},
			do:      func(ctx context.Context, c *Client) error { return c.RerunFailedJobs(ctx, bubbletea, 42) },
			is:      core.ErrRateLimited,
		},
		{
			name: "missing", status: http.StatusNotFound, body: `{"message":"Not Found"}`,
			do: func(ctx context.Context, c *Client) error { return c.RerunRun(ctx, bubbletea, 42) },
			is: core.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			err := tt.do(t.Context(), c)
			_, refused := errors.AsType[*core.RefusedError](err)
			if refused != tt.refused {
				t.Errorf("err = %v, refused %v; want %v", err, refused, tt.refused)
			}
			if tt.want != "" && (err == nil || err.Error() != tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("err = %v, want it to match %v", err, tt.is)
			}
		})
	}
}
