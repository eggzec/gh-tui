package github

import (
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestResolveSubjectURL(t *testing.T) {
	repo := core.RepoRef{Owner: "o", Name: "r"}
	at := time.Date(2026, 9, 24, 16, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		n       core.Notification
		serve   func(t *testing.T, w http.ResponseWriter, r *http.Request)
		want    string
		wantErr bool
	}{
		{
			name: "release",
			n:    core.Notification{Repo: repo, Subject: core.Subject{Type: core.SubjectRelease, ReleaseID: 991}},
			serve: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Helper()
				if r.URL.Path != "/repos/o/r/releases/991" {
					t.Errorf("path = %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"id":991,"html_url":"https://github.com/o/r/releases/tag/v1"}`))
			},
			want: "https://github.com/o/r/releases/tag/v1",
		},
		{
			name: "check suite by ID",
			n:    core.Notification{Repo: repo, Subject: core.Subject{Type: core.SubjectCheckSuite, CheckSuiteID: 55}},
			serve: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Helper()
				if r.URL.Path != "/repos/o/r/actions/runs" || r.URL.Query().Get("check_suite_id") != "55" {
					t.Errorf("request = %s", r.URL)
				}
				_, _ = w.Write([]byte(`{"workflow_runs":[{"id":7,"html_url":"https://github.com/o/r/actions/runs/7"}]}`))
			},
			want: "https://github.com/o/r/actions/runs/7",
		},
		{
			name: "check suite by title",
			n: core.Notification{Repo: repo, UpdatedAt: at, Subject: core.Subject{
				Type: core.SubjectCheckSuite, Title: "ci workflow run failed for feat/x branch",
			}},
			serve: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Helper()
				q := r.URL.Query()
				if q.Get("branch") != "feat/x" || q.Get("status") != "failure" {
					t.Errorf("query = %v", q)
				}
				_, _ = w.Write([]byte(`{"workflow_runs":[
					{"id":9,"name":"lint","updated_at":"2026-09-24T16:00:00Z","html_url":"https://github.com/o/r/actions/runs/9"},
					{"id":8,"name":"CI","updated_at":"2026-09-23T10:00:00Z","html_url":"https://github.com/o/r/actions/runs/8"},
					{"id":7,"name":"CI","updated_at":"2026-09-24T15:59:00Z","html_url":"https://github.com/o/r/actions/runs/7"}]}`))
			},
			want: "https://github.com/o/r/actions/runs/7",
		},
		{
			name: "check suite with no run",
			n:    core.Notification{Repo: repo, Subject: core.Subject{Type: core.SubjectCheckSuite, CheckSuiteID: 55}},
			serve: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"workflow_runs":[]}`))
			},
			wantErr: true,
		},
		{
			name: "failure",
			n:    core.Notification{Repo: repo, Subject: core.Subject{Type: core.SubjectRelease, ReleaseID: 991}},
			serve: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: true,
		},
		{
			name:    "other subject",
			n:       core.Notification{Repo: repo, Subject: core.Subject{Type: core.SubjectIssue, Number: 3}},
			serve:   func(*testing.T, http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.serve(t, w, r)
			}))
			got, err := c.ResolveSubjectURL(t.Context(), tt.n)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveSubjectURL error = %v, want error %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ResolveSubjectURL = %q, want %q", got, tt.want)
			}
		})
	}
}
