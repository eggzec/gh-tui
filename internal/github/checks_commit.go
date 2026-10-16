package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The checks of any commit of a pull request, of every kind: the check runs
// of GitHub Actions and of other Apps, with the App that owns each and,
// when asked, their annotations, and the commit statuses. Unlike the read
// of the head's checks, it pages through every context of the rollup. On
// an Enterprise Server whose schema lacks a field of it, such as the App of
// a suite or the annotations, the client sends the query without the field
// and logs a warning (see unsupported.go), and the read has the rest.

const (
	// commitContextsPage is how many contexts a page of the read lists.
	commitContextsPage = 100
	// commitContextPages bounds the pages read, so a commit that lists
	// thousands of contexts can't hold the client for long. What is left
	// out is reported as truncated.
	commitContextPages = 20
	// commitNotesPage is how many annotations of a check run are read: the
	// first page of them, of the count the run reports.
	commitNotesPage = 50
)

// commitContextsText returns the query that reads one page of the contexts
// of a commit's rollup, named name, with annotations as the selection of
// the annotations of a check run.
func commitContextsText(name, annotations string) string {
	return fmt.Sprintf(`query %[1]s($owner: String!, $name: String!, $sha: GitObjectID!, $after: String) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    object(oid: $sha) {
      ... on Commit {
        oid
        statusCheckRollup {
          state
          contexts(first: %[3]d, after: $after) {
            totalCount
            pageInfo { hasNextPage endCursor }
            nodes {
              __typename
              ... on CheckRun {
                databaseId name status conclusion detailsUrl startedAt completedAt title summary
                %[2]s
                checkSuite {
                  databaseId
                  app { name slug logoUrl }
                  workflowRun { databaseId runNumber event url workflow { name } }
                }
              }
              ... on StatusContext { context state description targetUrl createdAt }
            }
          }
        }
      }
    }
  }
}`, name, annotations, commitContextsPage)
}

// commitContextsQuery reads the contexts of a commit with the count of the
// annotations of each check run, and commitContextsNotesQuery with the
// first of them.
var (
	commitContextsQuery      = commitContextsText("CommitContexts", `annotations { totalCount }`)
	commitContextsNotesQuery = commitContextsText("CommitContextsNotes", fmt.Sprintf(`annotations(first: %d) {
                  totalCount
                  nodes {
                    path annotationLevel message title rawDetails
                    location { start { line column } end { line column } }
                  }
                }`, commitNotesPage))
)

