package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"net"
	"sync"
)

type Zone interface {
	AddRecord(string, string, net.IP, net.IP, *net.IPNet) error
	MatchLocal(string) (net.IP, error)
}

type Record struct {
	Ident   string
	Name    string
	Ip      net.IP
	WeaveIp net.IP
	Subnet  *net.IPNet
}

type ZoneDb struct {
	mx   sync.RWMutex
	recs []Record
}

type LookupError string

func (ops LookupError) Error() string {
	return "Unable to find " + string(ops)
}

type DuplicateError struct {
	Name    string
	WeaveIp net.IP
	Ident   string
}

func (err DuplicateError) Error() string {
	return "Duplicate " + err.Name + "," + err.WeaveIp.String() + " in container " + err.Ident
}

// Stop gap.
func (zone *ZoneDb) match(name string) (net.IP, error) {
	for _, r := range zone.recs {
		log.Printf("%s == %s ?", r.Name, name)
		if r.Name == name {
			return r.WeaveIp, nil
		}
	}
	return nil, LookupError(name)
}

func (zone *ZoneDb) indexOfNameAddr(name string, addr net.IP) int {
	for i, r := range zone.recs {
		if r.Name == name && r.WeaveIp.Equal(addr) {
			return i
		}
	}
	return -1
}

func (zone *ZoneDb) MatchLocal(name string) (net.IP, error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	return zone.match(name)
}

func (zone *ZoneDb) AddRecord(identifier string, name string, ip net.IP, weave_ip net.IP, weave_subnet *net.IPNet) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	fqdn := dns.Fqdn(name)
	if index := zone.indexOfNameAddr(fqdn, weave_ip); index != -1 {
		return DuplicateError{fqdn, weave_ip, zone.recs[index].Ident}
	}
	zone.recs = append(zone.recs, Record{identifier, fqdn, ip, weave_ip, weave_subnet})
	return nil
}
