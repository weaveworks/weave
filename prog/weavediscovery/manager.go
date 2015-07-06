package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/docker/machine/drivers/vmwarevsphere/errors"
	"github.com/docker/swarm/discovery"
	_ "github.com/docker/swarm/discovery/file"
	_ "github.com/docker/swarm/discovery/kv"
	_ "github.com/docker/swarm/discovery/nodes"
	_ "github.com/docker/swarm/discovery/token"
	. "github.com/weaveworks/weave/common"
)

var (
	errInvalidEndpoint = errors.New("Invalid endpoint")
)

type discoveryEndpoint struct {
	discovery.Discovery

	url      string
	stopChan chan struct{}

	// stats
	added        uint64
	removed      uint64
	lastRegister time.Time
}

// Create a new endpoint
func newDiscoveryEndpoint(url, localAddr string, weaveCli *WeaveClient, heartbeat time.Duration, ttl time.Duration) (*discoveryEndpoint, error) {
	d, err := discovery.New(url, heartbeat, ttl)
	if err != nil {
		return nil, err
	}
	stopChan := make(chan struct{})
	ep := discoveryEndpoint{
		Discovery: d,
		url:       url,
		stopChan:  stopChan,
	}

	entriesChan, errorsChan := d.Watch(stopChan)
	ticker := time.NewTicker(heartbeat)
	go func() {
		register := func() {
			Log.Debugf("[manager] Registering on '%s' we are at '%s' (%s period)...", url, localAddr, heartbeat)
			if err := d.Register(localAddr); err != nil {
				Log.Warningf("[manager] Registration failed: %s", err)
			} else {
				ep.lastRegister = time.Now()
			}
		}

		register()
		currentEntries := discovery.Entries{}
		for {
			select {
			case reportedEntries := <-entriesChan:
				added, removed := currentEntries.Diff(reportedEntries)

				ep.added += uint64(len(added))
				ep.removed += uint64(len(removed))

				currentEntries = reportedEntries
				Log.Printf("[manager] Updates from '%s': %d added, %d removed...", url, len(added), len(removed))
				for _, e := range added {
					weaveCli.Join(e.Host, e.Port)
				}
				for _, e := range removed {
					weaveCli.Forget(e.Host, e.Port)
				}
			case reportedError := <-errorsChan:
				Log.Warningf("[manager] Error from endpoint %s: %s...", url, reportedError)
			case <-ticker.C:
				register()
			case <-stopChan:
				ticker.Stop()
				return
			}
		}
	}()

	return &ep, nil
}

func (d *discoveryEndpoint) Disconnect() {
	close(d.stopChan)
}

func (d *discoveryEndpoint) String() string {
	regStr := "!REGISTERED"
	if !d.lastRegister.IsZero() {
		regStr = fmt.Sprintf("register@%s", d.lastRegister.Format("15:04:05.99"))
	}
	return fmt.Sprintf("%s [%d++/%d--/%s]", d.url, d.added, d.removed, regStr)
}

///////////////////////////////////////////////////////////////////////////

type DiscoveryManager struct {
	hb int

	localAddr string
	weaveCli  *WeaveClient
	endpoints map[string]*discoveryEndpoint
}

func NewDiscoveryManager(localAddr string, weaveCli *WeaveClient) *DiscoveryManager {
	dm := DiscoveryManager{
		localAddr: localAddr,
		weaveCli:  weaveCli,
		endpoints: make(map[string]*discoveryEndpoint),
	}
	return &dm
}

// Start the discovery manager
func (dm *DiscoveryManager) Start() {
}

// Stop the discovery manager
func (dm *DiscoveryManager) Stop() error {
	for _, d := range dm.endpoints {
		d.Disconnect()
	}
	return nil
}

func (dm *DiscoveryManager) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Endpoints:")
	for _, v := range dm.endpoints {
		fmt.Fprintln(&buf, " -", v)
	}
	return buf.String()
}

// Join a discovery endpoint
func (dm *DiscoveryManager) Join(url string, heartbeat time.Duration, ttl time.Duration) error {
	if _, found := dm.endpoints[url]; found {
		Log.Debugf("[manager] Endpoint %s already joined: ignored", url)
		return nil
	}

	Log.Debugf("[manager] Joining '%s'...", url)
	d, err := newDiscoveryEndpoint(url, dm.localAddr, dm.weaveCli, heartbeat, ttl)
	if err != nil {
		return err
	}
	Log.Debugf("[manager] '%s' successfully joined", url)
	dm.endpoints[url] = d
	return nil
}

// Leave a discovery endpoint
func (dm *DiscoveryManager) Leave(url string) error {
	if d, found := dm.endpoints[url]; found {
		Log.Debugf("[manager] Leaving %s", url)
		d.Disconnect()
		delete(dm.endpoints, url)
		return nil
	}
	return errInvalidEndpoint
}
