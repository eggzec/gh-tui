package owners

import "github.com/eggzec/gh-tui/internal/revalidate"

// Kept lists the entries that the service keeps with validators, for a
// revalidator to check in the background, each check a conditional
// request that costs no rate limit when nothing changed. The header, the
// repositories and the calendar are GraphQL reads, which have no
// validators, so they are read again once their TTL passes instead and
// none of them is listed; reads over REST, which have, add theirs here.
func (s *Service) Kept() []revalidate.Entry {
	return nil
}
