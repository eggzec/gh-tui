package pulls

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// MergeTexts reads every time: the text depends on the viewer and the head.
func TestMergeTextsReadsEveryTime(t *testing.T) {
	want := core.MergeTexts{
		Merge:  core.MergeMessage{Title: "Merge pull request #1", SendBody: true},
		Squash: core.MergeMessage{Title: "Add it (#1)", Body: "* one", SendBody: true},
	}
	api := &fakeAPI{mergeTexts: want}
	s := New(api)
	for range 2 {
		got, err := s.MergeTexts(t.Context(), repo, 1)
		if err != nil || got != want {
			t.Fatalf("MergeTexts = %+v, %v; want %+v", got, err, want)
		}
	}
	if n := api.count("mergetexts"); n != 2 {
		t.Errorf("reads = %d, want 2: the text is not cached", n)
	}
}

// Serial: it replaces the global slog default to read the log.
func TestMergeTextsError(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	api := &fakeAPI{mergeTextsErr: core.ErrForbidden}
	_, err := New(api).MergeTexts(t.Context(), repo, 1)
	if !errors.Is(err, core.ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
	if got := buf.String(); !strings.Contains(got, `"level":"WARN"`) || !strings.Contains(got, "merge message not read") {
		t.Errorf("log = %q, want a warning that the merge message was not read", got)
	}
}
