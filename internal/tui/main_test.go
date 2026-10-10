package tui

import (
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dlclark/regexp2/v2"

	"github.com/eggzec/gh-tui/internal/keyname"
)

func TestMain(m *testing.M) {
	startRegexpClock()
	// Many tests run the app's first commands to the end, and one of them
	// waits for the terminal's version, which a test never gets. They wait
	// a moment instead of the second the program does.
	terminalWait = time.Millisecond
	os.Exit(m.Run())
}

// startRegexpClock starts the clock that regexp2 times its matches by,
// set to run for longer than the tests do. Chroma's lexers match with a
// time limit, and regexp2 starts its clock, a goroutine shared by the
// whole process, when a match's limit falls past the clock's end. Started
// in a synctest bubble, that goroutine would keep the bubble from ending,
// and it keeps the fake time it saw there after the bubble ends, which
// confuses the next bubble's matches. Started here, outside any bubble,
// it runs on the wall clock and covers every limit the lexers set, so no
// match starts it again and the limits keep holding.
func startRegexpClock() {
	re := regexp2.MustCompile(".", regexp2.None)
	re.MatchTimeout = 365 * 24 * time.Hour
	if _, err := re.MatchString("a"); err != nil {
		panic(err)
	}
}

// keyPress returns a press of the key named name, as the config and
// bubbletea name keys, and false for a name it can't press.
func keyPress(name string) (tea.KeyPressMsg, bool) { return keyname.Press(name) }
