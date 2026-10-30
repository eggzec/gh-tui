package config

import (
	"strings"
	"testing"
)

func TestImagesTrustedHosts(t *testing.T) {
	t.Parallel()
	valid := Images{Enabled: ImagesAuto, MaxRows: 20}
	for _, h := range []string{"example.com", "img.example.com", "*.example.com", "Media.Example.COM"} {
		i := valid
		i.TrustedHosts = []string{h}
		if err := i.validate(); err != nil {
			t.Errorf("%q: %v, want it accepted", h, err)
		}
	}
	for _, h := range []string{
		"", "*", "*.com", "https://example.com", "example.com/path", "example.com:8443",
		"127.0.0.1", "[::1]", "::1", "a*.example.com", "*.*.example.com", ".example.com", "example.com.", "ex ample.com",
	} {
		i := valid
		i.TrustedHosts = []string{h}
		err := i.validate()
		if err == nil {
			t.Errorf("%q accepted, want an error", h)
			continue
		}
		if want := `images.trusted_hosts: "` + h + `"`; !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %q does not name the entry", h, err)
		}
	}
}
