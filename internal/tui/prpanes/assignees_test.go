package prpanes

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestAssigneeDiff(t *testing.T) {
	t.Parallel()
	people := map[string]core.User{"b": {Login: "b"}, "a": {Login: "a"}, "z": {Login: "z"}}
	add, remove := AssigneeDiff(
		map[string]bool{"b": true, "a": true, "keep": true},
		map[string]bool{"keep": true, "z": true, "y": true},
		people,
	)
	if len(add) != 2 || add[0].Login != "a" || add[1].Login != "b" {
		t.Fatalf("add = %v, want a then b", add)
	}
	if len(remove) != 2 || remove[0] != "y" || remove[1] != "z" {
		t.Fatalf("remove = %v, want y then z", remove)
	}
	if add, remove := AssigneeDiff(map[string]bool{"a": true}, map[string]bool{"a": true}, people); add != nil || remove != nil {
		t.Fatalf("no change gave add %v remove %v", add, remove)
	}
}
