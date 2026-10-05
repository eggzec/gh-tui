package owners

import (
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Kept lists the entries that the service keeps with validators, for a
// revalidator to check in the background, each check a conditional
// request that costs no rate limit when nothing changed: the profile
// READMEs. A README that changed is cached and kept, and reports SyncKey
// of its account. The other reads are GraphQL reads, or REST reads the
// client gives no validators for, so they are read again once their TTL
// passes instead and none of them is listed. It reads the store, so call
// it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	return recheck.Entries(s.readme.kept, kindReadme, s.ttls.Readme, s.readmeTarget)
}
