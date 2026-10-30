package config

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// Images configures the images drawn inline with kitty's graphics
// protocol, which only some terminals show as the app needs.
type Images struct {
	// Enabled is ImagesAuto, which draws images when the terminal is
	// known to show them, ImagesOn, which draws them without asking the
	// terminal for its kitty graphics but only on a terminal the app
	// knows shows them, or ImagesOff, which never draws or asks.
	Enabled string `yaml:"enabled"`
	// MaxRows is the tallest an inline image may be, in rows.
	MaxRows int `yaml:"max_rows"`
	// Avatars draws the avatars of people and the icons of repositories
	// and organizations.
	Avatars bool `yaml:"avatars"`
	// Animate plays animated GIFs where the terminal can.
	Animate bool `yaml:"animate"`
	// TrustedHosts are hosts other than GitHub's, as example.com or
	// *.example.com, that images are fetched from directly over https.
	TrustedHosts []string `yaml:"trusted_hosts"`
}

// The values of images.enabled.
const (
	ImagesAuto = "auto"
	ImagesOn   = "on"
	ImagesOff  = "off"
)

func (i Images) validate() error {
	var errs []error
	if !slices.Contains([]string{ImagesAuto, ImagesOn, ImagesOff}, i.Enabled) {
		errs = append(errs, fmt.Errorf("images.enabled: must be %s, %s or %s, got %q", ImagesAuto, ImagesOn, ImagesOff, i.Enabled))
	}
	if i.MaxRows < 1 {
		errs = append(errs, fmt.Errorf("images.max_rows: must be at least 1, got %d", i.MaxRows))
	}
	for _, h := range i.TrustedHosts {
		if err := checkTrustedHost(h); err != nil {
			errs = append(errs, fmt.Errorf("images.trusted_hosts: %q %w", h, err))
		}
	}
	return errors.Join(errs...)
}

// checkTrustedHost says why h is not a host, as example.com, or a wildcard
// of the subdomains of one, as *.example.com.
func checkTrustedHost(h string) error {
	name := h
	wildcard := strings.HasPrefix(h, "*.")
	if wildcard {
		name = h[2:]
	}
	switch {
	case h == "":
		return errors.New("is empty")
	case strings.Contains(h, "://"):
		return errors.New("must be a host without a scheme")
	case strings.ContainsAny(name, "/?#@"):
		return errors.New("must be a host without a path")
	case strings.Contains(name, ":") || strings.ContainsAny(name, "[]"):
		return errors.New("must be a host without a port")
	case strings.Contains(name, "*"):
		return errors.New("may use * only as the first label, as in *.example.com")
	case strings.ContainsAny(name, " \t") || name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.Contains(name, ".."):
		return errors.New("is not a host name")
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return errors.New("must be a host name, not an address")
	}
	if wildcard && !strings.Contains(name, ".") {
		return errors.New("must name a domain below the top level, as *.example.com")
	}
	return nil
}
