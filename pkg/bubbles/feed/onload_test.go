package feed

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type loadedMsg struct{ ids []string }

func TestOnLoadGetsEachStoredChunk(t *testing.T) {
	src := newSource(5, 2)
	var got [][]string
	m := load(t, src, WithOnLoad(func(items []item) tea.Cmd {
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.id)
		}
		got = append(got, ids)
		return func() tea.Msg { return loadedMsg{ids: ids} }
	}))
	if len(got) == 0 || len(got[0]) != 2 || got[0][0] != "0" {
		t.Fatalf("onLoad calls = %v, want the first chunk first", got)
	}
	if m.Len() == 0 {
		t.Fatal("feed shows no rows")
	}
}

func TestOnLoadSkipsFailedChunk(t *testing.T) {
	src := newSource(5, 2)
	src.setFail("", errors.New("boom"))
	var calls int
	load(t, src, WithOnLoad(func([]item) tea.Cmd {
		calls++
		return nil
	}))
	if calls != 0 {
		t.Errorf("onLoad ran %d times for a failed chunk, want 0", calls)
	}
}
