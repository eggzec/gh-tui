package notifications

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// loaded lists the threads in the order the list shows them.
func loaded(s *Section) []core.Notification {
	var out []core.Notification
	for i := range s.feed.Len() {
		if n, ok := s.feed.Item(i); ok {
			out = append(out, n)
		}
	}
	return out
}

// TestSortKeyOpensTheModal checks that s asks the app for the sort modal.
func TestSortKeyOpensTheModal(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	for _, m := range press(t, s, "s") {
		if open, ok := m.(ui.OpenModalMsg); ok {
			if !strings.HasPrefix(open.Modal.Title(), "Sort") {
				t.Errorf("title = %q", open.Modal.Title())
			}
			return
		}
	}
	t.Fatal("s did not open a modal")
}

// TestSortOrder checks the order of several fields, the newest update among
// the ties, and a flipped direction.
func TestSortOrder(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	run(t, s, s.setSort(levels("repository")))
	got := loaded(s)
	if len(got) < 2 {
		t.Fatalf("only %d threads", len(got))
	}
	check := func(name string, levels []orderpick.Choice) {
		less := s.sortLess(levels)
		got := loaded(s)
		for i := 1; i < len(got); i++ {
			if less(got[i], got[i-1]) < 0 {
				t.Errorf("%s: %s before %s", name, got[i-1].ID, got[i].ID)
			}
		}
	}
	check("repository", levels("repository"))

	flipped := []orderpick.Choice{{Value: "repository", Desc: true}}
	run(t, s, s.setSort(flipped))
	check("flipped", flipped)

	oldest := []orderpick.Choice{{Value: updatedField, Desc: true}}
	run(t, s, s.setSort(oldest))
	check("oldest first", oldest)
}

// TestSortMultiKeyOrder checks an explicit order of two keys: a tie falls
// through to the newest update, and a thread without a value goes last.
func TestSortMultiKeyOrder(t *testing.T) {
	t.Parallel()
	threads := []core.Notification{
		thread("1", "a/x", core.SubjectIssue, "one", "mention", true, time.Hour),
		thread("2", "b/y", core.SubjectIssue, "two", "mention", true, time.Hour),
		thread("3", "b/y", core.SubjectIssue, "three", "author", true, 3*time.Hour),
		thread("4", "b/y", core.SubjectIssue, "four", "author", true, 2*time.Hour),
		thread("5", "a/x", core.SubjectIssue, "five", "", true, time.Minute),
	}
	s := newSection(t, newFake(threads...), 80, 12)
	run(t, s, s.setSort(levels("reason", "repository")))
	var got []string
	for _, n := range loaded(s) {
		got = append(got, n.ID)
	}
	if want := "4 3 1 2 5"; strings.Join(got, " ") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

// TestSortKeepsTheOrderAcrossPages checks that a page that arrives while
// sorted joins the order, and that the cursor stays on its thread.
func TestSortKeepsTheOrderAcrossPages(t *testing.T) {
	t.Parallel()
	var threads []core.Notification
	for i := range 60 {
		repo := string(rune('a'+(i*7)%26)) + "/r"
		threads = append(threads, thread(strconv.Itoa(i+1), repo, core.SubjectIssue, "t"+repo, "mention", true, time.Duration(i+1)*time.Hour))
	}
	svc := newFake(threads...)
	svc.size = 3
	s := newSection(t, svc, 80, 6)
	run(t, s, s.setSort(levels("repository")))
	less := s.sortLess(levels("repository"))
	first := s.feed.Len()
	var prev string
	for range 2 * len(threads) {
		if s.feed.Len() > first {
			break
		}
		cur, _ := s.feed.Selected()
		prev = cur.ID
		press(t, s, "j")
	}
	if s.feed.Len() <= first {
		t.Fatalf("no second page arrived: %d rows", first)
	}
	got := loaded(s)
	for i := 1; i < len(got); i++ {
		if less(got[i], got[i-1]) < 0 {
			t.Errorf("%s before %s", got[i-1].ID, got[i].ID)
		}
	}
	// The press moved the cursor one row on, in the order the new page made.
	cur, _ := s.feed.Selected()
	ok := false
	for i, n := range got {
		if n.ID == prev && i+1 < len(got) && got[i+1].ID == cur.ID {
			ok = true
		}
	}
	if !ok && cur.ID != prev {
		t.Errorf("cursor on %s, want %s or the row after it", cur.ID, prev)
	}
}

// TestSortKeepsTheCursor checks that the cursor stays on its thread.
func TestSortKeepsTheCursor(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	press(t, s, "j", "j")
	want, ok := s.feed.Selected()
	if !ok {
		t.Fatal("nothing selected")
	}
	run(t, s, s.setSort([]orderpick.Choice{{Value: "repository", Desc: true}, {Value: "type"}}))
	if got, _ := s.feed.Selected(); got.ID != want.ID {
		t.Errorf("cursor on %s, want %s", got.ID, want.ID)
	}
}

// TestSortToggle checks that alt+s switches the sort off and on, and that the
// chip says so.
func TestSortToggle(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	if _, ok := s.toggleSort(); ok {
		t.Fatal("toggled with nothing sorted")
	}
	api := loaded(s)

	run(t, s, s.setSort([]orderpick.Choice{{Value: updatedField, Desc: true}}))
	sorted := loaded(s)
	press(t, s, "alt+s")
	if !s.sortOff {
		t.Fatal("alt+s did not switch the sort off")
	}
	off := loaded(s)
	for i := range api {
		if off[i].ID != api[i].ID {
			t.Fatalf("row %d is %s off, want %s from the API", i, off[i].ID, api[i].ID)
		}
	}
	if view := ansi.Strip(s.View()); !strings.Contains(view, "sort updated") || !strings.Contains(view, "off") {
		t.Errorf("the header should show the sort as off:\n%s", view)
	}
	golden.RequireEqual(t, s.View())
	press(t, s, "alt+s")
	if s.sortOff {
		t.Fatal("alt+s did not restore the sort")
	}
	for i, n := range loaded(s) {
		if n.ID != sorted[i].ID {
			t.Fatalf("row %d is %s restored, want %s", i, n.ID, sorted[i].ID)
		}
	}
}
