package nameserver

import "net"

// A basic mDNS service
type ZoneMDNS interface {
	// Start the service
	Start(*net.Interface) error
	// Stop the service
	Stop() error
}

// A mDNS server
type ZoneMDNSServer interface {
	ZoneMDNS
	// Return the Zone database used by the server
	Zone() Zone
}

// A mDNS client interface
type ZoneMDNSClient interface {
	ZoneMDNS
	ZoneLookup
	// Perform an insistent lookup for a name
	InsistentLookupName(name string) ([]ZoneRecord, error)
	// Perform an insistent lookup for a reverse address
	InsistentLookupInaddr(inaddr string) ([]ZoneRecord, error)
}
