package github

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// TestOnOldEnterprise tells of a server older than supported once, and of
// a supported one, or github.com, which tells no version, never.
func TestOnOldEnterprise(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    []string
	}{
		{"old", "3.12.4", []string{"3.12.4"}},
		{"oldest supported", core.MinEnterprise + ".0", nil},
		{"new", "3.22.1", nil},
		{"github.com", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.version != "" {
					w.Header().Set(enterpriseHeader, tt.version)
				}
				_, _ = w.Write([]byte("{}"))
			}))
			t.Cleanup(srv.Close)
			var (
				mu   sync.Mutex
				told []string
			)
			c, err := New(WithBaseURL(srv.URL), WithToken("t"), WithHTTPClient(srv.Client()),
				WithOnOldEnterprise(func(v string) {
					mu.Lock()
					told = append(told, v)
					mu.Unlock()
				}))
			if err != nil {
				t.Fatal(err)
			}
			for range 3 {
				var v struct{}
				if _, err := c.Get(t.Context(), "repos/o/r", Conditional{}, &v); err != nil {
					t.Fatalf("Get: %v", err)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if !slices.Equal(told, tt.want) {
				t.Errorf("told %q, want %q", told, tt.want)
			}
			if got := c.enterprise.Load(); got != (tt.version != "") {
				t.Errorf("enterprise = %v, want %v", got, tt.version != "")
			}
		})
	}
}
