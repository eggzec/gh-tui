package files

import (
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestAddFindingsKeepsTheNotesOfLines(t *testing.T) {
	t.Parallel()
	out := AddFindings(nil, "lint", []core.Annotation{
		{Path: "./a.go", EndLine: 4, Level: core.AnnotationNotice, Message: "  first\nsecond  "},
		{Path: "a.go", EndLine: 0, Level: core.AnnotationFailure, Message: "whole file"},
		{Path: "b.go", EndLine: 2, Level: core.AnnotationWarning, Message: "w"},
		{Path: "", EndLine: 2, Level: core.AnnotationFailure, Message: "no file"},
	})
	a := out["a.go"]
	if a.Count != 2 || a.Level != core.AnnotationFailure {
		t.Errorf("a.go = %d at %s, want 2 at the failure", a.Count, a.Level)
	}
	want := []Note{{Line: 4, Level: core.AnnotationNotice, App: "lint", Message: "first"}}
	if !reflect.DeepEqual(a.Notes, want) {
		t.Errorf("notes of a.go = %v, want %v: the first line only, none for the whole file", a.Notes, want)
	}
	if len(out) != 2 {
		t.Errorf("%d files, want a.go and b.go", len(out))
	}
	// Runs add to the same map.
	out = AddFindings(out, "Actions", []core.Annotation{{Path: "a.go", EndLine: 4, Level: core.AnnotationFailure, Message: "x"}})
	if got := out["a.go"]; got.Count != 3 || len(got.Notes) != 2 {
		t.Errorf("a.go = %d with %d notes after a second run, want 3 with 2", got.Count, len(got.Notes))
	}
}

func TestFindingByLineOrdersWorstLevelThenApp(t *testing.T) {
	t.Parallel()
	f := Finding{Notes: []Note{
		{Line: 3, Level: core.AnnotationNotice, App: "a"},
		{Line: 3, Level: core.AnnotationFailure, App: "z"},
		{Line: 3, Level: core.AnnotationFailure, App: "b"},
		{Line: 3, Level: core.AnnotationWarning, App: "a"},
		{Line: 5, Level: core.AnnotationNotice, App: "a"},
	}}
	by := f.ByLine()
	got := make([]string, 0, len(by[3]))
	for _, n := range by[3] {
		got = append(got, string(n.Level)+":"+n.App)
	}
	want := []string{"failure:b", "failure:z", "warning:a", "notice:a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("line 3 = %v, want %v", got, want)
	}
	if len(by[5]) != 1 || len(by) != 2 {
		t.Errorf("lines = %v, want 3 and 5", by)
	}
	if (Finding{Count: 2}).ByLine() != nil {
		t.Error("a finding with no notes lists lines")
	}
}
