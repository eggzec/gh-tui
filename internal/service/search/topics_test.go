package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/internal/core"
)

func (f *fakeAPI) SearchTopics(_ context.Context, query, cursor string, perPage int) (core.SearchPage[core.SearchTopic], error) {
	f.record(fmt.Sprintf("topics %q %d %s", query, perPage, cursor))
	if f.topics == nil {
		f.t.Error("unexpected SearchTopics")
		return core.SearchPage[core.SearchTopic]{}, errors.New("unexpected call")
	}
	return f.topics(query, cursor, perPage)
}

func TestTopicsCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, topics: func(string, string, int) (core.SearchPage[core.SearchTopic], error) {
			return core.SearchPage[core.SearchTopic]{
				Items: []core.SearchTopic{{Name: "go"}},
				Total: 1,
			}, nil
		}}
		s := New(api)
		q := TopicsQuery{Text: "  go is:featured "}
		if _, ok := s.CachedTopics(q); ok {
			t.Error("CachedTopics hit before any search")
		}
		got, err := s.Topics(t.Context(), q)
		if err != nil || got.Total != 1 || len(got.Items) != 1 {
			t.Fatalf("Topics = %+v, %v", got, err)
		}
		if _, err := s.Topics(t.Context(), TopicsQuery{Text: "GO is:featured"}); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.CachedTopics(q); !ok {
			t.Error("CachedTopics missed after a search")
		}
		api.wantCalls(t, `topics "go is:featured" 20 `)
	})
}
