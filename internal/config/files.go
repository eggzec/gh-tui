package config

import (
	"errors"
	"fmt"
	"time"
)

// Files configures the Files pane.
type Files struct {
	Prefetch Prefetch `yaml:"prefetch"`
	Preview  Preview  `yaml:"preview"`
}

// Prefetch configures reading files before they are opened, so the preview
// shows them at once. Each file costs one request.
type Prefetch struct {
	// Enabled reads the small top-level files of a repository once it is
	// listed, and the file under the cursor once it rests there.
	Enabled bool `yaml:"enabled"`
	// MaxSize is the largest top-level file read ahead. The file under the
	// cursor is read up to Preview.MaxSize.
	MaxSize Size `yaml:"max_size"`
	// HoverDelay is how long the cursor rests on a file before it is read,
	// so that moving through the tree doesn't read every file passed.
	HoverDelay time.Duration `yaml:"hover_delay"`
}

// Preview configures the file preview.
type Preview struct {
	// MaxSize is the largest file the preview reads. Larger files are
	// better opened in the browser.
	MaxSize Size `yaml:"max_size"`
}

// maxBlob is the largest file GitHub serves through the API.
const maxBlob = 100 * MiB

func defaultFiles() Files {
	return Files{
		Prefetch: Prefetch{Enabled: true, MaxSize: 64 * KiB, HoverDelay: 150 * time.Millisecond},
		Preview:  Preview{MaxSize: MiB},
	}
}

func (f Files) validate() error {
	var errs []error
	if f.Preview.MaxSize <= 0 || f.Preview.MaxSize > maxBlob {
		errs = append(errs, fmt.Errorf("files.preview.max_size: must be between 1B and %v, got %v", maxBlob, f.Preview.MaxSize))
	}
	switch p := f.Prefetch; {
	case p.MaxSize <= 0:
		errs = append(errs, fmt.Errorf("files.prefetch.max_size: must be positive, got %v", p.MaxSize))
	case p.MaxSize > f.Preview.MaxSize:
		errs = append(errs, fmt.Errorf("files.prefetch.max_size: must not exceed files.preview.max_size (%v), got %v", f.Preview.MaxSize, p.MaxSize))
	}
	if f.Prefetch.HoverDelay < 0 {
		errs = append(errs, fmt.Errorf("files.prefetch.hover_delay: must not be negative, got %v", f.Prefetch.HoverDelay))
	}
	return errors.Join(errs...)
}
