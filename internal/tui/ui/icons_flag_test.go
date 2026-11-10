package ui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestIconsFlagPriority(t *testing.T) {
	ic := NewIcons(config.IconsASCII)
	tests := []struct {
		name string
		r    core.Repo
		want string
	}{
		{"none", core.Repo{}, ""},
		{"private over fork", core.Repo{Private: true, Fork: true, Template: true}, ic.Private},
		{"fork over template", core.Repo{Fork: true, Template: true, Archived: true}, ic.Fork},
		{"template over archived", core.Repo{Template: true, Archived: true, Mirror: true}, ic.Template},
		{"archived over mirror", core.Repo{Archived: true, Mirror: true}, ic.Archived},
		{"mirror", core.Repo{Mirror: true}, ic.Mirror},
	}
	for _, tt := range tests {
		if got := ic.Flag(tt.r); got != tt.want {
			t.Errorf("%s: Flag = %q, want %q", tt.name, got, tt.want)
		}
	}
}
