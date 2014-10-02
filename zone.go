package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"sync"
)

type Zone interface {
	AddRecord(name string, ip string, weave_ip string) error
	MatchLocal(name string) (string, error)
	MatchGlobal(name string) (string, error)
}

type Record struct {
	Name    string
	Ip      string
	WeaveIp string
}

type ZoneDb struct {
	mx   sync.RWMutex
	recs []Record
}

type LookupError string

func (ops LookupError) Error() string {
	return string(ops)
}

// Stop gap.
func (zone ZoneDb) match(name string) (string, error) {
	for _, r := range zone.recs {
		log.Printf("%s == %s ?", r.Name, name)
		if r.Name == name {
			return r.WeaveIp, nil
		}
	}
	return "", LookupError(name)
}

func (zone *ZoneDb) MatchLocal(name string) (string, error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	return zone.match(name)
}

func (zone *ZoneDb) MatchGlobal(name string) (string, error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	return zone.match(name)
}

func (zone *ZoneDb) AddRecord(name string, ip string, weave_ip string) error {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	zone.recs = append(zone.recs, Record{dns.Fqdn(name), ip, weave_ip})
	return nil
}
