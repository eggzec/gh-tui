package config

// Notifications configures the notifications screen and the dashboard's
// inbox.
type Notifications struct {
	// MarkReadOnOpen marks a thread read when the select key opens what
	// it is about, in the app or in the browser. The open key, which opens
	// the thread on GitHub, never marks it read.
	MarkReadOnOpen bool `yaml:"mark_read_on_open"`
}

func defaultNotifications() Notifications {
	return Notifications{MarkReadOnOpen: true}
}
