package github

import (
	"context"
	"log/slog"

	"github.com/eggzec/gh-tui/internal/core"
)

// enterpriseHeader is the header in which a GitHub Enterprise Server tells
// its version, such as 3.17.4, in every answer. github.com sends none.
const enterpriseHeader = "X-GitHub-Enterprise-Version"

// WithOnOldEnterprise sets a function that is told, once, the version of
// the GitHub Enterprise Server the client talks to when it is older than
// core.MinEnterprise, as the first answer that tells it says. It is called
// from the goroutine of the request and must not block.
func WithOnOldEnterprise(f func(version string)) Option {
	return func(o *options) { o.onOld = f }
}

// observeVersion learns from version, the enterpriseHeader of an answer of
// the host, that the host is an Enterprise Server, and tells onOld if it
// is older than supported.
func (c *Client) observeVersion(ctx context.Context, version string) {
	if version == "" {
		return
	}
	c.enterprise.Store(true)
	if core.EnterpriseSupported(version) || !c.toldOld.CompareAndSwap(false, true) {
		return
	}
	slog.WarnContext(ctx, "enterprise server unsupported", "span", "http", "version", version, "min", core.MinEnterprise)
	if c.onOld != nil {
		c.onOld(version)
	}
}
