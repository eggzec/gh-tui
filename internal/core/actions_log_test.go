package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func logTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseLogLines(t *testing.T) {
	long := strings.Repeat("x", 1<<20)
	tests := []struct {
		name string
		in   string
		want []LogLine
	}{
		{"empty", "", []LogLine{}},
		{
			"plain",
			"2026-09-22T09:49:37.4680928Z Current runner version: '2.337.0'\n",
			[]LogLine{{Time: logTime("2026-09-22T09:49:37.4680928Z"), Text: "Current runner version: '2.337.0'"}},
		},
		{
			"byte order mark and CRLF",
			"\uFEFF2026-09-22T09:49:37.4680928Z first\r\n2026-09-22T09:49:37.5Z second\r\n",
			[]LogLine{
				{Time: logTime("2026-09-22T09:49:37.4680928Z"), Text: "first"},
				{Time: logTime("2026-09-22T09:49:37.5Z"), Text: "second"},
			},
		},
		{
			"no final line ending",
			"2026-09-22T09:49:37Z a\n2026-09-22T09:49:38Z b",
			[]LogLine{{Time: logTime("2026-09-22T09:49:37Z"), Text: "a"}, {Time: logTime("2026-09-22T09:49:38Z"), Text: "b"}},
		},
		{
			"group",
			"2026-09-22T09:49:39.1048950Z ##[group]Run git config --global core.autocrlf input\n" +
				"2026-09-22T09:49:39.1049920Z \x1b[36;1mgit config --global core.autocrlf input\x1b[0m\n" +
				"2026-09-22T09:49:39.2086689Z ##[endgroup]\n",
			[]LogLine{
				{Time: logTime("2026-09-22T09:49:39.1048950Z"), Text: "Run git config --global core.autocrlf input", Kind: LogGroup},
				{Time: logTime("2026-09-22T09:49:39.1049920Z"), Text: "\x1b[36;1mgit config --global core.autocrlf input\x1b[0m"},
				{Time: logTime("2026-09-22T09:49:39.2086689Z"), Kind: LogEndGroup},
			},
		},
		{
			"annotations",
			"2026-09-22T09:50:07.5273319Z ##[warning]Restore cache failed\n" +
				"2026-09-22T09:50:42.3860170Z ##[error]clipboard_backend.go:95:6: type clipboardCommand is unused (unused)\n" +
				"2026-09-22T09:50:42.3860170Z ##[notice]note\n" +
				"2026-09-22T09:50:42.3860170Z ##[debug]Evaluating condition\n",
			[]LogLine{
				{Time: logTime("2026-09-22T09:50:07.5273319Z"), Text: "Restore cache failed", Kind: LogWarning},
				{Time: logTime("2026-09-22T09:50:42.3860170Z"), Text: "clipboard_backend.go:95:6: type clipboardCommand is unused (unused)", Kind: LogError},
				{Time: logTime("2026-09-22T09:50:42.3860170Z"), Text: "note", Kind: LogNotice},
				{Time: logTime("2026-09-22T09:50:42.3860170Z"), Text: "Evaluating condition", Kind: LogDebug},
			},
		},
		{
			"commands",
			"2026-09-22T09:50:48.2605779Z [command]\"C:\\Program Files\\Git\\bin\\git.exe\" version\n" +
				"2026-09-22T09:50:48.2605779Z ##[command]/usr/bin/git version\n",
			[]LogLine{
				{Time: logTime("2026-09-22T09:50:48.2605779Z"), Text: `"C:\Program Files\Git\bin\git.exe" version`, Kind: LogCommand},
				{Time: logTime("2026-09-22T09:50:48.2605779Z"), Text: "/usr/bin/git version", Kind: LogCommand},
			},
		},
		{
			"unknown marker stays",
			"2026-09-22T09:50:48Z ##[section]Starting\n2026-09-22T09:50:48Z ##[unterminated\n",
			[]LogLine{
				{Time: logTime("2026-09-22T09:50:48Z"), Text: "##[section]Starting"},
				{Time: logTime("2026-09-22T09:50:48Z"), Text: "##[unterminated"},
			},
		},
		{
			"no time",
			"plain text\n2026-09-22 09:50:48 not a runner time\n2026-13-99T99:99:99Z\n",
			[]LogLine{{Text: "plain text"}, {Text: "2026-09-22 09:50:48 not a runner time"}, {Text: "2026-13-99T99:99:99Z"}},
		},
		{
			"only a time",
			"2026-09-22T09:50:48.1Z\n2026-09-22T09:50:48.1Zx\n",
			[]LogLine{{Time: logTime("2026-09-22T09:50:48.1Z")}, {Text: "2026-09-22T09:50:48.1Zx"}},
		},
		{
			"blank lines",
			"2026-09-22T09:50:48Z \n\n",
			[]LogLine{{Time: logTime("2026-09-22T09:50:48Z")}, {}},
		},
		{
			"very long line",
			"2026-09-22T09:50:48Z " + long + "\n",
			[]LogLine{{Time: logTime("2026-09-22T09:50:48Z"), Text: long}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLog(tt.in, nil)
			if len(got.Lines) != len(tt.want) {
				t.Fatalf("got %d lines, want %d: %+v", len(got.Lines), len(tt.want), got.Lines)
			}
			for i, l := range got.Lines {
				w := tt.want[i]
				if !l.Time.Equal(w.Time) || l.Text != w.Text || l.Kind != w.Kind || l.Step != 0 {
					t.Errorf("line %d = %+v, want %+v", i, l, w)
				}
			}
		})
	}
}

