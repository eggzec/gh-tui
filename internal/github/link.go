package github

import "strings"

// parseLinks maps each rel in a Link header to its target, as in
//
//	<https://api.github.com/repositories/1/pulls?page=2>; rel="next", <…>; rel="last"
//
// Targets are split on angle brackets rather than commas because query
// strings may contain commas.
func parseLinks(header string) map[string]string {
	links := make(map[string]string)
	for {
		start := strings.IndexByte(header, '<')
		if start < 0 {
			return links
		}
		end := strings.IndexByte(header[start:], '>')
		if end < 0 {
			return links
		}
		target := header[start+1 : start+end]
		header = header[start+end+1:]

		params := header
		if i := strings.IndexByte(header, '<'); i >= 0 {
			params, header = header[:i], header[i:]
		}
		for p := range strings.SplitSeq(params, ";") {
			key, value, ok := strings.Cut(strings.Trim(p, " ,"), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "rel") {
				continue
			}
			for rel := range strings.FieldsSeq(strings.Trim(strings.TrimSpace(value), `"`)) {
				links[rel] = target
			}
		}
	}
}
