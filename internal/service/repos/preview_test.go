package repos

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

type fakePreviewAPI struct {
	calls   atomic.Int32
	noRel   bool
	readErr error
}

func (f *fakePreviewAPI) RepoReadme(context.Context, core.RepoRef) (string, error) {
	f.calls.Add(1)
	return "# R", f.readErr
}

func (f *fakePreviewAPI) LatestRelease(context.Context, core.RepoRef) (core.Release, bool, error) {
	return core.Release{Tag: "v1"}, !f.noRel, nil
}

func (f *fakePreviewAPI) OpenCounts(context.Context, core.RepoRef) (issues, pulls int, err error) {
	return 4, 2, nil
}

func TestPreviewReadsAndCaches(t *testing.T) {
	t.Parallel()
	api := &fakePreviewAPI{}
	p := NewPreviews(api)
	ref := core.RepoRef{Owner: "o", Name: "r"}

	for range 2 {
		got, err := p.Preview(t.Context(), ref)
		if err != nil || got.Readme != "# R" || got.OpenIssues != 4 || got.OpenPulls != 2 || got.Release.Tag != "v1" {
			t.Fatalf("Preview = %+v, %v, want the README, 4 and 2 open, and v1", got, err)
		}
	}
	if n := api.calls.Load(); n != 1 {
		t.Errorf("README read %d times, want once, then cached", n)
	}
}

func TestPreviewWithoutRelease(t *testing.T) {
	t.Parallel()
	p := NewPreviews(&fakePreviewAPI{noRel: true})
	got, err := p.Preview(t.Context(), core.RepoRef{Owner: "o", Name: "r"})
	if err != nil || got.Release.Tag != "" {
		t.Errorf("Preview = %+v, %v, want no release", got, err)
	}
}

func TestPreviewErrorIsNotCached(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	api := &fakePreviewAPI{readErr: boom}
	p := NewPreviews(api)
	ref := core.RepoRef{Owner: "o", Name: "r"}
	if _, err := p.Preview(t.Context(), ref); !errors.Is(err, boom) {
		t.Fatalf("Preview error = %v, want boom", err)
	}
	api.readErr = nil
	if _, err := p.Preview(t.Context(), ref); err != nil {
		t.Errorf("Preview after the failure = %v, want it read again", err)
	}
}
