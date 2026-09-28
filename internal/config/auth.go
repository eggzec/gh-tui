package config

// Auth configures what the app checks of the token.
type Auth struct {
	// Check refuses at once what the token is known to lack a scope for,
	// rather than asking GitHub, and asks GitHub what the token may do
	// when no answer said so soon after the start. Turn it off if the app
	// ever refuses what the token may do.
	Check bool `yaml:"check"`
}

func defaultAuth() Auth {
	return Auth{Check: true}
}
