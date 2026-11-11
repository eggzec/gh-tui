// Package download saves a remote file with curl, detached from the
// terminal.
//
// Each download is one curl process in a session of its own, so it outlives
// the app and a later run of the same download resumes the partial file.
// curl is given only the URL, on its standard input: the URL may be a signed
// link, and it stays out of the process list.
package download

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Starter starts cmd and returns the function that waits for it to end.
type Starter func(cmd *exec.Cmd) (wait func() error, err error)

// Start starts cmd without waiting for it.
func Start(cmd *exec.Cmd) (func() error, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}

// ErrNoCurl means curl isn't installed.
var ErrNoCurl = errors.New("curl not found: install curl to download files")

// Runner starts downloads. Build it with New.
type Runner struct {
	lookPath func(string) (string, error)
	start    Starter

	mu   sync.Mutex
	busy map[string]bool // paths being downloaded to
}

// Option configures a Runner.
type Option func(*Runner)

// WithLookPath sets how curl is found, exec.LookPath by default.
func WithLookPath(lookPath func(string) (string, error)) Option {
	return func(r *Runner) { r.lookPath = lookPath }
}

// WithStarter sets what starts curl, Start by default.
func WithStarter(start Starter) Option {
	return func(r *Runner) { r.start = start }
}

// New returns a runner.
func New(opts ...Option) *Runner {
	r := &Runner{lookPath: exec.LookPath, start: Start, busy: map[string]bool{}}
	for _, o := range opts {
		o(r)
	}
	return r
}

// ErrInProgress means a download to that path is already running.
var ErrInProgress = errors.New("download: already downloading to that path")

// Start downloads url, an asset of size bytes (0 if unknown), to path, which
// must be absolute with a plain file name: its base is checked by Name. It
// returns once curl has started; curl keeps going without the app.
//
// curl writes to path.part, which a later run of the same download resumes,
// and the file gets its name only when curl ends well. A name that is taken
// is never written to: the file becomes "name (1).ext" instead, see
// Job.Path.
func (r *Runner) Start(url, path string, size int64) (*Job, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, errors.New("download: URL is not https")
	}
	if strings.ContainsAny(url, "\"\r\n") {
		return nil, errors.New("download: URL has characters curl's config can't carry")
	}
	if !filepath.IsAbs(path) || Name(filepath.Base(path)) != filepath.Base(path) || filepath.Base(path) == "" {
		return nil, fmt.Errorf("download: refusing file path %q", path)
	}
	bin, err := r.lookPath("curl")
	if err != nil {
		return nil, ErrNoCurl
	}
	key := filepath.Clean(path)
	if !r.claim(key) {
		return nil, ErrInProgress
	}
	job, err := r.begin(bin, url, key, size)
	if err != nil {
		r.release(key)
		return nil, err
	}
	// Reap curl when it ends, so it doesn't linger as a zombie while the app
	// runs, and tell how it went.
	go func(wait func() error) {
		err := job.finish(wait())
		r.release(key)
		job.done <- err
		close(job.done)
	}(job.wait)
	return job.Job, nil
}

// started is a job with the function that waits for its curl.
type started struct {
	*Job
	wait func() error
}

func (r *Runner) claim(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy[key] {
		return false
	}
	r.busy[key] = true
	return true
}

func (r *Runner) release(key string) {
	r.mu.Lock()
	delete(r.busy, key)
	r.mu.Unlock()
}

// begin picks the free name, checks the partial file and starts curl.
func (r *Runner) begin(bin, url, path string, size int64) (*started, error) {
	path, err := freeName(path)
	if err != nil {
		return nil, err
	}
	part := path + ".part"
	if st, err := os.Lstat(part); err == nil && !st.Mode().IsRegular() {
		return nil, fmt.Errorf("download: refusing %s, it is not a plain file", part)
	}
	cmd := exec.Command(bin, "-L", "-C", "-", "--fail", "--proto", "=https", "--proto-redir", "=https", "-o", part, "-K", "-")
	detach(cmd)
	cmd.Stdin = strings.NewReader("url = \"" + strings.NewReplacer(`\`, `\\`).Replace(url) + "\"\n")
	wait, err := r.start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start curl: %w", err)
	}
	job := &Job{path: path, part: part, size: size, cmd: cmd, done: make(chan error, 1)}
	return &started{Job: job, wait: wait}, nil
}

// freeName returns path, or the first "name (n).ext" beside it, that nothing
// exists at. A symlink counts as existing, so none is ever written through.
func freeName(path string) (string, error) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	cand := path
	for i := 1; i < 1000; i++ {
		_, err := os.Lstat(cand)
		if errors.Is(err, fs.ErrNotExist) {
			return cand, nil
		}
		if err != nil {
			return "", fmt.Errorf("download: %w", err)
		}
		cand = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	return "", fmt.Errorf("download: no free name for %s", path)
}

// Name returns a file name made safe from one GitHub gave: its last path
// element, with separators and control characters gone. It returns "" for
// a name that has nothing left, "." or "..".
func Name(name string) string {
	name = strings.NewReplacer("\\", "/").Replace(name)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// Path returns where the file called name goes in dir, or an error when the
// name is empty after Name cleans it. The file is the only thing a download
// creates, and always directly inside dir.
func Path(dir, name string) (string, error) {
	clean := Name(name)
	if clean == "" {
		return "", fmt.Errorf("download: no usable file name in %q", name)
	}
	return filepath.Join(dir, clean), nil
}

// Dir expands a leading ~ in dir to home and falls back to home when the
// directory doesn't exist.
func Dir(dir, home string) string {
	if dir == "~" {
		dir = home
	} else if rest, ok := strings.CutPrefix(dir, "~/"); ok {
		dir = filepath.Join(home, rest)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return home
	}
	return dir
}
