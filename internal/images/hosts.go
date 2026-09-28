package images

import (
	"net/url"
	"path"
	"strings"
)

// github is the web host of github.com, whose images live on hosts of
// their own.
const github = "github.com"

// githubHosts are the hosts of github.com's images, from which any path
// may be fetched.
var githubHosts = map[string]bool{
	"avatars.githubusercontent.com":             true,
	"camo.githubusercontent.com":                true,
	"private-user-images.githubusercontent.com": true,
	"user-images.githubusercontent.com":         true,
	"raw.githubusercontent.com":                 true,
}

// Where attachments are stored for github.com: the buckets its
// attachment addresses redirect to. Anyone can create a bucket of such a
// name, so one is fetched only as the target of a redirect from GitHub,
// never as an address a body names.
const (
	bucketPrefix = "github-production-user-asset-"
	bucketSuffix = ".s3.amazonaws.com"
)

// attachments is where github.com serves attachments under its own host;
// older ones are below a repository's, /<owner>/<repo>/assets/.
const attachments = "/user-attachments/assets/"

// enterprisePaths are where an Enterprise Server serves images under its
// own host, without subdomain isolation.
var enterprisePaths = []string{"/avatars/", "/storage/", "/user-attachments/", "/raw/"}

// enterpriseSubdomains are the hosts below an Enterprise Server's own that
// serve images, with subdomain isolation.
var enterpriseSubdomains = []string{"avatars.", "media.", "raw."}

// hosts says which addresses images may be fetched from: those of the
// GitHub that web, its web host, names, and no other.
type hosts struct {
	web string
}

func newHosts(web string) hosts {
	return hosts{web: hostPort(strings.ToLower(web), "")}
}

// allowed reports whether u may be fetched first: https, no user, and a
// host and path where web's GitHub serves images.
func (h hosts) allowed(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return false
	}
	host := hostPort(strings.ToLower(u.Hostname()), u.Port())
	if h.web == github {
		switch {
		case githubHosts[host]:
			return true
		case host == github:
			return githubAttachment(u)
		}
		return false
	}
	if host == h.web {
		for _, p := range enterprisePaths {
			if underPath(u, p) {
				return true
			}
		}
		return false
	}
	for _, sub := range enterpriseSubdomains {
		if host == sub+h.web {
			return true
		}
	}
	return false
}

// redirect reports whether a redirect from from to u may be followed:
// to an address allowed first, or from one of github.com's attachments to
// the bucket it is stored in.
func (h hosts) redirect(from, u *url.URL) bool {
	if h.allowed(u) {
		return true
	}
	if h.web != github || from == nil || !h.allowed(from) || !h.attachment(from) {
		return false
	}
	return u.Scheme == "https" && u.User == nil && u.Opaque == "" &&
		bucket(hostPort(strings.ToLower(u.Hostname()), u.Port()))
}

// githubAttachment reports whether u, on github.com, is the address of an
// attachment: /user-attachments/assets/<id>, or /<owner>/<repo>/assets/<id>.
func githubAttachment(u *url.URL) bool {
	if underPath(u, attachments) {
		return true
	}
	parts := strings.SplitN(u.EscapedPath(), "/", 5)
	return len(parts) == 5 && parts[0] == "" && parts[1] != "" && parts[2] != "" && parts[3] == "assets" &&
		underPath(u, "/"+parts[1]+"/"+parts[2]+"/assets/")
}

// attachment reports whether u names an attachment by an address that
// only the web host can sign, which it does in the rendered HTML of a
// private repository's bodies.
func (h hosts) attachment(u *url.URL) bool {
	host := hostPort(strings.ToLower(u.Hostname()), u.Port())
	if h.web == github {
		return host == github && githubAttachment(u) ||
			host == "user-images.githubusercontent.com" || host == "private-user-images.githubusercontent.com"
	}
	return host == h.web && (underPath(u, "/storage/") || underPath(u, "/user-attachments/")) ||
		host == "media."+h.web
}

// bucket reports whether host is one of github.com's attachment buckets:
// the prefix, a name of letters and digits, and the suffix.
func bucket(host string) bool {
	name, ok := strings.CutPrefix(host, bucketPrefix)
	if !ok {
		return false
	}
	name, ok = strings.CutSuffix(name, bucketSuffix)
	if !ok || name == "" {
		return false
	}
	for _, c := range []byte(name) {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// underPath reports whether u's path is below prefix, with no dot
// segments or escapes that could lead out of it.
func underPath(u *url.URL, prefix string) bool {
	p := u.EscapedPath()
	if p != u.Path || !strings.HasPrefix(p, prefix) {
		return false
	}
	return path.Clean(p) == p && len(p) > len(prefix)
}

// hostPort joins host and port, leaving out the default port of https.
func hostPort(host, port string) string {
	if h, p, ok := strings.Cut(host, ":"); ok && !strings.Contains(p, ":") {
		host, port = h, p
	}
	if port == "" || port == "443" {
		return host
	}
	return host + ":" + port
}
