package download

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestStart(t *testing.T) {
	t.Parallel()
	const url = "https://objects.example/signed?sig=secret"
	var got *exec.Cmd
	r := New(
		WithLookPath(func(string) (string, error) { return "/bin/curl", nil }),
		WithStarter(func(cmd *exec.Cmd) (func() error, error) {
			got = cmd
			return func() error { return nil }, nil
		}),
	)
	dir := t.TempDir()
	job, err := r.Start(url, filepath.Join(dir, "app.tar.gz"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if job.Path() != filepath.Join(dir, "app.tar.gz") {
		t.Errorf("Path = %q", job.Path())
	}
	want := []string{"/bin/curl", "-L", "-C", "-", "--fail", "--proto", "=https", "--proto-redir", "=https",
		"-o", filepath.Join(dir, "app.tar.gz.part"), "-K", "-"}
	if !slices.Equal(got.Args[1:], want[1:]) || got.Path != "/bin/curl" {
		t.Errorf("argv = %v", got.Args)
	}
	for _, a := range got.Args {
		if strings.Contains(a, "secret") || strings.Contains(a, "example") {
			t.Errorf("URL in argv: %q", a)
		}
	}
	in, _ := io.ReadAll(got.Stdin)
	if string(in) != "url = \""+url+"\"\n" {
		t.Errorf("stdin = %q", in)
	}
	if got.Stdout != nil || got.Stderr != nil {
		t.Error("output streams are tied to the terminal")
	}
	assertDetached(t, got)
}

func TestStartErrors(t *testing.T) {
	t.Parallel()
	started := false
	ok := func(string) (string, error) { return "curl", nil }
	start := WithStarter(func(*exec.Cmd) (func() error, error) {
		started = true
		return func() error { return nil }, nil
	})
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if _, err := New(WithLookPath(func(string) (string, error) { return "", errors.New("no") }), start).Start("https://x", a, 0); !errors.Is(err, ErrNoCurl) {
		t.Errorf("missing curl: %v", err)
	}
	r := New(WithLookPath(ok), start)
	for _, c := range [][2]string{
		{"", a},
		{"http://x", a},
		{"file:///etc/passwd", a},
		{"https://x\"\nurl = \"file:///etc/passwd", a},
		{"https://x", dir + string(filepath.Separator) + ".."},
		{"https://x", string(filepath.Separator)},
		{"https://x", "-"},
		{"https://x", "a"},
	} {
		if _, err := r.Start(c[0], c[1], 0); err == nil {
			t.Errorf("Start(%q, %q) succeeded", c[0], c[1])
		}
	}
	if started {
		t.Error("curl started on a refused download")
	}
	failing := New(WithLookPath(ok), WithStarter(func(*exec.Cmd) (func() error, error) { return nil, errors.New("boom") }))
	if _, err := failing.Start("https://x", a, 0); err == nil {
		t.Error("start failure not reported")
	}
	if _, err := failing.Start("https://x", a, 0); errors.Is(err, ErrInProgress) {
		t.Error("a failed start left the path busy")
	}
}

// partOf is the file a started curl writes, from its -o argument.
func partOf(cmd *exec.Cmd) string {
	i := slices.Index(cmd.Args, "-o")
	return cmd.Args[i+1]
}

func TestExistingFileKept(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	final := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(final, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(final, link); err != nil {
		t.Skip("no symlinks")
	}
	r := New(
		WithLookPath(func(string) (string, error) { return "curl", nil }),
		WithStarter(func(cmd *exec.Cmd) (func() error, error) {
			if err := os.WriteFile(partOf(cmd), []byte("v2"), 0o600); err != nil {
				return nil, err
			}
			return func() error { return nil }, nil
		}),
	)
	for _, name := range []string{"SHA256SUMS", "link"} {
		job, err := r.Start("https://x", filepath.Join(dir, name), 2)
		if err != nil {
			t.Fatal(err)
		}
		if err := <-job.Done(); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(final); string(got) != "v1" {
			t.Errorf("existing file changed: %q", got)
		}
		if got, _ := os.ReadFile(job.Path()); string(got) != "v2" || job.Path() == final || job.Path() == link {
			t.Errorf("saved to %q: %q", job.Path(), got)
		}
	}
}

func TestPartResumedAndSymlinkRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var got *exec.Cmd
	r := New(
		WithLookPath(func(string) (string, error) { return "curl", nil }),
		WithStarter(func(cmd *exec.Cmd) (func() error, error) {
			got = cmd
			return func() error { return nil }, nil
		}),
	)
	path := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(path+".part", []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := r.Start("https://x", path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if done, total := job.Progress(); done != 5 || total != 10 {
		t.Errorf("Progress = %d/%d", done, total)
	}
	if partOf(got) != path+".part" || !slices.Contains(got.Args, "-C") {
		t.Errorf("not resumed: %v", got.Args)
	}
	<-job.Done()

	other := filepath.Join(dir, "b.zip")
	if err := os.Symlink(path, other+".part"); err != nil {
		t.Skip("no symlinks")
	}
	if _, err := r.Start("https://x", other, 0); err == nil {
		t.Error("symlinked .part accepted")
	}
}

func TestSamePathRefused(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	r := New(
		WithLookPath(func(string) (string, error) { return "curl", nil }),
		WithStarter(func(*exec.Cmd) (func() error, error) {
			return func() error { <-release; return nil }, nil
		}),
	)
	path := filepath.Join(t.TempDir(), "a")
	job, err := r.Start("https://x", path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start("https://x", path, 0); !errors.Is(err, ErrInProgress) {
		t.Errorf("second start: %v", err)
	}
	close(release)
	<-job.Done()
	// The path is free again once the job ended; the rename then fails for
	// lack of a file, which is not what this test is about.
	if _, err := r.Start("https://x", path, 0); errors.Is(err, ErrInProgress) {
		t.Error("path still busy after the job ended")
	}
}

func TestName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"app.tar.gz":       "app.tar.gz",
		"../../etc/passwd": "passwd",
		`..\win\evil.exe`:  "evil.exe",
		"a/b/c.txt":        "c.txt",
		"..":               "",
		".":                "",
		"":                 "",
		"dir/":             "",
		"na\x00me\n.txt":   "name.txt",
		"  x  ":            "x",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPath(t *testing.T) {
	t.Parallel()
	got, err := Path("/dl", "../x.zip")
	if err != nil || got != filepath.FromSlash("/dl/x.zip") {
		t.Errorf("Path = %q, %v", got, err)
	}
	if _, err := Path("/dl", ".."); err == nil {
		t.Error("`..` accepted")
	}
}

func TestDir(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"~/Downloads": filepath.Join(home, "Downloads"),
		"~":           home,
		"~/Missing":   home,
		home:          home,
		"/no/such":    home,
	} {
		if got := Dir(in, home); got != want {
			t.Errorf("Dir(%q) = %q, want %q", in, got, want)
		}
	}
}
