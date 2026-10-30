package search

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// CodePreviews reads the file a code hit matched, by its blob.
type CodePreviews interface {
	// Blob returns the content of a file. A file over the preview size limit
	// fails with an error that matches core.ErrTooLarge.
	Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error)
}

// CodeReader returns what reads the whole file of a code hit, or nil when the
// hit is all its preview has.
type CodeReader func(h core.CodeHit) func(ctx context.Context) (string, error)

// WithCodeReader gives the preview of a code hit the whole file, read lazily
// like the other previews.
func WithCodeReader(r CodeReader) Option {
	return func(s *Section) { s.codeReader = r }
}

// errBinary tells that the file of a code hit is not text.
var errBinary = errors.New("binary file")

// NewCodeReader returns the CodeReader that reads files from svc. A nil svc
// gives no reader, so a code hit keeps its fragments.
func NewCodeReader(svc CodePreviews) CodeReader {
	if svc == nil {
		return nil
	}
	return func(h core.CodeHit) func(ctx context.Context) (string, error) {
		q := filesvc.BlobQuery{Repo: h.Repo, SHA: h.SHA}
		if h.SHA == "" {
			return nil
		}
		return func(ctx context.Context) (string, error) {
			b, err := svc.Blob(ctx, q)
			switch {
			case err != nil:
				return "", err
			case b.Binary:
				return "", errBinary
			}
			return string(b.Content), nil
		}
	}
}

// codeFileKey identifies the file of a code hit.
func codeFileKey(h core.CodeHit) string {
	return "code " + h.Repo.String() + ":" + h.Path + "@" + h.SHA
}

// codeBody returns the preview of a code hit: its fragments at once, then the
// whole file once read, at its first line that holds the query, or why the
// file is not shown.
func (s *Section) codeBody(h core.CodeHit) previewBody {
	b := previewBody{key: codeFileKey(h) + " " + s.text, name: h.Path, find: s.text}
	own := func() string { return codeText(h) }
	b.text = own
	if s.codeReader == nil || s.codeReader(h) == nil {
		return b
	}
	e, ok := s.extras[codeFileKey(h)]
	switch {
	case !ok:
		b.key += " loading"
		b.text = func() string { return own() + "\nLoading more" + s.icons.Ellipsis + "\n" }
	case errors.Is(e.err, core.ErrTooLarge):
		b.key += " large"
		b.text, b.find = nil, ""
		b.message = ui.TooLargeText(h.Path, e.err) + s.browserHint()
	case errors.Is(e.err, errBinary):
		b.key += " binary"
		b.text, b.find = nil, ""
		b.message = "Binary file, not shown" + s.browserHint()
	case e.err != nil:
		b.key += " failed"
		b.text = func() string { return own() + "\nCould not load the file: " + e.err.Error() + "\n" }
	default:
		b.key += " loaded"
		b.text = func() string { return e.text }
		b.line = firstMatchLine(e.text, s.text)
	}
	return b
}

// browserHint names the key that opens the hit in the browser.
func (s *Section) browserHint() string {
	if k := s.keys.Open.Help().Key; k != "" && s.keys.Open.Enabled() {
		return s.icons.Separator + s.icons.Key(k) + " opens it in the browser"
	}
	return ""
}

// firstMatchLine returns the number of the first line of text that holds query,
// ignoring case, or 1 if none does.
func firstMatchLine(text, query string) int {
	if query == "" {
		return 1
	}
	query = strings.ToLower(query)
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(line), query) {
			return i + 1
		}
	}
	return 1
}

// readCode returns the read of the file of h, unless it needs none or was
// read or is being read.
func (s *Section) readCode(h core.CodeHit) tea.Cmd {
	if s.codeReader == nil {
		return nil
	}
	return s.begin(codeFileKey(h), s.codeReader(h))
}
