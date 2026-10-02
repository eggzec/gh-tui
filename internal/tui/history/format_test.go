package history

import (
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestGraphRowFields(t *testing.T) {
	c := history("main", 3)[0]
	tests := []struct {
		row                         []string
		format                      string
		short, title, detail, right string
	}{
		{[]string{config.FieldShortSHA, config.FieldSubject, config.FieldAuthor, config.FieldAge}, config.DateRelative,
			short(c.SHA), "main: change 0", "kevin9327", "1d"},
		// Fields after the subject keep their order in their place.
		{[]string{config.FieldDate, config.FieldTrailers, config.FieldCommitter, config.FieldVerified, config.FieldAge}, config.DateRelative,
			"", "", "+1 · web-flow · ✓", "2026-09-23 1d"},
		// The date follows the format; the age stays an age.
		{[]string{config.FieldDate, config.FieldAge}, "Jan _2", "", "", "", "Sep 23 1d"},
		{[]string{config.FieldSubject}, config.DateRelative, "", "main: change 0", "", ""},
	}
	for _, tt := range tests {
		cfg := testConfig()
		cfg.Row = tt.row
		g := newFormat(cfg, ui.NewDates(tt.format).In(time.UTC), ui.NewIcons(config.IconsUnicode)).graphCommit(c, testNow)
		if g.Short != tt.short || g.Title != tt.title || g.Detail != tt.detail || g.Right != tt.right {
			t.Errorf("row %v = %q %q %q %q, want %q %q %q %q", tt.row, g.Short, g.Title, g.Detail, g.Right, tt.short, tt.title, tt.detail, tt.right)
		}
		if g.ID != c.SHA || len(g.Parents) != 1 {
			t.Errorf("row %v lost the ID or the parents", tt.row)
		}
	}
}

func TestDates(t *testing.T) {
	authored := testNow.Add(-50 * time.Hour)
	tests := []struct {
		format    string
		committed time.Time
		want      string
	}{
		{config.DateRelative, authored, "2d ago · 2026-09-22 10:00 UTC"},
		{config.DateRelative, testNow.Add(-time.Hour), "2d ago, committed 1h ago"},
		{config.DateAbsolute, authored, "2026-09-22 10:00 UTC"},
		{config.DateAbsolute, testNow.Add(-time.Hour), "2026-09-22 10:00 UTC, committed 2026-09-24 11:00 UTC"},
		{"Jan _2 15:04", authored.Add(30 * time.Second), "Sep 22 10:00"},
	}
	for _, tt := range tests {
		dates := ui.NewDates(tt.format).In(time.UTC)
		if got := newFormat(testConfig(), dates, ui.Icons{}).dates(authored, tt.committed, testNow); got != tt.want {
			t.Errorf("%s: dates = %q, want %q", tt.format, got, tt.want)
		}
	}
}

// The ASCII set marks signatures in ASCII, in the rows and in the header.
func TestVerificationASCII(t *testing.T) {
	f := newFormat(testConfig(), ui.NewDates(config.DateRelative), ui.NewIcons(config.IconsASCII))
	tests := []struct {
		v          core.Verification
		mark, text string
	}{
		{core.Verification{Verified: true, Signed: true}, "+", "+ verified"},
		{core.Verification{Signed: true, Reason: "unknown_key"}, "x", "x unknown key"},
	}
	for _, tt := range tests {
		if got := f.verifiedMark(tt.v); got != tt.mark {
			t.Errorf("verifiedMark(%+v) = %q, want %q", tt.v, got, tt.mark)
		}
		if got, _, _ := f.verification(tt.v); got != tt.text {
			t.Errorf("verification(%+v) = %q, want %q", tt.v, got, tt.text)
		}
	}
}

func TestHeaderFields(t *testing.T) {
	same := history("main", 3)[1]
	same.Trailers = []core.Trailer{{Key: "Co-authored-by", Value: "Ayman Bagabas <ayman@example.com>"}}
	tests := []struct {
		name   string
		c      core.Commit
		detail []string
		email  bool
		want   []string
		absent []string
	}{
		{name: "committer other than the author", c: history("main", 3)[0],
			detail: []string{config.FieldAuthor, config.FieldCommitter},
			want:   []string{"Author    Kevin @kevin9327", "Committer GitHub @web-flow"}},
		// The same person shares one line, where the author is listed.
		{name: "same person", c: same, detail: []string{config.FieldCommitter, config.FieldAuthor},
			want: []string{"Author    Kevin @kevin9327"}, absent: []string{"Committer"}},
		{name: "committer alone", c: same, detail: []string{config.FieldCommitter},
			want: []string{"Committer Kevin @kevin9327"}, absent: []string{"Author"}},
		{name: "emails", c: same, detail: []string{config.FieldAuthor, config.FieldTrailers}, email: true,
			want: []string{"Kevin <kevin@example.com> @kevin9327", "Co-authored-by: Ayman Bagabas <ayman@example.com>"}},
		{name: "no emails", c: same, detail: []string{config.FieldTrailers},
			want: []string{"Trailers  Co-authored-by: Ayman Bagabas"}, absent: []string{"ayman@example.com"}},
		{name: "unverified", c: withVerification(same, core.Verification{Signed: true, Reason: "unknown_key"}),
			detail: []string{config.FieldVerification}, want: []string{"Signature " + ui.NewIcons(config.Default().UI.Icons).No + " unknown key"}},
		{name: "unsigned", c: same, detail: []string{config.FieldVerification}, want: []string{"Signature unsigned"}},
		{name: "merge", c: history("main", 6)[2], detail: []string{config.FieldParents, config.FieldSHA},
			want: []string{"Parents   " + short(sha("main", 3)) + " " + short(sha("main", 4)), "Commit    " + sha("main", 2)[:20]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.histories["main"] = []core.Commit{tt.c}
			f.details[tt.c.SHA] = detail(tt.c)
			cfg := testConfig()
			cfg.Detail, cfg.ShowEmail = tt.detail, tt.email
			m, _ := newModal(t, f, wideW, wideH, WithConfig(cfg))
			text := paneText(m, commitPane)
			for _, w := range tt.want {
				if !strings.Contains(text, strings.Join(strings.Fields(w), " ")) {
					t.Errorf("header lacks %q:\n%s", w, text)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(text, a) {
					t.Errorf("header has %q:\n%s", a, text)
				}
			}
		})
	}
}

func withVerification(c core.Commit, v core.Verification) core.Commit {
	c.Verification = v
	return c
}
