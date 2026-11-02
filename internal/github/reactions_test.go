package github

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestTimelineReactions(t *testing.T) {
	var n timelineNode
	src := `{"__typename":"IssueComment","id":"IC_1","body":"b","reactionGroups":[
		{"content":"THUMBS_UP","reactors":{"totalCount":3}},
		{"content":"HEART","reactors":{"totalCount":0}},
		{"content":"EYES","reactors":{"totalCount":1}}]}`
	if err := json.Unmarshal([]byte(src), &n); err != nil {
		t.Fatal(err)
	}
	it, ok := n.core()
	if !ok {
		t.Fatal("comment dropped")
	}
	want := []core.Reaction{{Content: "THUMBS_UP", Count: 3}, {Content: "EYES", Count: 1}}
	if !reflect.DeepEqual(it.Reactions, want) {
		t.Errorf("Reactions = %+v, want %+v", it.Reactions, want)
	}
}

func TestIssueCommentReactions(t *testing.T) {
	var c issueComment
	src := `{"node_id":"IC_1","body":"b","reactions":{"total_count":5,"+1":2,"-1":0,"heart":3}}`
	if err := json.Unmarshal([]byte(src), &c); err != nil {
		t.Fatal(err)
	}
	want := []core.Reaction{{Content: "THUMBS_UP", Count: 2}, {Content: "HEART", Count: 3}}
	if got := c.core().Reactions; !reflect.DeepEqual(got, want) {
		t.Errorf("Reactions = %+v, want %+v", got, want)
	}
}
