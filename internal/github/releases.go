package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Releases are read with REST, which names them by ID as notifications do,
// and revalidates them for free with their ETag.

type restRelease struct {
	ID          int64     `json:"id"`
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Target      string    `json:"target_commitish"`
	Author      user      `json:"author"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Size          int64  `json:"size"`
		DownloadCount int    `json:"download_count"`
		URL           string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r restRelease) core() core.Release {
	rel := core.Release{
		ID:          r.ID,
		Tag:         r.TagName,
		Name:        r.Name,
		Target:      r.Target,
		Author:      r.Author.core(),
		Body:        r.Body,
		URL:         r.HTMLURL,
		Draft:       r.Draft,
		Prerelease:  r.Prerelease,
		CreatedAt:   r.CreatedAt,
		PublishedAt: r.PublishedAt,
	}
	for _, a := range r.Assets {
		rel.Assets = append(rel.Assets, core.ReleaseAsset{ID: a.ID, Name: a.Name, Size: a.Size, Downloads: a.DownloadCount, URL: a.URL})
	}
	return rel
}

// GetRelease returns the release id of repo. If cond is current, the
// Response has NotModified set and the release is empty.
func (c *Client) GetRelease(ctx context.Context, repo core.RepoRef, id int64, cond Conditional) (core.Release, Response, error) {
	var r restRelease
	res, err := c.Get(ctx, releasePath(repo, id), cond, &r)
	if err != nil {
		return core.Release{}, res, fmt.Errorf("get release %d of %s: %w", id, repo, err)
	}
	if res.NotModified {
		return core.Release{}, res, nil
	}
	return r.core(), res, nil
}

func releasePath(repo core.RepoRef, id int64) string {
	return releasesPath(repo) + "/" + strconv.FormatInt(id, 10)
}

func releasesPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/releases"
}

// ListReleases returns a page of the releases of repo, newest first. Cursor
// is the Next of the previous page, or empty for the first. PerPage sizes the
// first page, and 0 leaves the size to GitHub; later pages keep the size of
// the page their cursor came from. If cond is current, the Response has
// NotModified set and the page is empty.
func (c *Client) ListReleases(ctx context.Context, repo core.RepoRef, cursor string, perPage int, cond Conditional) (core.Page[core.Release], Response, error) {
	path := cursor
	if path == "" {
		path = releasesPath(repo)
		if perPage > 0 {
			path += "?per_page=" + strconv.Itoa(perPage)
		}
	}
	var items []restRelease
	res, err := c.Get(ctx, path, cond, &items)
	if err != nil {
		return core.Page[core.Release]{}, res, fmt.Errorf("list releases of %s: %w", repo, err)
	}
	if res.NotModified {
		return core.Page[core.Release]{}, res, nil
	}
	page := core.Page[core.Release]{Next: res.Next}
	for i := range items {
		page.Items = append(page.Items, items[i].core())
	}
	return page, res, nil
}

// ReleaseAssetURL returns where GitHub serves the file assetID of the
// releases of repo from. The address is signed and short-lived, so it is
// returned unread by anything that logs, and the redirect is not followed:
// nothing of the request, the token above all, reaches the storage.
func (c *Client) ReleaseAssetURL(ctx context.Context, repo core.RepoRef, assetID int64) (string, error) {
	path := releasesPath(repo) + "/assets/" + strconv.FormatInt(assetID, 10)
	u, err := c.resolve(path)
	if err != nil {
		return "", fmt.Errorf("release file %d of %s: %w", assetID, repo, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("release file %d of %s: %w", assetID, repo, err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	hc := *c.http
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.sendWith(&hc, req)
	if err != nil {
		return "", fmt.Errorf("release file %d of %s: %w", assetID, repo, err)
	}
	defer resp.Body.Close()
	switch {
	case isRedirect(resp.StatusCode):
		loc, err := c.logLocation(resp)
		if err != nil {
			return "", fmt.Errorf("release file %d of %s: %w", assetID, repo, err)
		}
		return loc.String(), nil
	case resp.StatusCode >= http.StatusMultipleChoices:
		return "", fmt.Errorf("release file %d of %s: %w", assetID, repo, c.httpError(resp))
	}
	return "", fmt.Errorf("release file %d of %s: GitHub did not redirect (status %d)", assetID, repo, resp.StatusCode)
}
