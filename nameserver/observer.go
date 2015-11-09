package nameserver

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/miekg/dns"

	"github.com/weaveworks/weave/net/address"
)

var (
	errUnknownName = errors.New("unknown name")
)

func fqdnWithDomain(hostname string, domain string) string {
	fqdnName := dns.Fqdn(hostname)
	if !strings.HasSuffix(fqdnName, domain) { // "x." -> "x.weave.local.", "x.y." -> "x.y.weave.local."
		fqdnName = fqdnName + domain
	}
	return fqdnName
}

// Observer is a name observer
type Observer struct {
	ID    string
	Last  []address.Address
	Addrs chan []address.Address
}

// NewObserver is an Observer constructor
func NewObserver(id string) *Observer {
	return &Observer{ID: id, Addrs: make(chan []address.Address)}
}

// Notify is a observer notifier
func (obs *Observer) Notify(addrs []address.Address) {
	if !reflect.DeepEqual(obs.Last, addrs) {
		obs.Addrs <- addrs
		obs.Last = make([]address.Address, len(addrs))
		copy(obs.Last, addrs)
	}
}

// Observers is the list of names that are being observed
type Observers struct {
	sync.RWMutex
	ns  *Nameserver
	obs map[string][]*Observer
}

// NewObservers is a Observers constructor
func NewObservers(ns *Nameserver) *Observers {
	return &Observers{
		ns:  ns,
		obs: make(map[string][]*Observer),
	}
}

// Observe starts observing a name
func (nobs *Observers) Observe(hostname, id string) (chan []address.Address, error) {
	nobs.Lock()
	defer nobs.Unlock()

	hostname = fqdnWithDomain(hostname, nobs.ns.domain)

	nobs.ns.debugf("%s is observing %s", id, hostname)
	observer := NewObserver(id)
	if _, found := nobs.obs[hostname]; found {
		// TODO: check the (hostname, id) does not exist
		nobs.obs[hostname] = append(nobs.obs[hostname], observer)
	} else {
		nobs.obs[hostname] = []*Observer{observer}
	}
	return observer.Addrs, nil
}

// Forget disconnects an observer from a name
func (nobs *Observers) Forget(hostname, id string) {
	nobs.Lock()
	defer nobs.Unlock()

	hostname = fqdnWithDomain(hostname, nobs.ns.domain)

	observers, found := nobs.obs[hostname]
	if !found {
		return
	}

	// filter out the `id`
	var newObservers []*Observer
	for _, v := range observers {
		if v.ID != id {
			newObservers = append(newObservers, v)
		}
	}
	nobs.obs[hostname] = newObservers
}

// Notify is a observers notifier
func (nobs *Observers) Notify() {
	nobs.Lock()
	defer nobs.Unlock()

	// we do not expect many observers (probably just a couple), so we just
	// perform a lookups for all observed names...
	// TODO: implement a proper observation mechanism
	for hostname, observers := range nobs.obs {
		for _, observer := range observers {
			nobs.ns.debugf("Notifying observers of %s", hostname)
			observer.Notify(nobs.ns.Lookup(hostname))
		}
	}
}
