package github

import (
	"context"
	"fmt"
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
	Author      user      `json:"author"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
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
		Author:      r.Author.core(),
		Body:        r.Body,
		URL:         r.HTMLURL,
		Draft:       r.Draft,
		Prerelease:  r.Prerelease,
		CreatedAt:   r.CreatedAt,
		PublishedAt: r.PublishedAt,
	}
	for _, a := range r.Assets {
		rel.Assets = append(rel.Assets, core.ReleaseAsset{Name: a.Name, Size: a.Size, Downloads: a.DownloadCount, URL: a.URL})
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
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/releases/" + strconv.FormatInt(id, 10)
}
