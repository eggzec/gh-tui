package core

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestLooksBinary(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", nil, false},
		{"text", []byte("package main\n"), false},
		{"utf-8", []byte("héllo wörld ✓"), false},
		{"nul", []byte("PNG\x00\x01"), true},
		{"nul past the sniffed prefix", append(bytes.Repeat([]byte("a"), binarySniffLen), 0), false},
		{"nul at the sniffed edge", append(bytes.Repeat([]byte("a"), binarySniffLen-1), 0), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksBinary(tt.in); got != tt.want {
				t.Errorf("LooksBinary = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTooLargeError(t *testing.T) {
	err := fmt.Errorf("get blob: %w", &TooLargeError{Size: 2 << 20, Limit: 1 << 20})
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("errors.Is(%v, ErrTooLarge) = false", err)
	}
	if e, ok := errors.AsType[*TooLargeError](err); !ok || e.Size != 2<<20 {
		t.Errorf("AsType = %v, %v; want the size", e, ok)
	}
	if got := (&TooLargeError{Limit: 10}).Error(); got != "larger than 10 bytes" {
		t.Errorf("unknown size message = %q", got)
	}
}

func TestTreeEntryKinds(t *testing.T) {
	dir := TreeEntry{Type: EntryTree, Mode: "040000"}
	sub := TreeEntry{Type: EntryCommit, Mode: "160000"}
	link := TreeEntry{Type: EntryBlob, Mode: ModeSymlink}
	if !dir.Dir() || dir.Submodule() || dir.Symlink() {
		t.Errorf("tree entry kinds wrong: %+v", dir)
	}
	if sub.Dir() || !sub.Submodule() {
		t.Errorf("submodule kinds wrong: %+v", sub)
	}
	if link.Dir() || !link.Symlink() {
		t.Errorf("symlink kinds wrong: %+v", link)
	}
}
