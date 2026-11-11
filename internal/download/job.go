package download

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
)

// ErrCancelled is what a cancelled job reports through Done.
var ErrCancelled = errors.New("download cancelled")

// Job is one running download. It saves to path.part and renames the file to
// path when curl ends well, so the real name never holds a partial file.
type Job struct {
	path, part string
	size       int64
	cmd        *exec.Cmd
	done       chan error
	cancelled  atomic.Bool
}

// Finished returns a job that is already done, for a downloader that starts
// nothing.
func Finished(path string, size int64) *Job {
	j := &Job{path: path, size: size, done: make(chan error, 1)}
	j.done <- nil
	close(j.done)
	return j
}

// Path is the file the download ends up as, which may differ from the one
// asked for when that name was taken.
func (j *Job) Path() string { return j.path }

// Progress returns the bytes saved so far and the asset's size. The size is
// 0 when it isn't known.
func (j *Job) Progress() (done, total int64) {
	st, err := os.Stat(j.part)
	if err != nil {
		st, err = os.Stat(j.path)
	}
	if err != nil {
		return 0, j.size
	}
	return st.Size(), j.size
}

// Cancel kills curl and removes the partial file. Done then reports
// ErrCancelled, unless curl had already finished.
func (j *Job) Cancel() {
	j.cancelled.Store(true)
	if j.cmd != nil {
		kill(j.cmd)
	}
}

// Done delivers the result once, then closes: nil for a saved file, else
// why it failed. One reader should take it.
func (j *Job) Done() <-chan error { return j.done }

// finish turns curl's exit into the job's result.
func (j *Job) finish(err error) error {
	switch {
	case j.cancelled.Load():
		_ = os.Remove(j.part)
		return ErrCancelled
	case err == nil:
		if err := os.Rename(j.part, j.path); err != nil {
			return fmt.Errorf("download: save %s: %w", j.path, err)
		}
		return nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return fmt.Errorf("download: %w", err)
	}
	code := exit.ExitCode()
	if code == 22 {
		// The server refused: the partial file can't be continued.
		_ = os.Remove(j.part)
	}
	return fmt.Errorf("download: %s (curl exit %d); the partial file is kept to resume", curlReason(code), code)
}

// curlReason says in words what the common curl exit codes mean.
func curlReason(code int) string {
	switch code {
	case 6, 7, 28, 35, 52, 55, 56:
		return "network error"
	case 22:
		return "the server refused the request, the link may have expired"
	case 23:
		return "can't write the file"
	}
	return "curl failed"
}
