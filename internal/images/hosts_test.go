package images

import (
	"net/url"
	"testing"
)

func TestAllowedGitHub(t *testing.T) {
	h := newHosts("github.com")
	tests := []struct {
		addr string
		want bool
	}{
		{"https://avatars.githubusercontent.com/u/1?s=64", true},
		{"https://camo.githubusercontent.com/abc/def", true},
		{"https://private-user-images.githubusercontent.com/1/2-x.png?jwt=a.b.c", true},
		{"https://user-images.githubusercontent.com/1/2.png", true},
		{"https://raw.githubusercontent.com/o/r/main/a.png", true},
		{"https://AVATARS.githubusercontent.com:443/u/1", true},
		{"https://github.com/user-attachments/assets/0f1e2d3c-aaaa-bbbb-cccc-0123456789ab", true},
		{"https://github.com/octo/repo/assets/1/0f1e2d3c-aaaa-bbbb-cccc-0123456789ab", true},
		// Anyone can name a bucket so: only a redirect from GitHub leads there.
		{"https://github-production-user-asset-6210df.s3.amazonaws.com/1/x.png?X-Amz-Expires=300", false},

		{"http://avatars.githubusercontent.com/u/1", false},
		{"https://user@avatars.githubusercontent.com/u/1", false},
		{"https://avatars.githubusercontent.com:8443/u/1", false},
		{"https://camo.githubusercontent.com.evil.test/x", false},
		{"https://evil.test/avatars.githubusercontent.com", false},
		{"https://github.com/login", false},
		{"https://github.com/user-attachments/assets/", false},
		{"https://github.com/user-attachments/assets/../../login", false},
		{"https://github.com/user-attachments/assets/%2e%2e/x", false},
		{"https://api.github.com/user", false},
		{"https://github-production-user-asset-.s3.amazonaws.com/x", false},
		{"https://github-production-user-asset-a.b.s3.amazonaws.com/x", false},
		{"https://evil-github-production-user-asset-1.s3.amazonaws.com/x", false},
		{"https://objects.githubusercontent.com/x", false},
		{"https://github.com/octo/repo/assets", false},
		{"https://github.com/octo/repo/assets/", false},
		{"https://github.com/octo/repo/blob/main/a.png", false},
		{"https://github.com/octo/repo/assets/../../settings", false},
		{"https://github.com//repo/assets/1/x", false},
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

func TestAllowedEnterprise(t *testing.T) {
	h := newHosts("GHE.example.com")
	tests := []struct {
		addr string
		want bool
	}{
		{"https://ghe.example.com/avatars/u/1?s=64", true},
		{"https://ghe.example.com/storage/user/1/files/x.png", true},
		{"https://ghe.example.com/user-attachments/assets/x", true},
		{"https://avatars.ghe.example.com/u/1", true},
		{"https://media.ghe.example.com/user/1/files/x", true},

		{"https://ghe.example.com/api/v3/user", false},
		{"https://ghe.example.com/storage/../api/v3/user", false},
		{"https://api.ghe.example.com/x", false},
		{"https://avatars.githubusercontent.com/u/1", false},
		{"https://camo.githubusercontent.com/x", false},
		{"https://github.com/user-attachments/assets/x", false},
		{"https://evil.ghe.example.com/x", false},
	}
	for _, tt := range tests {
		u, _ := url.Parse(tt.addr)
		if got := h.allowed(u); got != tt.want {
			t.Errorf("allowed(%s) = %v, want %v", tt.addr, got, tt.want)
		}
	}
	// A server on a port of its own is served there only.
	p := newHosts("ghe.example.com:8443")
	for addr, want := range map[string]bool{
		"https://ghe.example.com:8443/avatars/u/1": true,
		"https://ghe.example.com/avatars/u/1":      false,
	} {
		u, _ := url.Parse(addr)
		if got := p.allowed(u); got != want {
			t.Errorf("port 8443: allowed(%s) = %v, want %v", addr, got, want)
		}
	}
}

// A bucket is fetched only as the target of a redirect from one of
// github.com's attachments.
func TestRedirectToBucket(t *testing.T) {
	h := newHosts("github.com")
	s3 := "https://github-production-user-asset-6210df.s3.amazonaws.com/1/x.png"
	tests := []struct {
		from, to string
		want     bool
	}{
		{"https://github.com/user-attachments/assets/0f1e2d3c-aaaa-bbbb-cccc-0123456789ab", s3, true},
		{"https://github.com/octo/repo/assets/1/0f1e2d3c-aaaa-bbbb-cccc-0123456789ab", s3, true},
		{"https://private-user-images.githubusercontent.com/1/2.png", s3, true},
		{"", s3, false},
		{"https://avatars.githubusercontent.com/u/1", s3, false},
		{"https://camo.githubusercontent.com/a/b", s3, false},
		{s3, "https://github-production-user-asset-evil1.s3.amazonaws.com/x", false},
		{"https://github.com/user-attachments/assets/x", "http://github-production-user-asset-6210df.s3.amazonaws.com/x", false},
		{"https://github.com/user-attachments/assets/x", "https://evil.test/x", false},
		{"https://github.com/user-attachments/assets/x", "https://avatars.githubusercontent.com/u/1", true},
	}
	for _, tt := range tests {
		var from *url.URL
		if tt.from != "" {
			from, _ = url.Parse(tt.from)
		}
		to, _ := url.Parse(tt.to)
		if got := h.redirect(from, to); got != tt.want {
			t.Errorf("redirect(%s -> %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
	// An Enterprise Server has no buckets to follow.
	e := newHosts("ghe.example.com")
	from, _ := url.Parse("https://ghe.example.com/storage/user/1/files/x")
	to, _ := url.Parse(s3)
	if e.redirect(from, to) {
		t.Error("an Enterprise Server's attachment may redirect to a github.com bucket")
	}
}
