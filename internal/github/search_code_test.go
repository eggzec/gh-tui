package github

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// serveCodeFixture answers code searches with testdata/name and status,
// after checking that each asks for text matches.
func serveCodeFixture(t *testing.T, name string, status int, check func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != codeSearchAccept {
			t.Errorf("Accept = %q, want %q", got, codeSearchAccept)
		}
		check(w, r)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
}

func TestSearchCode(t *testing.T) {
	var host string
	c := serveCodeFixture(t, "search_code.json", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/search/code", map[string]string{"q": "NewStyle repo:charmbracelet/lipgloss", "per_page": "3"})
		host = r.Host
		w.Header().Set("Link", `<http://`+r.Host+`/search/code?q=NewStyle+repo%3Acharmbracelet%2Flipgloss&page=2&per_page=3>; rel="next"`)
		w.Header().Set("X-RateLimit-Limit", "10")
		w.Header().Set("X-RateLimit-Remaining", "9")
		w.Header().Set("X-RateLimit-Reset", "1790252795")
		w.Header().Set("X-RateLimit-Resource", "code_search")
	})
	page, err := c.SearchCode(t.Context(), "NewStyle repo:charmbracelet/lipgloss", "", 3)
	if err != nil {
		t.Fatalf("SearchCode: %v", err)
	}
	if page.Total != 48 || page.Incomplete || len(page.Items) != 3 {
		t.Fatalf("page = total %d, incomplete %t, %d hits; want 48, false, 3", page.Total, page.Incomplete, len(page.Items))
	}
	if want := "http://" + host + "/search/code?q=NewStyle+repo%3Acharmbracelet%2Flipgloss&page=2&per_page=3"; page.Next != want {
		t.Errorf("Next = %q, want %q", page.Next, want)
	}

	hit := page.Items[0]
	lipgloss := core.RepoRef{Owner: "charmbracelet", Name: "lipgloss"}
	if hit.Repo != lipgloss || hit.Path != "style.go" || hit.SHA != "3cd659526767c9390789b387ff4064e838ad79c8" ||
		hit.URL != "https://github.com/charmbracelet/lipgloss/blob/6a419c6543d3475a369ef08f6252a2a6f33be733/style.go" {
		t.Errorf("hit 0 = %s %s %s %s", hit.Repo, hit.Path, hit.SHA, hit.URL)
	}
	wantFragments := []int{1, 2, 2}
	for i, h := range page.Items {
		if len(h.Fragments) != wantFragments[i] {
			t.Errorf("hit %d has %d fragments, want %d", i, len(h.Fragments), wantFragments[i])
		}
		for _, f := range h.Fragments {
			if len(f.Matches) == 0 {
				t.Errorf("hit %d: fragment %q has no matches", i, f.Text)
			}
			for _, m := range f.Matches {
				if got := f.Text[m[0]:m[1]]; got != "NewStyle" {
					t.Errorf("hit %d: match %v = %q, want NewStyle", i, m, got)
				}
			}
		}
	}
	if got := hit.Fragments[0].Matches; len(got) != 2 || got[0] != [2]int{3, 11} || got[1] != [2]int{212, 220} {
		t.Errorf("matches = %v, want [[3 11] [212 220]]", got)
	}
	if rl := c.RateLimit(); rl.Resource != "code_search" || rl.Limit != 10 || rl.Remaining != 9 {
		t.Errorf("RateLimit = %+v, want the code search quota", rl)
	}
}

// GitHub gives match offsets in bytes, so a match after a multibyte
// character still slices out of the fragment.
func TestSearchCodeMultibyte(t *testing.T) {
	c := serveCodeFixture(t, "search_code_multibyte.json", http.StatusOK, func(http.ResponseWriter, *http.Request) {})
	page, err := c.SearchCode(t.Context(), `"╭" repo:charmbracelet/lipgloss`, "", 3)
	if err != nil {
		t.Fatalf("SearchCode: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("got %d hits, want 3", len(page.Items))
	}
	for _, h := range page.Items {
		for _, f := range h.Fragments {
			for _, m := range f.Matches {
				if got := f.Text[m[0]:m[1]]; got != "╭" {
					t.Errorf("%s: match %v = %q, want ╭", h.Path, m, got)
				}
			}
		}
	}
}

func TestCodeTextMatchDropsBadOffsets(t *testing.T) {
	var m codeTextMatch
	m.Fragment = "abc"
	for _, idx := range [][2]int{{0, 1}, {2, 1}, {1, 4}, {-1, 1}, {3, 3}} {
		m.Matches = append(m.Matches, struct {
			Indices [2]int `json:"indices"`
		}{idx})
	}
	if got := m.core().Matches; len(got) != 2 || got[0] != [2]int{0, 1} || got[1] != [2]int{3, 3} {
		t.Errorf("matches = %v, want only those inside the fragment", got)
	}
}

func TestSearchCodeInvalidQuery(t *testing.T) {
	c := serveCodeFixture(t, "search_code_invalid.json", http.StatusUnprocessableEntity, func(http.ResponseWriter, *http.Request) {})
	_, err := c.SearchCode(t.Context(), "x", "", 0)
	if !errors.Is(err, core.ErrInvalidQuery) {
		t.Fatalf("error = %v, want an invalid query", err)
	}
	if want := "search code: invalid search query: q missing"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestSearchCodeRateLimited(t *testing.T) {
	now := time.Unix(1790000000, 0)
	reset := now.Add(42 * time.Second)
	tests := map[string]func(w http.ResponseWriter){
		"primary": func(w http.ResponseWriter) {
			w.Header().Set("X-RateLimit-Limit", "10")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
			w.Header().Set("X-RateLimit-Resource", "code_search")
			w.WriteHeader(http.StatusForbidden)
		},
		"secondary": func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "42")
			w.WriteHeader(http.StatusForbidden)
		},
	}
	for name, respond := range tests {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				respond(w)
				_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
			}))
			c.now = func() time.Time { return now }
			_, err := c.SearchCode(t.Context(), "x", "", 0)
			rl, ok := errors.AsType[*core.RateLimitError](err)
			if !ok || !errors.Is(err, core.ErrRateLimited) {
				t.Fatalf("error = %v, want a rate limit", err)
			}
			if !rl.Reset.Equal(reset) {
				t.Errorf("reset = %v, want %v", rl.Reset, reset)
			}
		})
	}
}
