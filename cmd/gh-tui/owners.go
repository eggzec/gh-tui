package main

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	ownersvc "github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui"
)

// ownerHeaders reads the headers of users and organizations for goto,
// which opens the page of one only once it is known to exist.
type ownerHeaders struct {
	svc *ownersvc.Service
}

var _ tui.Owners = ownerHeaders{}

func (o ownerHeaders) CachedHeader(login string) (core.Owner, bool) {
	return o.svc.CachedHeader(login)
}

func (o ownerHeaders) Header(ctx context.Context, login string) (core.Owner, error) {
	return o.svc.Header(ctx, ownersvc.HeaderQuery{Login: login})
}
