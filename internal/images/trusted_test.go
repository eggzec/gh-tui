package images

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAllowedTrustedHosts(t *testing.T) {
	h := newHosts("github.com", "img.example.com", "*.cdn.test", "Mixed.Case.Test", "bad.test:8443", "*bad", "")
	tests := []struct {
		addr string
		want bool
	}{
		{"https://img.example.com/a.png", true},
		{"https://other.example.com/a.png", false},
		{"https://example.com/a.png", false},
		{"https://a.cdn.test/a.png", true},
		{"https://a.b.cdn.test/a.png", true},
		{"https://cdn.test/a.png", false},
		{"https://evilcdn.test/a.png", false},
		{"https://IMG.Example.COM/a.png", true},
		{"https://mixed.case.test/a.png", true},
		{"http://img.example.com/a.png", false},
		{"https://img.example.com:8443/a.png", false},
		{"https://user@img.example.com/a.png", false},
		{"https://bad.test/a.png", false},
		// The built-in hosts stay.
		{"https://avatars.githubusercontent.com/u/1", true},
	}
	for _, tt := range tests {
		u, err := url.Parse(tt.addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := h.allowed(u); got != tt.want {
			t.Errorf("allowed(%s) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestRedirectFromTrustedToPrivate(t *testing.T) {
	h := newHosts("github.com", "img.example.com")
	from, _ := url.Parse("https://img.example.com/a.png")
	for _, to := range []string{"https://127.0.0.1/a.png", "https://10.0.0.1/a.png", "https://localhost/a.png"} {
		u, _ := url.Parse(to)
		if h.redirect(from, u) {
			t.Errorf("redirect to %s allowed", to)
		}
	}
}

func TestTrustedDialRefusesPrivate(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	// The server is at a loopback address: refused as a trusted host,
	// reached as any other.
	trusted := newHosts("github.com", u.Hostname()).dialer()
	if _, err := trusted.DialContext(context.Background(), "tcp", u.Host); !errors.Is(err, errPrivate) {
		t.Fatalf("trusted dial err = %v, want errPrivate", err)
	}
	plain := newHosts("github.com").dialer()
	conn, err := plain.DialContext(context.Background(), "tcp", u.Host)
	if err != nil {
		t.Fatalf("plain dial: %v", err)
	}
	_ = conn.Close()
}
