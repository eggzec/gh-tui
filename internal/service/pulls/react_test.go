package pulls

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestReactShowsAtOnceAndRollsBack(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)

	op := s.React(repo, 1, "c1", "ROCKET", true)
	want := []core.Reaction{{Content: "ROCKET", Count: 1, Mine: true}}
	if p, _ := s.CachedComments(firstPage); !reflect.DeepEqual(p.Items[0].Reactions, want) {
		t.Fatalf("reactions before Do = %v, want %v", p.Items[0].Reactions, want)
	}
	api.opsErr = map[string]error{"AddReaction": errors.New("boom")}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	if p, _ := s.CachedComments(firstPage); len(p.Items[0].Reactions) != 0 {
		t.Errorf("reactions after the failure = %v, want them rolled back", p.Items[0].Reactions)
	}
}

func TestReactSendsAddAndRemove(t *testing.T) {
	api := &fakeAPI{}
	s := opService(t, api)
	if err := s.React(repo, 1, "c1", "EYES", true).Do(t.Context()); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.React(repo, 1, "c1", "EYES", false).Do(t.Context()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	var got []string
	for _, c := range api.callsOf() {
		got = append(got, c.method)
	}
	if want := []string{"AddReaction", "RemoveReaction"}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}
