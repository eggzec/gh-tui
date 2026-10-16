package config

import "errors"

// Conversation configures the conversation of a pull request.
type Conversation struct {
	// Filter lists the kinds of events the conversation shows when it opens,
	// by name, from ConversationGroups. The others are hidden until the
	// filter is changed there.
	Filter []string `yaml:"filter"`
}

// ConversationGroups are the names of the kinds of events that the
// conversation of a pull request can show or hide.
var ConversationGroups = []string{
	"comments", "reviews", "pushes", "labels", "people", "state", "branch", "references", "milestones",
}

func (c Conversation) validate() error {
	var errs []error
	if len(c.Filter) == 0 {
		errs = append(errs, errors.New("ui.conversation.filter: needs at least one kind of event"))
	}
	return errors.Join(append(errs, validateFields("ui.conversation.filter", c.Filter, ConversationGroups))...)
}
