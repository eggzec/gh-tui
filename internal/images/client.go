package images

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// Limits of the client.
const (
	// timeout bounds a whole fetch, redirects and body included.
	timeout = 10 * time.Second
	// maxRedirects is how many redirects a fetch follows.
	maxRedirects = 3
)

var errRedirects = errors.New("too many redirects")

// newClient returns a client that sends requests through base only to
// the addresses h allows, on every redirect too, and never with
// credentials: it has no cookie jar, and refuses a request that carries
// any.
func newClient(base http.RoundTripper, h hosts) *http.Client {
	return &http.Client{
		Transport: guard{base: base, hosts: h, limit: new(limiter)},
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errRedirects
			}
			if !h.redirect(via[len(via)-1].URL, req.URL) {
				return fmt.Errorf("%w: redirect to %s", ErrNotAllowed, req.URL.Host)
			}
			return nil
		},
	}
}

// newTransport returns the transport of the client: the default one's
// settings, with connections of its own, which refuse a private address
// when they go to a host the user trusts.
func newTransport(h hosts) http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = maxFetches
	t.DialContext = h.dialer().DialContext
	return t
}

var errPrivate = errors.New("private address refused")

// dialer returns a dialer that refuses loopback, private, link-local and
// unspecified addresses for a trusted host, and dials any other as the
// default dialer does.
func (h hosts) dialer() *trustedDialer {
	return &trustedDialer{
		hosts: h,
		// plain mirrors the dialer of http.DefaultTransport.
		plain: &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second},
		guarded: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   refusePrivate,
		},
	}
}

type trustedDialer struct {
	hosts          hosts
	plain, guarded *net.Dialer
}

func (d *trustedDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err == nil && d.hosts.guarded(host) {
		return d.guarded.DialContext(ctx, network, addr)
	}
	return d.plain.DialContext(ctx, network, addr)
}

// refusePrivate is a dialer's Control hook: it sees the resolved address,
// so a name that resolves, or a redirect that leads, to a private one is
// refused too.
func refusePrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("%w: %s", errPrivate, ip)
	}
	return nil
}

// credentials are the headers a request of the client must never carry.
var credentials = []string{"Authorization", "Proxy-Authorization", "Cookie"}

// guard checks each request, the first and each redirect, before base
// sends it, so no path through the client reaches another host or sends
// credentials, and spaces the requests to each host.
type guard struct {
	base  http.RoundTripper
	hosts hosts
	limit *limiter
}

func (g guard) RoundTrip(req *http.Request) (*http.Response, error) {
	// A redirect carries the response that caused it, whose request is
	// where it came from.
	var from *url.URL
	if req.Response != nil && req.Response.Request != nil {
		from = req.Response.Request.URL
	}
	if !g.hosts.redirect(from, req.URL) {
		closeBody(req)
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, req.URL.Host)
	}
	for _, h := range credentials {
		if req.Header.Get(h) != "" {
			closeBody(req)
			return nil, fmt.Errorf("image request with %s refused", h)
		}
	}
	if err := g.limit.wait(req.Context(), req.URL.Host); err != nil {
		closeBody(req)
		return nil, err
	}
	return g.base.RoundTrip(req)
}

// closeBody closes the body of a request that won't be sent, as a
// RoundTripper must.
func closeBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}
