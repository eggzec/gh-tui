package files

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

const hoverDelay = 150 * time.Millisecond

// hovering returns a section over f that reads the file under the cursor,
// with the cursor on the submodule, just above the first file.
func hovering(t *testing.T, f *fake) *Section {
	t.Helper()
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), WithHoverPrefetch(hoverDelay, 20_000))
	keys(s, slices.Repeat([]string{"down"}, rowGitignore-1)...)
	return s
}

func TestHoverReadsAfterDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := sampleFake()
		// Directories and the submodule are passed without a read.
		s := hovering(t, f)
		if got := f.blobSHAs(); len(got) != 0 {
			t.Fatalf("read %q before resting on a file", got)
		}
		wait := s.Update(press("down"))
		// Nothing is read until the delay passes.
		if got := f.blobSHAs(); len(got) != 0 {
			t.Fatalf("read %q before the delay", got)
		}
		start := time.Now()
		run(s, wait)
		if d := time.Since(start); d != hoverDelay {
			t.Errorf("read after %v, want %v", d, hoverDelay)
		}
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-.gitignore"}) {
			t.Errorf("read %q, want .gitignore", got)
		}
	})
}

func TestHoverDebounces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := sampleFake()
		s := hovering(t, f)
		// Pass .gitignore and rest on AGENTS.md: the delays run as the
		// program runs them, after both keys.
		passed := s.Update(press("down"))
		rested := s.Update(press("down"))
		run(s, passed)
		run(s, rested)
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-AGENTS.md"}) {
			t.Errorf("read %q, want only AGENTS.md", got)
		}
		// Coming back to a cached file waits for nothing.
		keys(s, "up")
		if cmd := s.Update(press("down")); cmd != nil {
			t.Error("moving to a cached file started a delay")
		}
	})
}

func TestHoverCancelsOlderRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := sampleFake()
		s := hovering(t, f)
		started := make(chan context.Context, 1)
		var agentsErr error
		f.onBlob = func(ctx context.Context, q filesvc.BlobQuery) {
			if q.SHA == "b-.gitignore" {
				// Hang until cancelled.
				started <- ctx
				<-ctx.Done()
				return
			}
			agentsErr = ctx.Err()
		}
		wait := s.Update(press("down"))
		read := s.Update(wait())
		if read == nil {
			t.Fatal("resting on .gitignore read nothing")
		}
		done := make(chan struct{})
		go func() {
			read()
			close(done)
		}()
		first := <-started

		// Resting on AGENTS.md reads it and cancels the read of .gitignore.
		keys(s, "down")
		<-done
		if first.Err() == nil {
			t.Error("the read of .gitignore wasn't cancelled")
		}
		if agentsErr != nil {
			t.Errorf("the read of AGENTS.md ran with %v", agentsErr)
		}
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-.gitignore", "b-AGENTS.md"}) {
			t.Errorf("read %q, want .gitignore, then AGENTS.md", got)
		}
	})
}

func TestHoverStopsOnRepoChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := sampleFake()
		s := hovering(t, f)
		wait := s.Update(press("down"))
		run(s, s.Update(ui.RepoMsg{Repo: other}))
		run(s, wait)
		for _, q := range f.blobReads {
			if q.Repo != other {
				t.Errorf("read %s of the repository left", q.SHA)
			}
		}
	})
}
