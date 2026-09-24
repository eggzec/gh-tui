package github

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestViewerContributions(t *testing.T) {
	c, reqs := serveFixture(t, "viewer_contributions.json")

	got, err := c.ViewerContributions(t.Context())
	if err != nil {
		t.Fatalf("ViewerContributions: %v", err)
	}
	if req := <-reqs; req.Query != viewerContributionsQuery || len(req.Variables) != 0 {
		t.Errorf("request = %q with %v, want viewerContributionsQuery without variables", req.Query, req.Variables)
	}

	if got.Total != 4246 || len(got.Weeks) != 53 {
		t.Fatalf("calendar = %d contributions in %d weeks, want 4246 in 53", got.Total, len(got.Weeks))
	}
	first := core.ContributionDay{Date: time.Date(2025, 9, 21, 0, 0, 0, 0, time.UTC)}
	if d := got.Weeks[0][0]; d != first {
		t.Errorf("first day = %+v, want %+v", d, first)
	}
	sum, levels := 0, map[int]bool{}
	for _, w := range got.Weeks {
		for _, d := range w {
			sum += d.Count
			levels[d.Level] = true
			if (d.Count == 0) != (d.Level == 0) {
				t.Errorf("%s: level %d for %d contributions", d.Date.Format(time.DateOnly), d.Level, d.Count)
			}
		}
	}
	if sum != got.Total {
		t.Errorf("days sum to %d, want the total %d", sum, got.Total)
	}
	for l := range 5 {
		if !levels[l] {
			t.Errorf("no day of level %d", l)
		}
	}
}

func TestViewerContributionsBadDate(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"viewer":{"contributionsCollection":{"contributionCalendar":{"totalContributions":1,
			"weeks":[{"contributionDays":[{"date":"Sep 24","contributionCount":1,"contributionLevel":"FIRST_QUARTILE"}]}]}}}}}`)
	}))

	if _, err := c.ViewerContributions(t.Context()); err == nil {
		t.Error("error = nil, want one for a malformed date")
	}
}

func BenchmarkDecodeContributions(b *testing.B) {
	body := benchFixture(b, "viewer_contributions.json")
	for b.Loop() {
		var data struct {
			Viewer struct {
				ContributionsCollection struct {
					ContributionCalendar struct {
						Weeks []struct {
							Days []contributionDay `json:"contributionDays"`
						} `json:"weeks"`
					} `json:"contributionCalendar"`
				} `json:"contributionsCollection"`
			} `json:"viewer"`
		}
		decodeData(b, body, &data)
		for _, w := range data.Viewer.ContributionsCollection.ContributionCalendar.Weeks {
			for _, d := range w.Days {
				if _, err := d.core(); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}

// benchFixture reads testdata/name and readies b to decode it.
func benchFixture(b *testing.B, name string) []byte {
	b.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	return body
}

// decodeData decodes a GraphQL response body the way Query does, its data
// into v.
func decodeData(b *testing.B, body []byte, v any) {
	b.Helper()
	var resp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := decode(bytes.NewReader(body), &resp); err != nil {
		b.Fatal(err)
	}
	if err := json.Unmarshal(resp.Data, v); err != nil {
		b.Fatal(err)
	}
}
