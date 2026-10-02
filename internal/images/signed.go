package images

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sync/singleflight"
)

// HTML returns the HTML GitHub renders of the bodies named by their node
// IDs, such as comments, by ID. It is the only part of a fetch that may
// count against the API's rate limit, and is called for a body only when
// one of its images needs it: an attachment of a private repository, whose
// address GitHub signs for a few minutes, or an image on another host,
// which loads only through the address of GitHub's proxy.
type HTML func(ctx context.Context, ids []string) (map[string]string, error)

// Timing of signed addresses.
const (
	// expiryMargin is how long before it expires a signed address is
	// taken for expired, so it isn't sent to expire on the way.
	expiryMargin = 30 * time.Second
	// unknownExpiry is how long a signed address whose expiry can't be
	// read is used.
	unknownExpiry = time.Minute
	// maxBodies is how many bodies' images the signer keeps at most.
	maxBodies = 256
	// bodyFor is how long what was read of a body's HTML is used at most,
	// shorter than the five minutes or so GitHub signs addresses for, so
	// a body edited since, or an address about to expire, is read anew.
	bodyFor = 4 * time.Minute
)

// img is an image of a body's HTML: the address it loads from, and the one
// the markdown named, which GitHub keeps beside a proxied address.
type img struct {
	src, canonical string
	// expires is when the address is read again: when its signature
	// expires, less a margin, or once bodyFor has passed.
	expires time.Time
}

// signer finds where GitHub serves an image of a body, from the body's
// HTML, and keeps what it read until the addresses in it expire.
type signer struct {
	html HTML
	now  func() time.Time

	reads  singleflight.Group // by body
	mu     sync.Mutex
	bodies map[string][]img
}

// url returns where to fetch the image stable names, the index-th of
// body, and whether it was found only by its place, which says nothing of
// what it is. It reads the body's HTML when what it kept of it has
// expired, or when the host refused the address refused, unless the HTML
// was read again since and gives another. Images of the same body share
// one read.
func (s *signer) url(ctx context.Context, body, stable string, index int, refused string) (addr string, placed bool, err error) {
	if s.html == nil {
		return "", false, fmt.Errorf("%w: no rendered HTML to find %s in", ErrNotAllowed, stable)
	}
	s.mu.Lock()
	imgs, ok := s.bodies[body]
	s.mu.Unlock()
	if ok {
		i, found, placed := match(stable, index, imgs)
		fresh := s.now().Before(i.expires)
		if found && (refused == "" && fresh || refused != "" && i.src != refused) {
			return i.src, placed, nil
		}
	}
	imgs, err = s.read(ctx, body)
	if err != nil {
		return "", false, err
	}
	i, found, placed := match(stable, index, imgs)
	if !found {
		return "", false, fmt.Errorf("%w: %s isn't in the body's html", ErrUnavailable, stable)
	}
	// Just read, so an address that seems expired is tried anyway: the
	// clocks may differ.
	return i.src, placed, nil
}

// read reads the images of body's HTML and keeps them. Callers of the
// same body share one read, which no one caller's leaving cancels.
func (s *signer) read(ctx context.Context, body string) ([]img, error) {
	ch := s.reads.DoChan(body, func() (any, error) {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flightTimeout)
		defer cancel()
		got, err := s.html(rctx, []string{body})
		if err != nil {
			return nil, err
		}
		imgs := images(got[body], s.now())
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.bodies == nil || len(s.bodies) >= maxBodies {
			s.bodies = make(map[string][]img)
		}
		s.bodies[body] = imgs
		return imgs, nil
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, fmt.Errorf("read body html: %w", r.Err)
		}
		return r.Val.([]img), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// images returns the images of a body's HTML, in order.
func images(doc string, now time.Time) []img {
	var imgs []img
	z := html.NewTokenizer(strings.NewReader(doc))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return imgs
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data != "img" {
				continue
			}
			var (
				i     img
				emoji bool
			)
			for _, a := range t.Attr {
				switch a.Key {
				case "src":
					i.src = a.Val
				case "data-canonical-src":
					i.canonical = a.Val
				case "class":
					emoji = slices.Contains(strings.Fields(a.Val), "emoji")
				}
			}
			// An emoji isn't an image of the markdown's, so it takes no
			// place among them.
			if i.src == "" || emoji {
				continue
			}
			i.expires = expiry(i.src, now)
			if i.expires.IsZero() || i.expires.After(now.Add(bodyFor)) {
				i.expires = now.Add(bodyFor)
			}
			imgs = append(imgs, i)
		default:
		}
	}
}

// assetID finds the ID of an attachment in its address, which the signed
// address keeps: github.com/user-attachments/assets/<id> and
// private-user-images.githubusercontent.com/<user>/<n>-<id>.png both hold it.
var assetID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// match finds the image of imgs that stable names, the index-th of its
// body: by the address the markdown named, which a proxied image keeps,
// then by the ID of an attachment, then by its place, which placed
// reports.
func match(stable string, index int, imgs []img) (found img, ok, placed bool) {
	for _, i := range imgs {
		if i.canonical == stable {
			return i, true, false
		}
	}
	if id := assetID.FindString(stable); id != "" {
		for _, i := range imgs {
			if u, err := url.Parse(i.src); err == nil && strings.Contains(u.Path, id) {
				return i, true, false
			}
		}
		// An attachment named by its ID is found by it or not at all.
		return img{}, false, false
	}
	if index >= 0 && index < len(imgs) && imgs[index].canonical == "" {
		return imgs[index], true, true
	}
	return img{}, false, false
}

// expiry returns when a signed address expires, less a margin, or zero
// for one that isn't signed. It reads the exp of a GitHub JWT and the
// date and lifetime of an S3 signature. One it can't read expires soon.
func expiry(addr string, now time.Time) time.Time {
	u, err := url.Parse(addr)
	if err != nil {
		return now
	}
	q := u.Query()
	if jwt := q.Get("jwt"); jwt != "" {
		if exp, ok := jwtExpiry(jwt); ok {
			return exp.Add(-expiryMargin)
		}
		return now.Add(unknownExpiry)
	}
	if date := q.Get("X-Amz-Date"); date != "" {
		t, err := time.Parse("20060102T150405Z", date)
		secs, err2 := strconv.Atoi(q.Get("X-Amz-Expires"))
		if err != nil || err2 != nil {
			return now.Add(unknownExpiry)
		}
		return t.Add(time.Duration(secs)*time.Second - expiryMargin)
	}
	return time.Time{}
}

// jwtExpiry reads the exp claim of a JWT, without checking its signature:
// it only says when to ask for a new one.
func jwtExpiry(jwt string) (time.Time, bool) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