// readLog reads a recorded job log and the steps of its job, as the
// jobs API reported them.
func readLog(tb testing.TB, name string) (string, []Step) {
	tb.Helper()
	text, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if err != nil {
		tb.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", name+".steps.json"))
	if err != nil {
		tb.Fatal(err)
	}
	var raw []struct {
		Number      int        `json:"number"`
		Name        string     `json:"name"`
		Status      RunStatus  `json:"status"`
		Conclusion  Conclusion `json:"conclusion"`
		StartedAt   time.Time  `json:"started_at"`
		CompletedAt time.Time  `json:"completed_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		tb.Fatal(err)
	}
	steps := make([]Step, len(raw))
	for i, r := range raw {
		steps[i] = Step(r)
	}
	return string(text), steps
}

// TestParseLogSteps checks on recorded logs which step each line is
// given: the first line of each step's output, by line number, is where
// the step starts. Skipped steps write nothing.
func TestParseLogSteps(t *testing.T) {
	tests := []struct {
		name      string
		lines     int
		starts    map[int]int
		errors    int
		warnings  int
		groups    int
		untimed   int
		failStep  int
		lastError string
	}{
		{
			name:  "actions_log_lint_windows",
			lines: 413,
			starts: map[int]int{
				1: 1, 45: 2, 49: 3, 135: 4, 209: 5, 285: 7, 343: 11, 347: 13, 380: 14, 413: 15,
			},
			// An input with a line break writes a line without a time.
			errors: 5, warnings: 1, groups: 31, untimed: 1, failStep: 7,
			lastError: "issues found",
		},
		{
			name:  "actions_log_build_ubuntu",
			lines: 314,
			starts: map[int]int{
				1: 1, 58: 2, 159: 4, 243: 5, 251: 6, 259: 7, 267: 8, 276: 15, 282: 16, 314: 17,
			},
			groups: 20,
		},
		{
			name:  "actions_log_build_windows",
			lines: 552,
			starts: map[int]int{
				1: 1, 58: 2, 146: 4, 232: 5, 240: 6, 249: 7, 257: 8, 520: 16, 552: 17,
			},
			errors: 1, groups: 20, failStep: 8,
			lastError: "Process completed with exit code 1.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, steps := readLog(t, tt.name)
			log := ParseLog(text, steps)
			if len(log.Lines) != tt.lines {
				t.Fatalf("got %d lines, want %d", len(log.Lines), tt.lines)
			}
			got := map[int]int{}
			prev := -1
			var errs, warns, groups, untimed int
			for i, l := range log.Lines {
				if l.Step != prev {
					got[i+1], prev = l.Step, l.Step
				}
				if l.Time.IsZero() {
					untimed++
				}
				if strings.HasSuffix(l.Text, "\r") || strings.HasPrefix(l.Text, "##[") {
					t.Errorf("line %d = %q, want it without its ending and marker", i+1, l.Text)
				}
				switch l.Kind {
				case LogError:
					errs++
					if tt.failStep != 0 && l.Step != tt.failStep {
						t.Errorf("error on line %d is in step %d, want %d", i+1, l.Step, tt.failStep)
					}
					if l.Text == tt.lastError {
						tt.lastError = ""
					}
				case LogWarning:
					warns++
				case LogGroup:
					groups++
				default:
				}
			}
			for line, step := range tt.starts {
				if got[line] != step {
					t.Errorf("line %d starts step %d, want %d", line, got[line], step)
				}
			}
			if len(got) != len(tt.starts) {
				t.Errorf("steps start at %v, want %v", got, tt.starts)
			}
			if errs != tt.errors || warns != tt.warnings || groups != tt.groups || untimed != tt.untimed {
				t.Errorf("%d errors, %d warnings, %d groups, %d untimed; want %d, %d, %d, %d",
					errs, warns, groups, untimed, tt.errors, tt.warnings, tt.groups, tt.untimed)
			}
			if tt.lastError != "" {
				t.Errorf("no error %q", tt.lastError)
			}
		})
	}
}

func TestParseLogStepsInProgress(t *testing.T) {
	at := logTime("2026-09-25T07:37:28Z")
	steps := []Step{
		{Number: 1, Name: "Set up job", Status: RunCompleted, Conclusion: ConclusionSuccess, StartedAt: at, CompletedAt: at.Add(time.Second)},
		{Number: 2, Name: "run the build", Status: RunInProgress, StartedAt: at.Add(time.Second)},
		{Number: 3, Name: "upload", Status: RunPending},
	}
	log := ParseLog("2026-09-25T07:37:28.5Z setting up\n"+
		"2026-09-25T07:37:29.1Z ##[group]Run make\n"+
		"2026-09-25T07:39:00Z building\n"+
		"untimed\n", steps)
	want := []int{1, 2, 2, 2}
	for i, l := range log.Lines {
		if l.Step != want[i] {
			t.Errorf("line %d (%q) is in step %d, want %d", i+1, l.Text, l.Step, want[i])
		}
	}
}

func TestConclusionFailed(t *testing.T) {
	for c, want := range map[Conclusion]bool{
		ConclusionFailure: true, ConclusionTimedOut: true, ConclusionStartupFailure: true,
		ConclusionSuccess: false, ConclusionCancelled: false, ConclusionNone: false, ConclusionSkipped: false,
	} {
		if got := c.Failed(); got != want {
			t.Errorf("%q.Failed() = %v, want %v", c, got, want)
		}
	}
}

func TestRefusedError(t *testing.T) {
	cause := ErrConflict
	err := &RefusedError{Action: "run can't be re-run", Reason: "it is too old", Err: cause}
	if got := err.Error(); got != "run can't be re-run: it is too old" {
		t.Errorf("Error() = %q", got)
	}
	if got := (&RefusedError{Action: "run can't be cancelled"}).Error(); got != "run can't be cancelled" {
		t.Errorf("Error() without a reason = %q", got)
	}
	if !errors.Is(err, ErrConflict) {
		t.Error("RefusedError doesn't unwrap to its cause")
	}
}

// BenchmarkParseLog parses a log of about 5 MB, made of recorded logs.
func BenchmarkParseLog(b *testing.B) {
	text, steps := readLog(b, "actions_log_build_windows")
	var sb strings.Builder
	for sb.Len() < 5<<20 {
		sb.WriteString(text)
	}
	big := sb.String()
	b.SetBytes(int64(len(big)))
	b.ReportAllocs()
	for b.Loop() {
		ParseLog(big, steps)
	}
}
