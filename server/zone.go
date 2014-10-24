package weavedns

import (
	"github.com/miekg/dns"
	"net"
	"sync"
)

type Zone interface {
	AddRecord(ident string, name string, localIP net.IP, weaveIP net.IP) error
	DeleteRecord(ident string, weaveIp net.IP) error
	DeleteRecordsFor(ident string) error
	MatchLocal(name string) (net.IP, error)
	MatchLocalIP(ip net.IP) (string, error)
}

type Record struct {
	Ident   string
	Name    string
	LocalIP net.IP
	WeaveIP net.IP
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
	Name    string
	WeaveIP net.IP
	Ident   string
}

func (err DuplicateError) Error() string {
	return "Duplicate " + err.Name + "," + err.WeaveIP.String() + " for identity " + err.Ident
}

func (zone *ZoneDb) indexOf(match func(Record) bool) int {
	for i, r := range zone.recs {
		if match(r) {
			return i
		}
	}
	return -1
}

func (zone *ZoneDb) MatchLocal(name string) (net.IP, error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	for _, r := range zone.recs {
		if r.Name == name {
			return r.WeaveIP, nil
		}
	}
	return nil, LookupError(name)
}

func (zone *ZoneDb) MatchLocalIP(ip net.IP) (string, error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	for _, r := range zone.recs {
		if r.WeaveIP.Equal(ip) {
			return r.Name, nil
		}
	}
	return "", LookupError(ip.String())
}

func (zone *ZoneDb) AddRecord(ident string, name string, localIP net.IP, weaveIP net.IP) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	fqdn := dns.Fqdn(name)
	if index := zone.indexOf(
		func(r Record) bool { return r.Name == fqdn && r.WeaveIP.Equal(weaveIP) }); index != -1 {
		return DuplicateError{fqdn, weaveIP, zone.recs[index].Ident}
	}
	zone.recs = append(zone.recs, Record{ident, fqdn, localIP, weaveIP})
	return nil
}

func (zone *ZoneDb) DeleteRecord(ident string, weaveIP net.IP) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	if index := zone.indexOf(
		func(r Record) bool { return r.Ident == ident && r.WeaveIP.Equal(weaveIP) }); index == -1 {
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
