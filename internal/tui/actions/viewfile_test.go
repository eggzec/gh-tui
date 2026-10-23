package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestViewWorkflowOpensTheFileAtTheCommit(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.workflows = []core.Workflow{{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"}}
	m, h := newModal(t, f, wideW, wideH)
	h.keys("v")
	var got []ui.OpenFileMsg
	for _, msg := range h.take() {
		if o, ok := msg.(ui.OpenFileMsg); ok {
			got = append(got, o)
		}
	}
	if len(got) != 1 {
		t.Fatalf("v sent %d file messages, want 1", len(got))
	}
	o := got[0]
	if o.Path != ".github/workflows/ci.yml" || o.Ref != "f00dcafe" || o.Repo != repo || o.Return != ui.Modal(m) {
		t.Errorf("OpenFileMsg = %+v", o)
	}
}

func TestViewWorkflowWithoutAFileToasts(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.workflows = []core.Workflow{{ID: 1, Name: "CI", Path: "dynamic/dependabot"}}
	_, h := newModal(t, f, wideW, wideH)
	h.keys("v")
	var notified bool
	for _, msg := range h.take() {
		switch msg.(type) {
		case ui.OpenFileMsg:
			t.Fatal("a dynamic workflow has no file to open")
		case ui.NotifyMsg:
			notified = true
		}
	}
	if !notified {
		t.Error("no toast for a run without a workflow file")
	}
}
