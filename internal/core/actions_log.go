package core

import (
	"strings"
	"time"
)

// LogKind is what a line of a job log is, as the runner marks it.
type LogKind uint8

// Log kinds. The runner marks them with a prefix such as ##[error], which
// ParseLog removes. A group runs until its end group, and groups don't
// nest.
const (
	LogPlain LogKind = iota
	LogGroup
	LogEndGroup
	LogError
	LogWarning
	LogNotice
	LogDebug
	// LogCommand is a command that a step ran, such as git.
	LogCommand
)

// LogLine is a line of a job log.
type LogLine struct {
	// Time is when the runner wrote the line, or zero if it wrote none.
	Time time.Time
	// Text is the line without its time, its kind's prefix or its line
	// ending. It keeps the ANSI escape sequences that color it.
	Text string
	Kind LogKind
	// Step is the Number of the step that wrote the line, as far as its
	// time and the start of the step's output tell, or 0 if unknown.
	Step int
}

// Log is the parsed log of a job.
type Log struct {
	Lines []LogLine
	// Truncated reports that the log was too large to read whole, so
	// Lines are only its end.
	Truncated bool
}

// PartialLog is the log of a job in progress as far as GitHub publishes it,
// in blocks of about 2 MiB as the runner uploads them, so its end is often
// minutes behind the job.
type PartialLog struct {
	Log
	// At is when the read that brought its last lines ended.
	At time.Time
	// Gen counts the times it was read again from the start, as when the
	// log started over; within one Gen, Lines only grow.
	Gen int
}

// logMarkers are the kinds of the ##[name] prefixes the runner writes.
var logMarkers = map[string]LogKind{
	"group":    LogGroup,
	"endgroup": LogEndGroup,
	"error":    LogError,
	"warning":  LogWarning,
	"notice":   LogNotice,
	"debug":    LogDebug,
	"command":  LogCommand,
}

// ParseLog parses the log of a job, as GitHub serves it: lines ending in
// LF or CRLF, each after the time the runner wrote it, such as
// "2026-09-22T09:49:37.4680928Z ##[group]Run actions/checkout@v4". Steps
// are the job's, to tell which step wrote each line, and may be nil.
//
// The log doesn't name its steps, so a line belongs to the step whose time
// span holds it. Steps start and end within the same second, and GitHub
// counts only seconds, so where the output of a step starts decides:
// a group that starts with "Run ", "Post job cleanup." and "Cleaning up
// orphan processes" start the next step that ran.
func ParseLog(text string, steps []Step) Log {
	return Log{Lines: NewLogParser(steps).Parse(text, steps)}
}

// LogParser parses a log that grows, such as that of a job in progress, a
// part at a time, the way ParseLog parses it whole: which step wrote a
// line depends on the lines before it.
type LogParser struct {
	st      *stepper
	started bool
}

// NewLogParser returns a parser of the log of a job with steps, which may
// be nil.
func NewLogParser(steps []Step) *LogParser {
	return &LogParser{st: newStepper(steps)}
}

// Parse parses text, the whole lines that follow those parsed before,
// with the steps of the job as they stand now, which move on as it runs.
func (p *LogParser) Parse(text string, steps []Step) []LogLine {
	if !p.started {
		text = strings.TrimPrefix(text, "\uFEFF")
		p.started = true
	}
	p.st.update(steps)
	lines := make([]LogLine, 0, strings.Count(text, "\n")+1)
	for text != "" {
		var line string
		line, text, _ = strings.Cut(text, "\n")
		line = strings.TrimSuffix(line, "\r")
		l := parseLogLine(line)
		l.Step = p.st.at(l)
		lines = append(lines, l)
	}
	return lines
}

func parseLogLine(line string) LogLine {
	var l LogLine
	if t, rest, ok := cutLogTime(line); ok {
		l.Time, line = t, rest
	}
	if name, rest, ok := cutMarker(line); ok {
		if kind, known := logMarkers[name]; known {
			l.Kind, line = kind, rest
		}
	} else if rest, ok := strings.CutPrefix(line, "[command]"); ok {
		l.Kind, line = LogCommand, rest
	}
	l.Text = line
	return l
}