// commitContextsData is the JSON shape of a page of commitContextsQuery.
type commitContextsData struct {
	Repository *struct {
		Object *struct {
			OID               string `json:"oid"`
			StatusCheckRollup *struct {
				State    string `json:"state"`
				Contexts struct {
					TotalCount int `json:"totalCount"`
					PageInfo   struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []commitContext `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"object"`
	} `json:"repository"`
}

// commitContext is a CheckRun or a StatusContext of a page.
type commitContext struct {
	Typename string `json:"__typename"`

	// CheckRun fields.
	DatabaseID  int64      `json:"databaseId"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	DetailsURL  string     `json:"detailsUrl"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Annotations struct {
		TotalCount int                `json:"totalCount"`
		Nodes      []commitAnnotation `json:"nodes"`
	} `json:"annotations"`
	CheckSuite *struct {
		DatabaseID int64 `json:"databaseId"`
		App        *struct {
			Name    string `json:"name"`
			Slug    string `json:"slug"`
			LogoURL string `json:"logoUrl"`
		} `json:"app"`
		WorkflowRun *struct {
			DatabaseID int64  `json:"databaseId"`
			RunNumber  int    `json:"runNumber"`
			Event      string `json:"event"`
			URL        string `json:"url"`
			Workflow   struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`

	// StatusContext fields.
	Context     string    `json:"context"`
	State       string    `json:"state"`
	Description string    `json:"description"`
	TargetURL   string    `json:"targetUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

// commitAnnotation is an annotation of a check run, as GraphQL shapes it.
type commitAnnotation struct {
	Path       string `json:"path"`
	Level      string `json:"annotationLevel"`
	Message    string `json:"message"`
	Title      string `json:"title"`
	RawDetails string `json:"rawDetails"`
	Location   *struct {
		Start commitPosition `json:"start"`
		End   commitPosition `json:"end"`
	} `json:"location"`
}

// commitPosition is a line and, for an annotation of one line, a column,
// which GitHub leaves out otherwise.
type commitPosition struct {
	Line   int  `json:"line"`
	Column *int `json:"column"`
}

func (a commitAnnotation) core() core.Annotation {
	out := core.Annotation{
		Path:       a.Path,
		Level:      core.AnnotationLevel(strings.ToLower(a.Level)),
		Title:      a.Title,
		Message:    a.Message,
		RawDetails: a.RawDetails,
	}
	if l := a.Location; l != nil {
		out.StartLine, out.EndLine = l.Start.Line, l.End.Line
		if l.Start.Column != nil {
			out.StartColumn = *l.Start.Column
		}
		if l.End.Column != nil {
			out.EndColumn = *l.End.Column
		}
	}
	return out
}

func (n *commitContext) commitCheck() core.CommitCheck {
	run := core.CommitCheck{
		ID:          n.DatabaseID,
		Name:        n.Name,
		Status:      core.RunStatus(strings.ToLower(n.Status)),
		Conclusion:  core.Conclusion(strings.ToLower(n.Conclusion)),
		DetailsURL:  n.DetailsURL,
		Title:       n.Title,
		Summary:     n.Summary,
		Annotations: n.Annotations.TotalCount,
		StartedAt:   jobTime(n.StartedAt),
		CompletedAt: jobTime(n.CompletedAt),
	}
	for i := range n.Annotations.Nodes {
		run.Notes = append(run.Notes, n.Annotations.Nodes[i].core())
	}
	s := n.CheckSuite
	if s == nil {
		return run
	}
	run.SuiteID = s.DatabaseID
	if s.App != nil {
		run.App = &core.CheckApp{Name: s.App.Name, Slug: s.App.Slug, LogoURL: s.App.LogoURL}
	}
	// The check run of a job of GitHub Actions has the job's ID.
	if w := s.WorkflowRun; w != nil {
		run.JobID = n.DatabaseID
		run.RunID = w.DatabaseID
		run.Workflow = w.Workflow.Name
		run.RunNumber = w.RunNumber
		run.Event = w.Event
		run.RunURL = w.URL
	}
	return run
}

func (n *commitContext) commitStatus() core.StatusContext {
	return core.StatusContext{
		Context:     n.Context,
		State:       strings.ToLower(n.State),
		Description: n.Description,
		TargetURL:   n.TargetURL,
		CreatedAt:   n.CreatedAt,
	}
}

// CommitChecksAll returns every check run and commit status of commit sha
// of repo, reading page after page of them. With notes it also reads the
// first annotations of each check run; without, only how many it has.
func (c *Client) CommitChecksAll(ctx context.Context, repo core.RepoRef, sha string, notes bool) (core.CommitChecks, error) {
	if notes {
		return c.commitChecksAll(ctx, commitContextsNotesQuery, repo, sha, true)
	}
	return c.commitChecksAll(ctx, commitContextsQuery, repo, sha, false)
}

func (c *Client) commitChecksAll(ctx context.Context, query string, repo core.RepoRef, sha string, notes bool) (core.CommitChecks, error) {
	out := core.CommitChecks{SHA: sha, Notes: notes}
	var after string
	for range commitContextPages {
		var data commitContextsData
		vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "sha": sha}
		if after != "" {
			vars["after"] = after
		}
		if err := c.Query(ctx, query, vars, &data); err != nil {
			return core.CommitChecks{}, fmt.Errorf("checks of %s@%s: %w", repo, sha, err)
		}
		if data.Repository == nil || data.Repository.Object == nil || data.Repository.Object.OID == "" {
			return core.CommitChecks{}, fmt.Errorf("checks of %s@%s: %w", repo, sha, core.ErrNotFound)
		}
		obj := data.Repository.Object
		out.SHA = obj.OID
		r := obj.StatusCheckRollup
		if r == nil {
			return out, nil
		}
		out.State = checksState(r.State)
		out.Total = r.Contexts.TotalCount
		for i := range r.Contexts.Nodes {
			n := &r.Contexts.Nodes[i]
			switch n.Typename {
			case "CheckRun":
				out.Runs = append(out.Runs, n.commitCheck())
			case "StatusContext":
				out.Statuses = append(out.Statuses, n.commitStatus())
			}
		}
		p := r.Contexts.PageInfo
		if !p.HasNextPage || p.EndCursor == "" {
			break
		}
		after = p.EndCursor
	}
	out.Truncated = out.Total > len(out.Runs)+len(out.Statuses)
	return out, nil
}
