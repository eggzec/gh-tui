package github

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// The logins are what the body must carry as given, with the characters that
// a path would have to escape.
var assigneeLogins = []string{"monalisa", "dependabot[bot]"}

const assigneesBodyWant = `{"assignees":["monalisa","dependabot[bot]"]}`

func checkAssigned(t *testing.T, got core.Issue) {
	t.Helper()
	logins := make([]string, 0, len(got.Assignees))
	for _, u := range got.Assignees {
		logins = append(logins, u.Login)
	}
	if want := []string{"monalisa", "dependabot[bot]"}; !slices.Equal(logins, want) {
		t.Errorf("assignees = %v, want %v", logins, want)
	}
	if !slices.Equal(got.Labels, wantIssueLabels[:1]) {
		t.Errorf("labels = %+v, want %+v", got.Labels, wantIssueLabels[:1])
	}
}

func TestAddIssueAssignees(t *testing.T) {
	c := serveIssueMutation(t, "issues_assignees.json", http.StatusCreated, func(r *http.Request, body string) {
		checkIssueRequest(t, r, http.MethodPost, "/repos/octo-org/hello/issues/42/assignees", nil)
		checkIssueBody(t, body, assigneesBodyWant)
	})
	got, err := c.AddIssueAssignees(t.Context(), issueRepo, 42, assigneeLogins)
	if err != nil {
		t.Fatalf("AddIssueAssignees: %v", err)
	}
	checkAssigned(t, got)
}

func TestRemoveIssueAssignees(t *testing.T) {
	c := serveIssueMutation(t, "issues_assignees.json", http.StatusOK, func(r *http.Request, body string) {
		checkIssueRequest(t, r, http.MethodDelete, "/repos/octo-org/hello/issues/42/assignees", nil)
		checkIssueBody(t, body, assigneesBodyWant)
	})
	got, err := c.RemoveIssueAssignees(t.Context(), issueRepo, 42, assigneeLogins)
	if err != nil {
		t.Fatalf("RemoveIssueAssignees: %v", err)
	}
	checkAssigned(t, got)
}

func TestIssueAssigneesErrors(t *testing.T) {
	calls := map[string]func(c *Client, ctx context.Context) error{
		"AddIssueAssignees": func(c *Client, ctx context.Context) error {
			_, err := c.AddIssueAssignees(ctx, issueRepo, 42, assigneeLogins)
			return err
		},
		"RemoveIssueAssignees": func(c *Client, ctx context.Context) error {
			_, err := c.RemoveIssueAssignees(ctx, issueRepo, 42, assigneeLogins)
			return err
		},
	}
	statuses := map[int]error{
		http.StatusNotFound:            core.ErrNotFound,
		http.StatusUnprocessableEntity: core.ErrConflict,
	}
	for name, call := range calls {
		for status, want := range statuses {
			t.Run(name+"/"+http.StatusText(status), func(t *testing.T) {
				c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
				}))
				if err := call(c, t.Context()); !errors.Is(err, want) {
					t.Errorf("error = %v, want %v", err, want)
				}
			})
		}
	}
}
