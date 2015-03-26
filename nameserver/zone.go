package nameserver

import (
	"bytes"
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
	"sync"
)

const (
	RDNS_DOMAIN = "in-addr.arpa."
)

// +1 to also exclude a dot
var rdnsDomainLen = len(RDNS_DOMAIN) + 1

type Lookup interface {
	LookupName(name string) (net.IP, error)
	LookupInaddr(inaddr string) (string, error)
}

type Zone interface {
	AddRecord(ident string, name string, ip net.IP) error
	DeleteRecord(ident string, ip net.IP) error
	DeleteRecordsFor(ident string) error
	Lookup
}

type Record struct {
	Ident string
	Name  string
	IP    net.IP
}

// Very simple data structure for now, with linear searching.
// TODO: make more sophisticated to improve performance.
type ZoneDb struct {
	mx   sync.RWMutex
	recs []Record
}

type LookupError string

func (ops LookupError) Error() string {
	return "Unable to find " + string(ops)
}

type DuplicateError struct {
}

func (dup DuplicateError) Error() string {
	return "Tried to add a duplicate entry"
}

func (zone *ZoneDb) indexOf(match func(Record) bool) int {
	for i, r := range zone.recs {
		if match(r) {
			return i
		}
	}
	return -1
}

func (zone *ZoneDb) String() string {
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	var buf bytes.Buffer
	for _, r := range zone.recs {
		fmt.Fprintf(&buf, "%.12s %s %v\n", r.Ident, r.IP, r.Name)
	}
	return buf.String()
}

func (zone *ZoneDb) LookupName(name string) (net.IP, error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	for _, r := range zone.recs {
		if r.Name == name {
			return r.IP, nil
		}
	}
	return nil, LookupError(name)
}

func (zone *ZoneDb) LookupInaddr(inaddr string) (string, error) {
	if revIP := net.ParseIP(inaddr[:len(inaddr)-rdnsDomainLen]); revIP != nil {
		revIP4 := revIP.To4()
		ip := []byte{revIP4[3], revIP4[2], revIP4[1], revIP4[0]}
		Debug.Printf("[zonedb] Looking for address: %+v", ip)
		zone.mx.RLock()
		defer zone.mx.RUnlock()
		for _, r := range zone.recs {
			if r.IP.Equal(ip) {
				return r.Name, nil
			}
		}
		return "", LookupError(inaddr)
	} else {
		Warning.Printf("[zonedb] Asked to reverse lookup %s", inaddr)
		return "", LookupError(inaddr)
	}
}

func (zone *ZoneDb) AddRecord(ident string, name string, ip net.IP) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	fqdn := dns.Fqdn(name)
	if index := zone.indexOf(
		func(r Record) bool {
			return r.Name == fqdn && r.IP.Equal(ip) && r.Ident == ident
		}); index != -1 {
		return DuplicateError{}
	}
	zone.recs = append(zone.recs, Record{ident, fqdn, ip})
	return nil
}

func (zone *ZoneDb) DeleteRecord(ident string, ip net.IP) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	if index := zone.indexOf(
		func(r Record) bool {
			return r.Ident == ident && r.IP.Equal(ip)
		}); index == -1 {
		return LookupError(ident)
	} else {
		zone.recs = append(zone.recs[:index], zone.recs[index+1:]...)
	}
	return nil
}

func (zone *ZoneDb) DeleteRecordsFor(ident string) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	w := 0 // write index

	for _, r := range zone.recs {
		if r.Ident != ident {
			zone.recs[w] = r
			w++
		}
	}
	zone.recs = zone.recs[:w]
	return nil
}
