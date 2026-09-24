package core

import (
	"slices"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		subject  string
		body     string
		trailers []Trailer
	}{
		{name: "empty"},
		{name: "subject only", message: "fix: a bug\n", subject: "fix: a bug"},
		{
			name:    "folded subject",
			message: "feat: a subject\nthat goes on\n\nThe body.",
			subject: "feat: a subject that goes on",
			body:    "The body.",
		},
		{
			name:    "body and trailers",
			message: "feat: history\n\nWhy it matters.\n\nMore of it.\n\nSigned-off-by: A U Thor <a@example.com>\nCo-authored-by: B <b@example.com>\n",
			subject: "feat: history",
			body:    "Why it matters.\n\nMore of it.",
			trailers: []Trailer{
				{Key: "Signed-off-by", Value: "A U Thor <a@example.com>"},
				{Key: "Co-authored-by", Value: "B <b@example.com>"},
			},
		},
		{
			name:     "trailers without a body",
			message:  "fix: x\n\nReviewed-by: C <c@example.com>",
			subject:  "fix: x",
			trailers: []Trailer{{Key: "Reviewed-by", Value: "C <c@example.com>"}},
		},
		{
			name:    "folded value",
			message: "fix: x\n\nBody.\n\nChange-Id: I123\nNote: a long value\n  that goes on\n\tand on\nTested-by:\n",
			subject: "fix: x",
			body:    "Body.",
			trailers: []Trailer{
				{Key: "Change-Id", Value: "I123"},
				{Key: "Note", Value: "a long value that goes on and on"},
				{Key: "Tested-by", Value: ""},
			},
		},
		{
			name:    "last paragraph isn't trailers",
			message: "fix: x\n\nBody.\n\nFixes: #12\nand some prose after it.",
			subject: "fix: x",
			body:    "Body.\n\nFixes: #12\nand some prose after it.",
		},
		{
			name:    "a URL isn't a trailer",
			message: "docs: link\n\nhttps://example.com/a",
			subject: "docs: link",
			body:    "https://example.com/a",
		},
		{
			name:    "key with a space",
			message: "fix: x\n\nSee also: the docs",
			subject: "fix: x",
			body:    "See also: the docs",
		},
		{
			name:    "continuation first",
			message: "fix: x\n\n  indented: text",
			subject: "fix: x",
			body:    "  indented: text",
		},
		{
			name:    "the subject is never trailers",
			message: "Revert: something",
			subject: "Revert: something",
		},
		{
			name:     "CRLF and blank lines",
			message:  "fix: x\r\n\r\n\r\nBody.\r\n\r\nAcked-by: D\r\n\r\n",
			subject:  "fix: x",
			body:     "Body.",
			trailers: []Trailer{{Key: "Acked-by", Value: "D"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, body, trailers := SplitMessage(tt.message)
			if subject != tt.subject || body != tt.body || !slices.Equal(trailers, tt.trailers) {
				t.Errorf("SplitMessage(%q) =\n%q, %q, %v\nwant\n%q, %q, %v", tt.message, subject, body, trailers, tt.subject, tt.body, tt.trailers)
			}
		})
	}
}

func TestCommitMerge(t *testing.T) {
	if (Commit{Parents: []string{"a"}}).Merge() {
		t.Error("a commit with one parent is a merge")
	}
	if !(Commit{Parents: []string{"a", "b"}}).Merge() {
		t.Error("a commit with two parents isn't a merge")
	}
}
