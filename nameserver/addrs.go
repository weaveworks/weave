package nameserver

import (
	"bytes"
	"fmt"
	"net"
)

// A zone record
// Note: we will try to keep all the zone records information is concentrated here. Maybe not all queries will
//       use things like priorities or weights, but we do not want to create a hierarchy for dealing with all
//       possible queries...
type ZoneRecord interface {
	Name() string  // The name for this IP
	IP() net.IP    // The IP (v4) address
	Priority() int // The priority
	Weight() int   // The weight
	TTL() int      // TTL
}

type ZoneLookup interface {
	// Lookup for a name
	LookupName(name string) ([]ZoneRecord, error)
	// Lookup for an address
	LookupInaddr(inaddr string) ([]ZoneRecord, error)
}

type ZoneObserver interface {
	// Observe anything that affects a particular name in the zone
	ObserveName(name string, observer ZoneRecordObserver) error
	// Observe anything that affects a particular IP in the zone
	ObserveInaddr(inaddr string, observer ZoneRecordObserver) error
}

type ZoneRecordObserver func()

/////////////////////////////////////////////////////////////

// A basic record struct (satisfying the ZoneRecord interface)
type Record struct {
	name     string
	ip       net.IP
	priority int
	weight   int
	ttl      int
}

func (r Record) Name() string  { return r.name }
func (r Record) IP() net.IP    { return r.ip }
func (r Record) Priority() int { return r.priority }
func (r Record) Weight() int   { return r.weight }
func (r Record) TTL() int      { return r.ttl }

func (r Record) String() string {
	var buf bytes.Buffer
	if len(r.Name()) > 0 {
		fmt.Fprintf(&buf, "%s", r.Name())
	}
	if !r.IP().IsUnspecified() {
		fmt.Fprintf(&buf, "[%s]", r.IP())
	}
	if r.Priority() > 0 {
		fmt.Fprintf(&buf, "/P:%d", r.Priority())
	}
	if r.Weight() > 0 {
		fmt.Fprintf(&buf, "/W:%d", r.Weight())
	}
	if r.TTL() > 0 {
		fmt.Fprintf(&buf, "/TTL:%d", r.TTL())
	}

	return buf.String()
}

/////////////////////////////////////////////////////////////

func newParseError(reason string, addr string) *net.ParseError {
	return &net.ParseError{Type: reason, Text: addr}
}

// simple IPv4 type that can be used as a key in a map (in contrast with net.IP), used for sets of IPs, etc...
type IPv4 [4]byte

func (ip IPv4) toNetIP() net.IP { return net.IPv4(ip[0], ip[1], ip[2], ip[3]) }
func (ip IPv4) String() string  { return ip.toNetIP().String() }
func ipToIPv4(nip net.IP) IPv4  { ip := nip.To4(); return IPv4([4]byte{ip[0], ip[1], ip[2], ip[3]}) }

// Parse an address (eg, "10.13.12.11") and return the corresponding IPv4 (eg, "10.13.12.11")
func addrToIPv4(addr string) (IPv4, error) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return IPv4{}, newParseError("IP address", addr)
	}
	return ipToIPv4(ip), nil
}

// Parse a reverse address (eg, "11.12.13.10.in-addr.arpa.") and return the corresponding IPv4 (eg, "10.13.12.11")
func raddrToIPv4(addr string) (IPv4, error) {
	l := len(addr)
	if l < rdnsDomainLen+1 {
		return IPv4{}, newParseError("too short reverse IP address", addr)
	}

	suffixLen := l - rdnsDomainLen
	suffix := addr[suffixLen+1:]
	if suffix != RDNSDomain {
		return IPv4{}, newParseError("suffix of reverse IP address", addr)
	}
	ipStr := addr[:suffixLen]
	revIP := net.ParseIP(ipStr)
	if revIP == nil {
		return IPv4{}, newParseError("reverse IP address", addr)
	}

	revIP4 := revIP.To4()
	return IPv4([4]byte{revIP4[3], revIP4[2], revIP4[1], revIP4[0]}), nil
}

func raddrToIP(addr string) (net.IP, error) {
	r, err := raddrToIPv4(addr)
	if err != nil {
		return net.IP{}, err
	}
	return r.toNetIP(), nil
}
