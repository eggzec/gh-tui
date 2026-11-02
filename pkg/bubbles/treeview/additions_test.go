package treeview

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestAtAndByID(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	n, ok := m.At(0)
	if !ok || n.ID != "a" {
		t.Fatalf("At(0) = %v, %v", n.ID, ok)
	}
	if !n.Folded {
		t.Error("a closed fold: Folded = false")
	}
	if _, ok := m.At(5); ok {
		t.Error("At(5) found a row")
	}
	if _, ok := m.At(-1); ok {
		t.Error("At(-1) found a row")
	}
	m = pressRun(t, m, "l")
	if n, _ := m.ByID("a"); n.Folded {
		t.Error("an open fold: Folded = true")
	}
	if n, ok := m.ByID("a1"); !ok || n.ID != "a1" {
		t.Errorf("ByID(a1) = %v, %v", n.ID, ok)
	}
	if _, ok := m.ByID("nope"); ok {
		t.Error("ByID(nope) found a row")
	}
	if sel, _ := m.Selected(); sel.ID != "a" || sel.Folded {
		t.Errorf("Selected = %q, folded %v", sel.ID, sel.Folded)
	}
}

func TestSetIconsSeesFolded(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m.SetIcons(func(n Node) string {
		if n.Folded {
			return "+"
		}
		return "-"
	})
	if got := lines(m)[0]; !contains(got, "+ a") {
		t.Errorf("closed row = %q", got)
	}
	m = pressRun(t, m, "l")
	if got := lines(m)[0]; !contains(got, "- a") {
		t.Errorf("open row = %q", got)
	}
}

func TestRename(t *testing.T) {
	m := New(Rows([]Node{leaf("a", 0), leaf("b", 0)}), WithSize(20, 4))
	if !m.Rename("b", "zed") {
		t.Fatal("Rename(b) = false")
	}
	if m.Rename("nope", "x") {
		t.Error("Rename(nope) = true")
	}
	if got := lines(m)[1]; !contains(got, "zed") {
		t.Errorf("row = %q", got)
	}
	m.SetFilter("zed")
	if got := visible(m); !slices.Equal(got, []string{"b"}) {
		t.Errorf("filtered rows = %v", got)
	}
}

func TestExpandOpensAncestorsAndLoads(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	m = run(m, m.Expand("a"))
	if !slices.Contains(visible(m), "a1") {
		t.Errorf("rows = %v", visible(m))
	}
	if m.Expand("nope") != nil {
		t.Error("Expand(nope) returned a cmd")
	}
	n := w.count("a")
	m = run(m, m.Expand("a"))
	if w.count("a") != n {
		t.Errorf("Expand loaded a again: %d", w.count("a"))
	}
}

func TestLoadSkipsLoadedAndRetriesFailed(t *testing.T) {
	w := &world{fail: map[string]error{"a": errors.New("boom")}}
	m := lazyTree(w)
	m = pressRun(t, m, "l")
	if m.Err("a") == nil {
		t.Fatal("a did not fail")
	}
	delete(w.fail, "a")
	m = run(m, m.Load("a"))
	if m.Err("a") != nil || w.count("a") != 2 {
		t.Errorf("err %v, loads %d", m.Err("a"), w.count("a"))
	}
	if m.Load("a") != nil {
		t.Error("Load of a loaded row returned a cmd")
	}
	if m.Load("nope") != nil {
		t.Error("Load of a missing row returned a cmd")
	}
}

func TestRetryReloadsOnlyFailed(t *testing.T) {
	w := &world{fail: map[string]error{"a": errors.New("boom")}}
	m := lazyTree(w, WithMaxLoads(1))
	m = pressRun(t, m, "l")
	delete(w.fail, "a")
	cmd := m.Retry()
	if cmd == nil {
		t.Fatal("Retry() = nil with a failed row")
	}
	m = run(m, cmd)
	if m.Err("a") != nil || w.count("a") != 2 {
		t.Errorf("err %v, loads %d", m.Err("a"), w.count("a"))
	}
	if w.count("") != 1 {
		t.Errorf("top level loaded %d times", w.count(""))
	}
	if m.Retry() != nil {
		t.Error("Retry() with nothing failed returned a cmd")
	}
}

func TestRetryTopLevel(t *testing.T) {
	w := &world{fail: map[string]error{"": errors.New("offline")}}
	m := lazyTree(w)
	delete(w.fail, "")
	m = run(m, m.Retry())
	if !slices.Equal(visible(m), []string{"a", "b"}) {
		t.Errorf("rows = %v", visible(m))
	}
}

func TestExpandAllLimitsGetter(t *testing.T) {
	m := New(Source{}, WithExpandAllLimits(7, 3))
	if n, d := m.ExpandAllLimits(); n != 7 || d != 3 {
		t.Errorf("limits = %d, %d", n, d)
	}
	m.SetExpandAllLimits(9, 4)
	if n, d := m.ExpandAllLimits(); n != 9 || d != 4 {
		t.Errorf("limits = %d, %d", n, d)
	}
}

func TestFilterFunc(t *testing.T) {
	rows := []Node{
		{ID: "dir", Lines: []string{"dir"}, Branch: true},
		{ID: "dir/x", Lines: []string{"x.go"}, Depth: 1},
		{ID: "dir/y", Lines: []string{"y.md"}, Depth: 1},
		{ID: "z", Lines: []string{"z.go"}},
	}
	m := New(Rows(rows), WithSize(20, 6))
	m.SetFilterFunc(func(n Node) bool { return strings.HasSuffix(n.Lines[0], ".go") })
	if got := visible(m); !slices.Equal(got, []string{"dir", "dir/x", "z"}) {
		t.Errorf("rows = %v", got)
	}
	if shown, total := m.Filtered("dir"); shown != 1 || total != 2 {
		t.Errorf("Filtered(dir) = %d/%d", shown, total)
	}
	if shown, total := m.Filtered(""); shown != 2 || total != 2 {
		t.Errorf("Filtered(top) = %d/%d", shown, total)
	}
	m.SetFilter("x")
	if got := visible(m); !slices.Equal(got, []string{"dir", "dir/x"}) {
		t.Errorf("combined rows = %v", got)
	}
	m.SetFilter("")
	m.SetFilterFunc(nil)
	if got := visible(m); len(got) != 3 && len(got) != 4 {
		t.Errorf("cleared rows = %v", got)
	}
	if shown, total := m.Filtered("dir"); shown != total {
		t.Errorf("cleared Filtered = %d/%d", shown, total)
	}
}

func TestMinName(t *testing.T) {
	n := Node{ID: "a", Lines: []string{"abcdefgh"}, Right: "rr"}
	wide := New(Rows([]Node{n}), WithSize(12, 2), WithMinName(9))
	if got := lines(wide)[0]; contains(got, "rr") {
		t.Errorf("right part kept: %q", got)
	}
	narrow := New(Rows([]Node{n}), WithSize(12, 2))
	if got := lines(narrow)[0]; !contains(got, "rr") {
		t.Errorf("right part dropped: %q", got)
	}
}
