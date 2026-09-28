package notifications

import "github.com/eggzec/gh-tui/internal/config"

// configure passes the settings of c that the set command changed to the
// opener, which reads the threads ahead.
func (s *Section) configure(c config.Config) {
	s.opener.Configure(c)
}
