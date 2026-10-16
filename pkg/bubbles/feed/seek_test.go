package feed

import (
	"errors"
	"testing"
)

func TestSeek(t *testing.T) {
	tests := []struct {
		name string
		key  string
		fail string
		want string
		page int
		done bool
	}{
		{name: "on the first chunk", key: "3", want: "3", page: 1},
		{name: "on the second chunk", key: "14", want: "14", page: 2},
		{name: "on the last chunk", key: "29", want: "29", page: 3, done: true},
		{name: "missing", key: "99", want: "0", page: 3, done: true},
		{name: "after a failed fetch", key: "14", fail: "10", want: "0", page: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSource(30, 10)
			if tt.fail != "" {
				src.setFail(tt.fail, errors.New("boom"))
			}
			m := newModel(src.fetch, renderItem, WithSize(40, 5), WithFocused(true), WithKey(itemKey))
			if cmd := m.Seek(tt.key); cmd != nil {
				t.Errorf("Seek before Init returned a command")
			}
			if !m.Seeking() {
				t.Fatal("Seek before Init isn't seeking")
			}
			m = run(t, m, m.Init())
			if m.Seeking() {
				t.Error("still seeking once the search ended")
			}
			if it, _ := m.Selected(); it.id != tt.want {
				t.Errorf("Selected() = %v, want item %s", it, tt.want)
			}
			// The window reads ahead of the item it settled on.
			if m.Pages() < tt.page || m.Done() != tt.done && tt.done {
				t.Errorf("fetched %d chunks and done is %v, want at least %d and %v", m.Pages(), m.Done(), tt.page, tt.done)
			}
			assertVisible(t, m)
		})
	}
}

// A seek on a feed that has its chunks looks at them at once, and moves the
// selection by fetching, not by keys.
func TestSeekLoadedFeed(t *testing.T) {
	src := newSource(30, 10)
	m := load(t, src, WithKey(itemKey))
	m = run(t, m, m.Seek("17"))
	if it, _ := m.Selected(); it.id != "17" {
		t.Errorf("Selected() = %v, want item 17", it)
	}
	assertVisible(t, m)
}

// A move of the user's ends the seek: the chunk already being fetched still
// arrives, but it doesn't take the selection away from where they put it.
func TestSeekEndsWhenTheUserMoves(t *testing.T) {
	tests := []struct {
		name string
		move []string
		want string
	}{
		{"down", []string{"j"}, "1"},
		{"down twice", []string{"j", "j"}, "2"},
		{"find", []string{"/", "bug", "enter"}, "2"},
		{"find next", []string{"/", "bug", "enter", "n"}, "7"},
		{"filter", []string{"&", "bug", "enter"}, "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := bugs(40)
			m := load(t, src, WithKey(itemKey))
			// The first chunk is on screen while the second is fetched.
			fetch := m.Seek("35")
			if fetch == nil || !m.Seeking() {
				t.Fatalf("Seek on a loaded feed: command %v, seeking %v", fetch != nil, m.Seeking())
			}
			m = typed(t, m, tt.move...)
			if m.Seeking() {
				t.Error("still seeking after the user moved")
			}
			m = run(t, m, fetch)
			if m.Seeking() {
				t.Error("seeking again once the chunk arrived")
			}
			if it, _ := m.Selected(); it.id != tt.want {
				t.Errorf("Selected() = %v, want item %s", it, tt.want)
			}
			if m.Done() || m.Pages() > 3 {
				t.Errorf("the seek went on: %d chunks, done %v", m.Pages(), m.Done())
			}
			assertVisible(t, m)
		})
	}
}

func TestSeekNeedsAKey(t *testing.T) {
	src := newSource(30, 10)
	m := load(t, src)
	if cmd := m.Seek("17"); cmd != nil || m.Seeking() {
		t.Error("Seek without a key function sought")
	}
}
