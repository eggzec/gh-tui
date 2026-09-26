package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Code search has no GraphQL equivalent. It has a rate limit of its own,
// code_search, of 10 requests a minute, so it runs only when asked for.
// The text-match media type adds the fragments of each file that matched.

// codeSearchAccept asks for text matches along with the results.
const codeSearchAccept = "application/vnd.github.text-match+json"

// codeSearchResults is the body of a code search response.
type codeSearchResults struct {
	TotalCount        int             `json:"total_count"`
	IncompleteResults bool            `json:"incomplete_results"`
	Items             []codeSearchHit `json:"items"`
}

// codeSearchHit is the REST shape of a file in code search results.
type codeSearchHit struct {
	Path       string `json:"path"`
	SHA        string `json:"sha"`
	HTMLURL    string `json:"html_url"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	TextMatches []codeTextMatch `json:"text_matches"`
}

// codeTextMatch is a fragment of a file with its matches. Indices are byte
// offsets into Fragment.
type codeTextMatch struct {
	Fragment string `json:"fragment"`
	Matches  []struct {
		Indices [2]int `json:"indices"`
	} `json:"matches"`
}

func (h codeSearchHit) core() core.CodeHit {
	// GitHub is the authority on its own names, so they are taken as they
	// come rather than checked by core.ParseRepoRef.
	owner, name, _ := strings.Cut(h.Repository.FullName, "/")
	return core.CodeHit{
		Repo:      core.RepoRef{Owner: owner, Name: name},
		Path:      h.Path,
		SHA:       h.SHA,
		URL:       h.HTMLURL,
		Fragments: convert(h.TextMatches, codeTextMatch.core),
	}
}

// core maps the fragment, dropping any match that doesn't fit in it, so
// that slicing the text by a match is always safe.
func (m codeTextMatch) core() core.Fragment {
	f := core.Fragment{Text: m.Fragment}
	for _, mm := range m.Matches {
		start, end := mm.Indices[0], mm.Indices[1]
		if start < 0 || start > end || end > len(m.Fragment) {
			continue
		}
		f.Matches = append(f.Matches, [2]int{start, end})
	}
	return f
}

// SearchCode returns a page of the files that match query, best match
// first, each with the fragments that matched. Query takes GitHub's code
// search syntax, qualifiers such as repo:, language: or path: included.
// Cursor is the Next of the previous page, or empty for the first. PerPage
// sizes the first page, and 0 leaves the size to GitHub; later pages keep
// the size of the page their cursor came from.
//
// Running out of the code search limit fails with a *core.RateLimitError,
// and a query GitHub refuses with a *core.InvalidQueryError.
func (c *Client) SearchCode(ctx context.Context, query, cursor string, perPage int) (core.SearchPage[core.CodeHit], error) {
	path := cursor
	if path == "" {
		q := url.Values{"q": {query}}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		path = "search/code?" + q.Encode()
	}
	var body codeSearchResults
	res, err := c.codeSearchGet(ctx, path, codeSearchAccept, &body)
	if err != nil {
		return core.SearchPage[core.CodeHit]{}, fmt.Errorf("search code: %w", codeSearchError(err))
	}
	return core.SearchPage[core.CodeHit]{
		Items: convert(body.Items, codeSearchHit.core), Next: res.Next,
		Total:      body.TotalCount,
		Incomplete: body.IncompleteResults,
	}, nil
}

// codeSearchGet is Get for a media type other than JSON.
func (c *Client) codeSearchGet(ctx context.Context, path, accept string, v any) (Response, error) {
	u, err := c.resolve(path)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Accept", accept)
	resp, err := c.send(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	res := newResponse(resp)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return res, c.httpError(resp)
	}
	return res, decode(resp.Body, v)
}

// codeSearchError turns the 422 GitHub sends for a query it can't run into a
// *core.InvalidQueryError, which reads better than a validation failure.
func codeSearchError(err error) error {
	e, ok := errors.AsType[*Error](err)
	if !ok || e.StatusCode != http.StatusUnprocessableEntity {
		return err
	}
	reason := e.Message
	// "Validation Failed (…)" says nothing its details don't.
	if _, details, ok := strings.Cut(reason, " ("); ok {
		reason = strings.TrimSuffix(details, ")")
	}
	return &core.InvalidQueryError{Reason: reason}
}
