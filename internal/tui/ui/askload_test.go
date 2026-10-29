package ui

import (
	"errors"
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestAskLoad(t *testing.T) {
	t.Parallel()
	mib := int64(config.MiB)
	q, ok := AskLoad("big.txt", fmt.Errorf("get: %w", &core.TooLargeError{Size: 5 * mib / 2, Limit: 2 * mib, Warn: true}))
	want := "big.txt is 2.5 MiB, over the 2 MiB warning limit (files.preview.warn_size). Load it anyway?"
	if !ok || q != want {
		t.Errorf("AskLoad = %q, %v; want %q", q, ok, want)
	}
	if _, ok := AskLoad("big.txt", &core.TooLargeError{Size: 30 * mib, Limit: 20 * mib}); ok {
		t.Error("AskLoad asked about a file over the hard limit")
	}
	if _, ok := AskLoad("big.txt", errors.New("other")); ok {
		t.Error("AskLoad asked about another error")
	}
}
