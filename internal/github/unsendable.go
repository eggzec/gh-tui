package github

import (
	"errors"
	"net/http"
	"strings"
)

// unsendable reports whether err says that the request can never be sent
// as it is, so that sending it again fails the same way: a header or a
// method the transport refuses, a URL it can't send to, or a proxy that
// wants credentials. net/http says so only in words, so it looks at the
// error that err wraps, through a chain of single wraps. An errors.Join
// or an fmt.Errorf with several %w wraps more than one error and stops
// the walk there, which only means one more retry.
func unsendable(err error) bool {
	for u := errors.Unwrap(err); u != nil; u = errors.Unwrap(err) {
		err = u
	}
	msg := err.Error()
	if msg == http.StatusText(http.StatusProxyAuthRequired) {
		return true
	}
	for _, p := range []string{"net/http: invalid ", "unsupported protocol scheme", "http: no Host in request URL"} {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}
