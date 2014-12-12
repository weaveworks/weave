package nameserver

import (
	"github.com/miekg/dns"
	"net"
	"sync"
)

type Zone interface {
	AddRecord(ident string, name string, ip net.IP) error
	DeleteRecord(ident string, ip net.IP) error
	DeleteRecordsFor(ident string) error
	LookupLocal(name string) ([]net.IP, error)
	ReverseLookupLocal(ip net.IP) (string, error)
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

func (zone *ZoneDb) LookupLocal(name string) (res []net.IP, err error) {
	err = nil
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	for _, r := range zone.recs {
		if r.Name == name {
			res = append(res, r.IP)
		}
	}

	if len(res) == 0 {
		err = LookupError(name)
	}
	return
}

func (zone *ZoneDb) ReverseLookupLocal(ip net.IP) (string, error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	for _, r := range zone.recs {
		if r.IP.Equal(ip) {
			return r.Name, nil
		}
	}
	return "", LookupError(ip.String())
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
