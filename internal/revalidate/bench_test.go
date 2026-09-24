package revalidate

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// BenchmarkDue orders the entries of a large cache for a pass.
func BenchmarkDue(b *testing.B) {
	now := time.Now()
	entries := make([]Entry, 5000)
	for i := range entries {
		entries[i] = Entry{
			ID:     "entry" + strconv.Itoa(i),
			Repo:   core.RepoRef{Owner: "octo", Name: "repo" + strconv.Itoa(i%50)},
			UsedAt: now.Add(-time.Duration(i) * time.Minute),
			Check:  func(context.Context) Result { return Result{} },
		}
	}
	r := New(nil)
	r.SetRepo(core.RepoRef{Owner: "octo", Name: "repo7"})
	b.ReportAllocs()
	for b.Loop() {
		first, rest := r.due(entries, now)
		if len(first)+len(rest) == 0 {
			b.Fatal("nothing due")
		}
	}
}
