// Package uitest holds fixtures that the tests of several sections share.
package uitest

import (
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// bodies are comments as GitHub sends them, with CRLF line endings, that
// hold between them what bodies on GitHub hold.
var bodies = []string{
	// A filled-in template: comments, headings, lines broken by hand and
	// a task list.
	"<!-- Please fill in the sections below. -->\r\n" +
		"### What happened\r\n" +
		"The modal shows **raw** markdown\r\n" +
		"instead of rendering it.\r\n\r\n" +
		"### Checklist\r\n" +
		"- [x] I searched the issues\r\n" +
		"- [ ] I tried `main`\r\n",
	// Code, links, a mention and a reference.
	"Looks like `renderComment` wraps the body as plain text, see #12 and " +
		"[the docs](https://github.com/charmbracelet/glamour). cc @mona\r\n\r\n" +
		"```go\r\nfunc render(body string) string {\r\n\treturn ansi.Wrap(body, 80, \"\")\r\n}\r\n```\r\n",
	// A table and a quote.
	"> the comments inside the current issue and pr are not rendering\r\n\r\n" +
		"| Where | Before | After |\r\n|---|---|---|\r\n| issues | glamour | shared |\r\n| pulls | plain | shared |\r\n",
	// Images and a collapsed section.
	"![screenshot](https://github.com/user-attachments/assets/1234)\r\n" +
		"<img width=\"480\" alt=\"light theme\" src=\"https://github.com/user-attachments/assets/5678\">\r\n\r\n" +
		"<details><summary>Stack trace</summary>\r\n\r\n" +
		"```\r\npanic: runtime error\r\n\tmain.go:12\r\n```\r\n\r\n</details>\r\n",
	// A diagram, and escape sequences, which must not reach the terminal.
	"```mermaid\r\ngraph LR\r\n  A[issue] --> B[modal]\r\n```\r\n\r\n" +
		"Title \x1b]0;pwned\x07 and \x1b[31mred\x1b[0m text.\r\n",
}

// Comments returns one comment for each kind of body, oldest first, the
// last one an hour before now.
func Comments(now time.Time) []core.Comment {
	return Thread(len(bodies), now)
}

// Thread returns n comments, oldest first, the last one an hour before
// now, whose bodies go round the kinds that Comments has.
func Thread(n int, now time.Time) []core.Comment {
	authors := []string{"hubot", "octocat", "monalisa", "defunkt"}
	out := make([]core.Comment, 0, n)
	for i := range n {
		at := now.Add(-time.Duration(n-i) * time.Hour)
		out = append(out, core.Comment{
			ID:        "IC_" + strconv.Itoa(i),
			Author:    core.User{Login: authors[i%len(authors)]},
			AvatarURL: "https://avatars.githubusercontent.com/u/" + authors[i%len(authors)] + "?v=4",
			Body:      bodies[i%len(bodies)],
			CreatedAt: at,
			UpdatedAt: at,
		})
	}
	return out
}
