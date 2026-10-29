package actions

import "testing"

// ubuntuName is the name of the failed run's job that holds the error.
const ubuntuName = "test (ubuntu-latest, 1.26)"

// toJobRow steps down the rows of the run under the cursor, stepping over the
// groups (which open on a failure) on the way, to the row of the job named name, not the group it is in.
func (h *host) toJobRow(tb testing.TB, name string) {
	tb.Helper()
	h.toJobs(tb)
	for range 60 {
		j, ok := h.m.cursorJob()
		switch {
		case ok && j.Name == name && !h.m.onGroupRow():
			return
		default:
			h.keys("j")
		}
	}
	tb.Fatalf("the cursor never reached the row of the job %q", name)
}

// openLogOf opens the log of the job named name of the run under the
// cursor, and returns the overlay.
func (h *host) openLogOf(tb testing.TB, name string) *logModal {
	tb.Helper()
	h.toJobRow(tb, name)
	h.keys("enter")
	if h.log == nil {
		tb.Fatalf("enter on the job %q didn't open its log", name)
	}
	return h.log
}
