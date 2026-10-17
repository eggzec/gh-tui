package pulls

import (
	"context"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// The title of the diff asks for the paths on every draw, so they must come at
// once while the files are being read.
func TestDiffSourcePathsDoNotWaitForTheRead(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	src := newDiffSource(func(context.Context) ([]core.CommitFile, error) {
		close(started)
		<-release
		return []core.CommitFile{{Path: "b.go"}, {Path: "a.go"}}, nil
	})
	done := make(chan []core.CommitFile)
	go func() {
		fs, _ := src.read(t.Context())
		done <- fs
	}()
	<-started

	got := make(chan []string)
	go func() { got <- src.paths() }()
	if paths := <-got; paths != nil {
		t.Errorf("paths while the read is in flight = %v, want none", paths)
	}

	close(release)
	if fs := <-done; len(fs) != 2 {
		t.Fatalf("read %d files, want 2", len(fs))
	}
	if paths := src.paths(); !slices.Equal(paths, []string{"a.go", "b.go"}) {
		t.Errorf("paths after the read = %v", paths)
	}
}
