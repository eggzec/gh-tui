package pulls_test

import (
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// The GitHub client is the API the service runs on in the app.
var _ pulls.API = (*github.Client)(nil)
