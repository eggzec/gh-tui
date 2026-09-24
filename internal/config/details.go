package config

import (
	"errors"
	"fmt"
	"time"
)

// Details configures the details of pull requests and issues, which open in
// a modal from their lists.
type Details struct {
	Prefetch DetailsPrefetch `yaml:"prefetch"`
}

// DetailsPrefetch configures reading details before they are opened, so the
// modal shows them at once. Each detail costs two requests, one for the
// pull request or issue and one for its first comments.
type DetailsPrefetch struct {
	// Enabled reads the details of the first rows of a list once it loads,
	// and of the row under the cursor once it rests there.
	Enabled bool `yaml:"enabled"`
	// Rows is how many of the first rows of each list are read ahead. Zero
	// reads only the row under the cursor.
	Rows int `yaml:"rows"`
	// HoverDelay is how long the cursor rests on a row before its detail is
	// read, so that moving through a list doesn't read every row passed.
	HoverDelay time.Duration `yaml:"hover_delay"`
}

// maxPrefetchRows is the size of a first list page, beyond which rows
// aren't loaded yet.
const maxPrefetchRows = 30

func defaultDetails() Details {
	return Details{
		Prefetch: DetailsPrefetch{Enabled: true, Rows: 5, HoverDelay: 150 * time.Millisecond},
	}
}

func (d Details) validate() error {
	var errs []error
	if p := d.Prefetch; p.Rows < 0 || p.Rows > maxPrefetchRows {
		errs = append(errs, fmt.Errorf("details.prefetch.rows: must be between 0 and %d, got %d", maxPrefetchRows, p.Rows))
	}
	if d.Prefetch.HoverDelay < 0 {
		errs = append(errs, fmt.Errorf("details.prefetch.hover_delay: must not be negative, got %v", d.Prefetch.HoverDelay))
	}
	return errors.Join(errs...)
}
