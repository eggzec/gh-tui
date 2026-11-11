package main

import (
	"os"
	"sync/atomic"

	"github.com/eggzec/gh-tui/internal/download"
	"github.com/eggzec/gh-tui/internal/tui"
)

// downloads is what the app saves files with: curl, detached, started in
// the folder the settings name now.
type downloads struct {
	run  *download.Runner
	fake bool
	dir  atomic.Value // string, as the settings give it
}

// newDownloads returns the downloader for dir. With fake set it starts
// nothing, so a demo or a test never touches the network or the disk.
func newDownloads(dir string, fake bool) *downloads {
	d := &downloads{run: download.New(), fake: fake}
	d.setDir(dir)
	return d
}

// setDir sets the folder downloads start in, when the settings change.
func (d *downloads) setDir(dir string) { d.dir.Store(dir) }

// Dir is the folder downloads start in, with ~ expanded and the home
// directory standing in for a folder that doesn't exist.
func (d *downloads) Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	dir, _ := d.dir.Load().(string)
	return download.Dir(dir, home)
}

// releaseDownloads adapts downloads to the release modal, which tells only
// whether a transfer started.
type releaseDownloads struct{ d *downloads }

// Dir is where the folder picker starts: the folder of downloads, or the
// working directory for a demo, which never writes there.
func (r releaseDownloads) Dir() string {
	if r.d.fake {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
	}
	return r.d.Dir()
}

// Path is where the file called name goes in dir.
func (releaseDownloads) Path(dir, name string) (string, error) { return download.Path(dir, name) }

// Start starts the transfer of url to path.
func (r releaseDownloads) Start(url, path string, size int64) error {
	_, err := r.d.Start(url, path, size)
	return err
}

// Start starts the download of url to path. A fake downloader gives a job
// that is already done.
func (d *downloads) Start(url, path string, size int64) (tui.Download, error) {
	if d.fake {
		return download.Finished(path, size), nil
	}
	j, err := d.run.Start(url, path, size)
	if err != nil {
		return nil, err
	}
	return j, nil
}
