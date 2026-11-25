package config

import "errors"

// Conversation configures the conversation of a pull request or an issue.
type Conversation struct {
	// Filter lists the kinds of events the conversation shows when it opens,
	// by name, from ConversationGroups. The others are hidden until the
	// filter is changed there.
	Filter []string `yaml:"filter"`
}

// ConversationGroups are the names of the kinds of events that the
// conversation of a pull request or an issue can show or hide. The hidden
// ones, which GitHub minimized, show only when named.
var ConversationGroups = []string{
	"comments", "reviews", "pushes", "labels", "people", "state", "branch", "references", "milestones", "hidden",
}

func (c Conversation) validate(path string) error {
	var errs []error
	if len(c.Filter) == 0 {
		errs = append(errs, errors.New(path+".filter: needs at least one kind of event"))
	}
	return errors.Join(append(errs, validateFields(path+".filter", c.Filter, ConversationGroups))...)
}
