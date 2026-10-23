package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestFilterOfSendsOneValueOnly(t *testing.T) {
	t.Parallel()
	m := &Modal{}
	m.workflows.items = []core.Workflow{{ID: 1, Name: "CI"}, {ID: 2, Name: "Deploy"}}
	v := map[string]filterform.Value{
		ui.SearchField.Key: filterform.TextValue(""),
		fieldWorkflow:      filterform.ListValue("CI", "Deploy"),
		fieldStatus:        filterform.ListValue("failure"),
		fieldBranch:        filterform.ListValue("main", "dev"),
		fieldActor:         filterform.ListValue("@me", "@bob"),
	}
	f := m.filterOf(v, nil)
	if f.Status != "failure" || f.Statuses != "" {
		t.Fatalf("one status: Status=%q Statuses=%q", f.Status, f.Statuses)
	}
	if f.Branch != "" || f.Branches != core.AnyOfValues("main", "dev") {
		t.Fatalf("two branches: Branch=%q Branches=%q", f.Branch, f.Branches)
	}
	if f.WorkflowID != 0 || len(f.Workflows.List()) != 2 {
		t.Fatalf("two workflows: id=%d list=%q", f.WorkflowID, f.Workflows)
	}
	if got := f.Actors.List(); len(got) != 2 || got[0] != "@me" || got[1] != "bob" {
		t.Fatalf("actors = %q", got)
	}
	if r := f.Remote(); r.Multi() || r.Status != "failure" {
		t.Fatalf("Remote = %+v", r)
	}
}

func TestQueryOfRepeatsQualifiers(t *testing.T) {
	t.Parallel()
	wfs := []core.Workflow{{ID: 1, Name: "CI"}, {ID: 2, Name: "Deploy"}}
	f := core.RunFilter{
		Workflows: core.AnyOfValues("1", "2"),
		Statuses:  core.AnyOfValues("failure", "success"),
		Event:     "push",
		Words:     "fix",
	}
	got := queryOf(f, wfs)
	want := "workflow:CI workflow:Deploy event:push status:failure status:success fix"
	if got != want {
		t.Fatalf("queryOf = %q, want %q", got, want)
	}
}

func TestKeepsAnyOf(t *testing.T) {
	t.Parallel()
	f := core.RunFilter{Statuses: core.AnyOfValues("failure", "queued"), Branches: core.AnyOfValues("main", "dev")}
	tests := []struct {
		run  core.Run
		want bool
	}{
		{core.Run{Branch: "main", Conclusion: core.ConclusionFailure}, true},
		{core.Run{Branch: "dev", Status: core.RunQueued}, true},
		{core.Run{Branch: "other", Status: core.RunQueued}, false},
		{core.Run{Branch: "main", Conclusion: core.ConclusionSuccess}, false},
	}
	for _, tt := range tests {
		if got := f.Keeps(tt.run); got != tt.want {
			t.Errorf("Keeps(%+v) = %v, want %v", tt.run, got, tt.want)
		}
	}
}
