package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

type repo struct {
	Name string `json:"name"`
}

func TestGetDecodes(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r" {
			t.Errorf("path = %q, want /repos/o/r", r.URL.Path)
		}
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", "Mon, 21 Sep 2026 10:00:00 GMT")
		_, _ = io.WriteString(w, `{"name":"r"}`)
	}))

	var got repo
	res, err := c.Get(t.Context(), "repos/o/r", Conditional{}, &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "r" {
		t.Errorf("name = %q, want r", got.Name)
	}
	want := Response{StatusCode: 200, ETag: `"abc"`, LastModified: "Mon, 21 Sep 2026 10:00:00 GMT"}
	if res != want {
		t.Errorf("response = %+v, want %+v", res, want)
	}
}

func TestConditionalGet(t *testing.T) {
	tests := []struct {
		name       string
		cond       Conditional
		header     string
		validator  string
		unmodified bool
	}{
		{"etag matches", Conditional{ETag: `"abc"`}, "If-None-Match", `"abc"`, true},
		{"last modified matches", Conditional{LastModified: "Mon, 21 Sep 2026 10:00:00 GMT"}, "If-Modified-Since", "Mon, 21 Sep 2026 10:00:00 GMT", true},
		{"etag stale", Conditional{ETag: `"old"`}, "If-None-Match", `"abc"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(tt.header) == tt.validator {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				_, _ = io.WriteString(w, `{"name":"fresh"}`)
			}))

			got := repo{Name: "cached"}
			res, err := c.Get(t.Context(), "repos/o/r", tt.cond, &got)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if res.NotModified != tt.unmodified {
				t.Errorf("NotModified = %v, want %v", res.NotModified, tt.unmodified)
			}
			wantName := "fresh"
			if tt.unmodified {
				wantName = "cached"
			}
			if got.Name != wantName {
				t.Errorf("name = %q, want %q", got.Name, wantName)
			}
		})
	}
}

func TestDoSendsJSON(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var in repo
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"name":"`+in.Name+`-created"}`)
	}))

	var got repo
	res, err := c.Do(t.Context(), http.MethodPost, "user/repos", repo{Name: "r"}, &got)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res.StatusCode != http.StatusCreated || got.Name != "r-created" {
		t.Errorf("got %d %q, want 201 r-created", res.StatusCode, got.Name)
	}
}

func TestDoNoContent(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	var got repo
	if _, err := c.Do(t.Context(), http.MethodPut, "user/starred/o/r", nil, &got); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestErrorMapping(t *testing.T) {
	sentinels := []error{core.ErrUnauthorized, core.ErrNotFound, core.ErrConflict, core.ErrRateLimited}
	tests := []struct {
		name        string
		status      int
		body        string
		want        error
		wantMessage string
	}{
		{"unauthorized", 401, `{"message":"Bad credentials"}`, core.ErrUnauthorized, "Bad credentials"},
		{"not found", 404, `{"message":"Not Found"}`, core.ErrNotFound, "Not Found"},
		{"conflict", 409, `{"message":"Merge conflict"}`, core.ErrConflict, "Merge conflict"},
		{
			"validation failed", 422,
			`{"message":"Validation Failed","errors":[{"resource":"Issue","field":"title","code":"missing_field"},"plain"]}`,
			core.ErrConflict, "Validation Failed (title missing_field; plain)",
		},
		{"forbidden", 403, `{"message":"Resource not accessible"}`, nil, "Resource not accessible"},
		{"server error without body", 500, ``, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))

			_, err := c.Get(t.Context(), "x", Conditional{}, nil)

			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error %v is not an *Error", err)
			}
			if apiErr.StatusCode != tt.status || apiErr.Message != tt.wantMessage {
				t.Errorf("got %d %q, want %d %q", apiErr.StatusCode, apiErr.Message, tt.status, tt.wantMessage)
			}
			for _, s := range sentinels {
				if got, want := errors.Is(err, s), errors.Is(tt.want, s); got != want {
					t.Errorf("errors.Is(err, %v) = %v, want %v", s, got, want)
				}
			}
		})
	}
}
