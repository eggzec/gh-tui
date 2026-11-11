// Package releasesfake serves made-up releases, so the releases pane and modal
// can be tried without a network. The wiring turns it on with
// GH_TUI_FAKE_RELEASES=1.
package releasesfake

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// Enabled reports whether the fake releases are switched on.
func Enabled() bool { return os.Getenv("GH_TUI_FAKE_RELEASES") == "1" }

// API serves Releases for every repository, with the same paging as GitHub.
// It satisfies the service's API.
type API struct{}

const baseURL = "https://github.com/acme/widgets"

// GetRelease returns the fake release with the given ID.
func (API) GetRelease(_ context.Context, _ core.RepoRef, id int64, _ github.Conditional) (core.Release, github.Response, error) {
	all := Releases()
	for i := range all {
		if all[i].ID == id {
			return all[i], github.Response{}, nil
		}
	}
	return core.Release{}, github.Response{}, core.ErrNotFound
}

// ListReleases returns a page of the fake releases. The cursor is the index
// of the first release of the page.
func (API) ListReleases(_ context.Context, _ core.RepoRef, cursor string, perPage int, _ github.Conditional) (core.Page[core.Release], github.Response, error) {
	all := Releases()
	if perPage <= 0 {
		perPage = len(all)
	}
	start := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 || n > len(all) {
			return core.Page[core.Release]{}, github.Response{}, core.ErrNotFound
		}
		start = n
	}
	end := min(start+perPage, len(all))
	page := core.Page[core.Release]{Items: all[start:end]}
	if end < len(all) {
		page.Next = strconv.Itoa(end)
	}
	return page, github.Response{}, nil
}

// LatestRelease returns the release marked latest.
func (API) LatestRelease(context.Context, core.RepoRef) (core.Release, bool, error) {
	all := Releases()
	for i := range all {
		if all[i].Latest {
			return all[i], true, nil
		}
	}
	return core.Release{}, false, nil
}

// ReleaseAssetURL returns the public address of the asset.
func (API) ReleaseAssetURL(_ context.Context, _ core.RepoRef, assetID int64) (string, error) {
	all := Releases()
	for i := range all {
		for _, a := range all[i].Assets {
			if a.ID == assetID {
				return a.URL, nil
			}
		}
	}
	return "", core.ErrNotFound
}

// CommitOf returns a verified commit for any ref.
func (API) CommitOf(_ context.Context, _ core.RepoRef, _ string, _ github.Conditional) (core.Commit, github.Response, error) {
	return core.Commit{SHA: "0123456789abcdef0123456789abcdef01234567", Verification: core.Verification{Verified: true}}, github.Response{}, nil
}

// Compare returns a branch a few commits ahead of any tag.
func (API) Compare(context.Context, core.RepoRef, string, string, github.Conditional) (core.Compare, github.Response, error) {
	return core.Compare{Status: core.CompareAhead, AheadBy: 7}, github.Response{}, nil
}

func asset(id int64, tag, name string, size int64, downloads int) core.ReleaseAsset {
	return core.ReleaseAsset{
		ID: id, Name: name, Size: size, Downloads: downloads,
		URL: fmt.Sprintf("%s/releases/download/%s/%s", baseURL, tag, name),
	}
}

func platforms(base int64, tag string, n int) []core.ReleaseAsset {
	all := []struct {
		name string
		size int64
		dl   int
	}{
		{"widgets_linux_amd64.tar.gz", 8_420_113, 12840},
		{"widgets_darwin_arm64.tar.gz", 8_101_227, 9310},
		{"widgets_windows_amd64.zip", 8_877_640, 4120},
		{"widgets_linux_arm64.tar.gz", 7_998_002, 2210},
		{"checksums.txt", 612, 1830},
		{"widgets.sbom.json", 142_880, 96},
	}
	var out []core.ReleaseAsset
	for i := 0; i < n && i < len(all); i++ {
		a := all[i]
		out = append(out, asset(base+int64(i), tag, a.name, a.size, a.dl))
	}
	return out
}

// Releases returns the fake releases, newest first.
func Releases() []core.Release {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour
	bot := core.User{Login: "github-actions[bot]", Bot: true}
	ana := core.User{Login: "anaperez", Name: "Ana Perez"}
	liu := core.User{Login: "liuwei", Name: "Wei Liu"}
	sam := core.User{Login: "samokafor", Name: "Sam Okafor"}

	mk := func(id int64, tag, name string, by core.User, body string, age time.Duration, assets int) core.Release {
		return core.Release{
			ID: id, Tag: tag, Name: name, Author: by, Body: body,
			URL:         baseURL + "/releases/tag/" + tag,
			CreatedAt:   ago(age + 2*time.Hour),
			PublishedAt: ago(age),
			Assets:      platforms(id*10, tag, assets),
			Target:      "main",
		}
	}

	notes := func(title string) string {
		return "## " + title + "\n\n" +
			"This release focuses on **stability** and a faster startup.\n\n" +
			"### Highlights\n\n" +
			"- Faster cold start: config is parsed once, lazily\n" +
			"- New `--dry-run` flag for `widgets sync`\n" +
			"- Fixed a crash when the cache directory is read-only\n\n" +
			"### Upgrade\n\n" +
			"```sh\nwidgets self-update --to latest\n```\n\n" +
			"If you pin the version in CI, change it to the tag above.\n\n" +
			"### Breaking changes\n\n" +
			"1. `sync.interval` now takes a duration such as `30s`\n" +
			"2. The `legacy` output format was removed\n\n" +
			"**Full changelog**: " + baseURL + "/compare/previous-this\n"
	}

	latest := mk(1008, "v2.4.0", "Widgets 2.4.0", ana, notes("Widgets 2.4.0"), 3*day, 6)
	latest.Latest = true
	latest.Target = "a1b2c3d"
	pre := mk(1007, "v2.5.0-rc.1", "Widgets 2.5.0 release candidate 1", liu, notes("Widgets 2.5.0-rc.1")+"\n> Release candidate: please report problems.\n", 1*day, 3)
	pre.Prerelease = true
	pre.Target = "release/2.5"
	draft := mk(1006, "v2.4.1", "Widgets 2.4.1", sam, "## Widgets 2.4.1\n\n- Draft notes, not final\n- TODO: credit contributors\n", 0, 2)
	draft.Draft = true
	draft.PublishedAt = time.Time{}
	draft.CreatedAt = ago(5 * time.Hour)

	return []core.Release{
		pre,
		draft,
		latest,
		mk(1005, "v2.3.2", "Widgets 2.3.2", bot, notes("Widgets 2.3.2"), 12*day, 4),
		mk(1004, "v2.3.1", "Widgets 2.3.1", ana, notes("Widgets 2.3.1"), 31*day, 5),
		mk(1003, "v2.3.0", "Widgets 2.3.0", liu, notes("Widgets 2.3.0"), 62*day, 6),
		mk(1002, "v2.2.0", "Widgets 2.2.0", sam, notes("Widgets 2.2.0"), 118*day, 4),
		mk(1001, "v2.0.0", "Widgets 2.0.0", ana, notes("Widgets 2.0.0"), 240*day, 2),
	}
}
