package github

import (
	"io"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestParseLinks(t *testing.T) {
	const (
		next  = "https://api.github.com/repositories/1/pulls?page=2"
		last  = "https://api.github.com/repositories/1/pulls?page=5"
		first = "https://api.github.com/repositories/1/pulls?page=1"
	)
	tests := []struct {
		name   string
		header string
		want   map[string]string
	}{
		{"missing", "", map[string]string{}},
		{
			"next and last",
			`<` + next + `>; rel="next", <` + last + `>; rel="last"`,
			map[string]string{"next": next, "last": last},
		},
		{
			"last page has no next",
			`<` + first + `>; rel="prev", <` + first + `>; rel="first"`,
			map[string]string{"prev": first, "first": first},
		},
		{
			"comma in query",
			`<https://api.github.com/issues?labels=a,b&page=2>; rel="next"`,
			map[string]string{"next": "https://api.github.com/issues?labels=a,b&page=2"},
		},
		{
			"several rels and extra params",
			`<` + last + `>; title="end"; rel="last next"`,
			map[string]string{"next": last, "last": last},
		},
		{"malformed", `<` + next + `; rel="next"`, map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLinks(tt.header); !maps.Equal(got, tt.want) {
				t.Errorf("parseLinks = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetFollowsNext(t *testing.T) {
	var base string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", `<`+base+`items?page=2>; rel="next", <`+base+`items?page=2>; rel="last"`)
			_, _ = io.WriteString(w, `[1,2]`)
		case "2":
			w.Header().Set("X-Poll-Interval", "60")
			_, _ = io.WriteString(w, `[3]`)
		}
	}))
	base = c.restURL.String()

	var all []int
	path := "items"
	var res Response
	for path != "" {
		var page []int
		var err error
		res, err = c.Get(t.Context(), path, Conditional{}, &page)
		if err != nil {
			t.Fatalf("Get %s: %v", path, err)
		}
		all = append(all, page...)
		path = res.Next
	}

	if want := []int{1, 2, 3}; !slices.Equal(all, want) {
		t.Errorf("items = %v, want %v", all, want)
	}
	if res.PollInterval != time.Minute {
		t.Errorf("PollInterval = %v, want 1m", res.PollInterval)
	}
}
