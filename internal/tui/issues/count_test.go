package issues

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// bar is the header line over the list, as shown.
func bar(h *host) string { return ansi.Strip(strings.SplitN(h.View(), "\n", 2)[0]) }

func TestCountUnfiltered(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(5))
	svc.total = 40
	h := started(t, svc, 120, 12)
	forms := h.countForms()
	if len(forms) == 0 || !strings.HasPrefix(forms[0], "40") {
		t.Fatalf("forms = %q, want the total first", forms)
	}
	if got := bar(h); !strings.Contains(got, forms[0]) {
		t.Errorf("bar %q lacks %q", got, forms[0])
	}
}

// The total counts the pinned issues, which the pages leave out, so a list
// read to its end must still show the bare total.
func TestCountPinnedIssuesAreLoaded(t *testing.T) {
	t.Parallel()
	all := sampleIssues(8)
	svc := newFakeService(all)
	svc.pin(all[0], all[1])
	// The open tab lists what the fake keeps open, less the pinned two.
	all2 := started(t, svc, 120, 12).list.Len() + 2
	svc.total = all2
	h := started(t, svc, 120, 12)
	forms := h.countForms()
	if len(forms) == 0 || forms[0] != strconv.Itoa(all2) {
		t.Fatalf("forms = %q, want the bare total", forms)
	}
	if got := bar(h); strings.Contains(got, "[") {
		t.Errorf("bar %q shows issues still to load", got)
	}
}

func TestCountUnknownTotalShowsLoaded(t *testing.T) {
	t.Parallel()
	h := started(t, newFakeService(sampleIssues(5)), 120, 12)
	forms := h.countForms()
	if len(forms) == 0 || strings.Contains(forms[0], "/") || strings.HasPrefix(forms[0], "0") {
		t.Errorf("forms = %q, want the loaded count alone", forms)
	}
}

func TestCountFiltered(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(5))
	svc.total = 40
	h := started(t, svc, 120, 12)
	apply(t, h, "label:bug")
	forms := h.countForms()
	if len(forms) == 0 || !strings.Contains(forms[0], "/40") {
		t.Errorf("forms = %q, want matched/all", forms)
	}
}

func TestCountNoneWhileLoadingOrFailed(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(5))
	svc.total = 40
	h := newSection(t, svc, 120, 12)
	if forms := h.countForms(); len(forms) != 0 {
		t.Errorf("before the list loads, forms = %q", forms)
	}
	svc.listErr = errors.New("down")
	h = started(t, svc, 120, 12)
	if forms := h.countForms(); len(forms) != 0 {
		t.Errorf("after a failed load, forms = %q", forms)
	}
}

func TestCountNarrowDropsToBareOrNone(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(5))
	svc.total = 40
	h := started(t, svc, 120, 12)
	for _, w := range []int{40, 30} {
		h.SetSize(w, 12)
		if got := bar(h); strings.Contains(got, "[") {
			t.Errorf("width %d shows the long form: %q", w, got)
		}
	}
}
