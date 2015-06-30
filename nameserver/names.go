package nameserver

import (
	"strings"

	"github.com/miekg/dns"
)

// Get the number of components of a name
// For example, "redis" is 1, "redis.weave.local"  is 3...
func nameNumComponents(name string) int {
	if len(name) == 0 || name == "." {
		return 0
	}
	return strings.Count(dns.Fqdn(name), ".")
}

// normalize a name by
// a) adding the local domain when it is a single component name
// b) converting the result to FQDN
func normalizeName(n string, domain string) string {
	// we should never add anything to a FQDN like "redis.", but that's the only thing that
	// some broken resolvers will do: to convert the name to FQDN...
	if nameNumComponents(n) == 1 {
		n = dns.Fqdn(n) + domain
	}
	return dns.Fqdn(n)
}
