package files

import (
	"maps"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

func openDisk(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// keptHead returns a disk store that a session which listed HEAD of repo at
// *commit left behind.
func keptHead(t *testing.T, commit *string) *disk.Store {
	t.Helper()
	store := openDisk(t)
	if _, err := New(&fakeAPI{t: t, treeAll: server(commit)}, WithStore(store)).All(t.Context(), head); err != nil {
		t.Fatalf("All: %v", err)
	}
	return store
}

// checkOne checks the only ref s keeps.
func checkOne(t *testing.T, s *Service) (revalidate.Entry, revalidate.Result) {
	t.Helper()
	entries := s.Kept()
	if len(entries) != 1 {
		t.Fatalf("Kept = %+v, want the listing of HEAD", entries)
	}
	e := entries[0]
	if e.Repo != repo || e.UsedAt.IsZero() {
		t.Errorf("entry = %+v, want one of %v", e, repo)
	}
	res := e.Check(t.Context())
	return e, res
}

func TestKeptRefNotMoved(t *testing.T) {
	commit := commitA
	store := keptHead(t, &commit)
	used := maps.Collect(store.List(kindRef))

	api := &fakeAPI{t: t, treeAll: server(&commit)}
	s := New(api, WithStore(store))
	if _, r := checkOne(t, s); r.Status != revalidate.NotModified || r.Sync != "" {
		t.Errorf("check = %+v, want not modified, with nothing to show", r)
	}
	api.wantCalls(t, `all eggzec/gh-tui HEAD "`+commitA+`"`)
	if e, _ := checkOne(t, s); time.Since(e.CheckedAt) > time.Minute {
		t.Errorf("checked at %v, want now", e.CheckedAt)
	}
	// Checking isn't using.
	if got := maps.Collect(store.List(kindRef)); !maps.EqualFunc(got, used, time.Time.Equal) {
		t.Errorf("used at %v after the check, want %v", got, used)
	}
	if _, ok := s.CachedAll(head); ok {
		t.Error("the listing is in memory, want it left alone")
	}
}

func TestKeptRefMoved(t *testing.T) {
	commit := commitA
	store := keptHead(t, &commit)
	commit = commitB

	s := New(&fakeAPI{t: t, treeAll: server(&commit)}, WithStore(store))
	if _, r := checkOne(t, s); r.Status != revalidate.Changed || r.Sync != SyncKey(repo) {
		t.Errorf("check = %+v, want changed, reporting %q", r, SyncKey(repo))
	}
	// The next session lists the new commit without a request.
	api := &fakeAPI{t: t}
	got, err := New(api, WithStore(store)).All(t.Context(), head)
	if err != nil || got.SHA != commitB {
		t.Errorf("All in the next session = %+v, %v; want the listing at %s", got, err, commitB)
	}
	api.wantCalls(t)
}

func TestKeptRefInMemory(t *testing.T) {
	commit := commitA
	store := keptHead(t, &commit)
	api := &fakeAPI{t: t, treeAll: server(&commit)}
	s := New(api, WithStore(store))
	if _, err := s.All(t.Context(), head); err != nil {
		t.Fatal(err)
	}
	if _, r := checkOne(t, s); r.Status != revalidate.Skipped {
		t.Errorf("check of a fresh listing = %+v, want skipped", r)
	}

	// A force-push moves HEAD while the listing is shown, stale.
	s.Invalidate(repo)
	commit = commitB
	if _, r := checkOne(t, s); r.Status != revalidate.Changed || r.Sync != SyncKey(repo) {
		t.Errorf("check = %+v, want changed", r)
	}
	if got, ok := s.CachedAll(head); !ok || got.SHA != commitB {
		t.Errorf("CachedAll = %+v, want the listing at %s", got, commitB)
	}
	// The first read was fresh from the store.
	api.wantCalls(t, `all eggzec/gh-tui HEAD "`+commitA+`"`)
}

func TestKeptRefFails(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want revalidate.Status
	}{
		{"offline", errOffline, revalidate.Offline},
		{"limited", &core.RateLimitError{Reset: time.Now().Add(time.Hour)}, revalidate.Limited},
		{"gone", core.ErrNotFound, revalidate.Gone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			commit := commitA
			store := keptHead(t, &commit)
			s := New(&fakeAPI{t: t, treeAll: func(string, github.Conditional) (core.Tree, github.Response, error) {
				return core.Tree{}, github.Response{}, tt.err
			}}, WithStore(store))
			if _, r := checkOne(t, s); r.Status != tt.want {
				t.Errorf("check = %+v, want %v", r, tt.want)
			}
		})
	}
}

func TestKeptSkipsOldRecords(t *testing.T) {
	store := openDisk(t)
	v1 := appendString(appendString(appendString([]byte{refVersion1}, commitA), `"e"`), "")
	if err := store.Put(kindRef, refKey(kindListing, repo, "HEAD"), v1); err != nil {
		t.Fatal(err)
	}
	if got := New(&fakeAPI{t: t}, WithStore(store)).Kept(); len(got) != 0 {
		t.Errorf("Kept = %+v, want nothing for a record that doesn't say what it is", got)
	}
	if got := New(&fakeAPI{t: t}, WithStore(newMemStore())).Kept(); len(got) != 0 {
		t.Errorf("Kept of a store that can't list = %+v", got)
	}
}