// cutMarker cuts a ##[name] prefix from line.
func cutMarker(line string) (name, rest string, ok bool) {
	if !strings.HasPrefix(line, "##[") {
		return "", line, false
	}
	end := strings.IndexByte(line, ']')
	if end < 0 {
		return "", line, false
	}
	return line[3:end], line[end+1:], true
}

// cutLogTime cuts the time and the space after it from the start of line.
// It parses the one layout the runner writes, RFC 3339 in UTC with up to
// nine fractional digits, by hand: time.Parse would take most of the time
// of parsing a log.
func cutLogTime(line string) (time.Time, string, bool) {
	const minLen = len("2006-01-02T15:04:05Z ")
	if len(line) < minLen || line[4] != '-' || line[7] != '-' || line[10] != 'T' || line[13] != ':' || line[16] != ':' {
		return time.Time{}, line, false
	}
	year, ok1 := digits(line[0:4])
	month, ok2 := digits(line[5:7])
	day, ok3 := digits(line[8:10])
	hour, ok4 := digits(line[11:13])
	minute, ok5 := digits(line[14:16])
	sec, ok6 := digits(line[17:19])
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || sec > 60 {
		return time.Time{}, line, false
	}
	i, nsec := 19, 0
	if line[i] == '.' {
		i++
		scale := 100_000_000
		for ; i < len(line) && line[i] >= '0' && line[i] <= '9'; i++ {
			nsec += int(line[i]-'0') * scale
			scale /= 10
		}
	}
	if i+1 >= len(line) || line[i] != 'Z' || line[i+1] != ' ' {
		// A line that is only a time has no space after it.
		if i+1 == len(line) && line[i] == 'Z' {
			return time.Date(year, time.Month(month), day, hour, minute, sec, nsec, time.UTC), "", true
		}
		return time.Time{}, line, false
	}
	return time.Date(year, time.Month(month), day, hour, minute, sec, nsec, time.UTC), line[i+2:], true
}

// digits parses s, which must be only decimal digits.
func digits(s string) (int, bool) {
	n := 0
	for i := range len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// stepper follows the steps of a job through its log.
type stepper struct {
	steps []Step
	// cur is the index of the step of the last line, or -1.
	cur int
}

func newStepper(steps []Step) *stepper {
	s := &stepper{steps: steps, cur: -1}
	for i, st := range steps {
		if st.Conclusion != ConclusionSkipped {
			s.cur = i
			break
		}
	}
	return s
}

// update takes the steps as they stand now, the same steps with later
// times, and keeps the step reached. Steps first known now start the
// stepper, and none, as of a job no longer known, leave it as it was.
func (s *stepper) update(steps []Step) {
	if len(steps) == 0 {
		return
	}
	if s.cur < 0 {
		*s = *newStepper(steps)
		return
	}
	s.steps = steps
	s.cur = min(s.cur, len(steps)-1)
}

// at returns the number of the step that wrote l.
func (s *stepper) at(l LogLine) int {
	if s.cur < 0 {
		return 0
	}
	if !l.Time.IsZero() {
		sec := l.Time.Truncate(time.Second)
		cur := s.steps[s.cur]
		// A line after the current step ended belongs to a later one,
		// even if its output didn't start as expected.
		if startsStep(l) || (!cur.CompletedAt.IsZero() && sec.After(cur.CompletedAt)) {
			s.advance(sec)
		}
	}
	return s.steps[s.cur].Number
}

// advance moves to the first later step that ran and hadn't ended by sec.
func (s *stepper) advance(sec time.Time) {
	for i := s.cur + 1; i < len(s.steps); i++ {
		st := s.steps[i]
		if st.Conclusion == ConclusionSkipped || st.StartedAt.IsZero() {
			continue
		}
		if st.CompletedAt.IsZero() || !st.CompletedAt.Before(sec) {
			s.cur = i
			return
		}
	}
}

// startsStep reports whether l is how the output of a step starts.
func startsStep(l LogLine) bool {
	switch l.Kind {
	case LogGroup:
		return strings.HasPrefix(l.Text, "Run ")
	case LogPlain:
		return l.Text == "Post job cleanup." || l.Text == "Cleaning up orphan processes"
	default:
		return false
	}
}
