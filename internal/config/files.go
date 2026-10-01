package config

import (
	"errors"
	"fmt"
)

// Files configures the Files pane.
type Files struct {
	Preview Preview `yaml:"preview" when:"startup" why:"the files are read with it from the start"`
	Finder  Finder  `yaml:"finder"`
}

// Preview configures the file preview.
type Preview struct {
	// MaxSize is the largest file the preview reads. Larger files are
	// better opened in the browser.
	MaxSize Size `yaml:"max_size"`
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
		errs = append(errs, fmt.Errorf("files.preview.max_size: must be between 1B and %v, got %v", maxBlob, f.Preview.MaxSize))
	}
	return errors.Join(errs...)
}
