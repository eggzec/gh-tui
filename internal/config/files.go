package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Files configures the Files pane.
type Files struct {
	Preview Preview `yaml:"preview" when:"startup" why:"the files are read with it from the start"`
	Finder  Finder  `yaml:"finder"`
	// Markdown is how the preview shows a markdown file, such as a
	// README: MarkdownRendered or MarkdownRaw, its source. The raw
	// command switches the file open between them.
	Markdown string `yaml:"markdown"`
}

// How the preview shows a markdown file.
const (
	MarkdownRendered = "rendered"
	MarkdownRaw      = "raw"
)

// Preview configures the file preview.
type Preview struct {
	// MaxSize is the largest file the preview reads. Larger files are
	// refused and better opened in the browser.
	MaxSize Size `yaml:"max_size"`
	// WarnSize is the size above which a file is large enough to ask
	// before it is read. It is at most MaxSize.
	WarnSize Size `yaml:"warn_size"`
}

// MaxSizeKey is the setting that limits the size of a file the preview reads.
const MaxSizeKey = "files.preview.max_size"

// TooLargeText says why a file of size bytes, 0 when not known, is refused
// at limit bytes, and which setting changes the limit.
func TooLargeText(name string, size, limit int64) string {
	if size <= 0 {
		return fmt.Sprintf("%s is over the %s limit (%s).", name, humanSize(limit), MaxSizeKey)
	}
	return fmt.Sprintf("%s is %s, over the %s limit (%s).", name, humanSize(size), humanSize(limit), MaxSizeKey)
}

// WarnSizeKey is the setting above which a file is read only once the user
// agrees.
const WarnSizeKey = "files.preview.warn_size"

// WarnText says that a file of size bytes, 0 when not known, is over the
// limit bytes it is read without asking, and which setting changes the limit.
func WarnText(name string, size, limit int64) string {
	if size <= 0 {
		return fmt.Sprintf("%s is over the %s warning limit (%s).", name, humanSize(limit), WarnSizeKey)
	}
	return fmt.Sprintf("%s is %s, over the %s warning limit (%s).", name, humanSize(size), humanSize(limit), WarnSizeKey)
}

// humanSize formats n in the largest binary unit it reaches, with one
// decimal when it isn't whole, such as "2.4 MiB".
func humanSize(n int64) string {
	for _, u := range []struct {
		size Size
		name string
	}{{GiB, "GiB"}, {MiB, "MiB"}, {KiB, "KiB"}} {
		if n >= int64(u.size) {
			s := strconv.FormatFloat(float64(n)/float64(u.size), 'f', 1, 64)
			return strings.TrimSuffix(s, ".0") + " " + u.name
		}
	}
	return strconv.FormatInt(n, 10) + " B"
}

// Finder configures the file finder.
type Finder struct {
	// Preview shows the content of the selected file beside the paths,
	// where the finder is at least 100 columns wide. Tab shows or hides
	// it either way.
	Preview bool `yaml:"preview"`
}

// maxBlob is the largest file GitHub serves through the API.
const maxBlob = 100 * MiB

func (f Files) validate() error {
	var errs []error
	if f.Preview.MaxSize <= 0 || f.Preview.MaxSize > maxBlob {
		errs = append(errs, fmt.Errorf("%s: must be between 1B and %v, got %v", MaxSizeKey, maxBlob, f.Preview.MaxSize))
	}
	if f.Preview.WarnSize <= 0 || f.Preview.WarnSize > f.Preview.MaxSize {
		errs = append(errs, fmt.Errorf("files.preview.warn_size: must be between 1B and %s (%v), got %v", MaxSizeKey, f.Preview.MaxSize, f.Preview.WarnSize))
	}
	if f.Markdown != MarkdownRendered && f.Markdown != MarkdownRaw {
		errs = append(errs, fmt.Errorf("files.markdown: must be %s or %s, got %q", MarkdownRendered, MarkdownRaw, f.Markdown))
	}
	return errors.Join(errs...)
}
