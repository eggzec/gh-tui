package issues

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestReact(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	primeTimeline(t, s, api)
	var sent []string
	api.react = func(id, content string, _ bool) error {
		sent = append(sent, id, content)
		return nil
	}

	op := s.React(repo, 7, "IC_2", "HEART", true)
	want := []core.Reaction{{Content: "HEART", Count: 1, Mine: true}}
	if p, _ := s.CachedComments(firstOfTwo); !reflect.DeepEqual(p.Items[1].Reactions, want) {
		t.Errorf("comment reactions = %v, want %v", p.Items[1].Reactions, want)
	}
	if p, _ := s.CachedTimeline(timelineQuery); !reflect.DeepEqual(p.Items[1].Reactions, want) {
		t.Errorf("timeline reactions = %v, want %v", p.Items[1].Reactions, want)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	api.checkCalls(t, "AddReaction")

	if err := s.React(repo, 7, "IC_2", "HEART", false).Do(t.Context()); err != nil {
		t.Fatalf("Do remove: %v", err)
	}
	if p, _ := s.CachedComments(firstOfTwo); len(p.Items[1].Reactions) != 0 {
		t.Errorf("reactions = %v, want none once taken back", p.Items[1].Reactions)
	}
	api.checkCalls(t, "RemoveReaction")
}

func TestReactRollsBack(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.react = func(string, string, bool) error { return fakeError("boom") }

	if err := s.React(repo, 7, "IC_2", "EYES", true).Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	if p, _ := s.CachedComments(firstOfTwo); len(p.Items[1].Reactions) != 0 {
		t.Errorf("reactions = %v, want the old ones back", p.Items[1].Reactions)
	}
}

func TestReactRefusesPending(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	if err := s.React(repo, 7, "pending:1", "EYES", true).Do(t.Context()); !errors.Is(err, errUnsent) {
		t.Errorf("err = %v, want errUnsent", err)
	}
	api.checkCalls(t)
}
