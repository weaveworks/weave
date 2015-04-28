package nameserver

import (
	. "github.com/weaveworks/weave/common"
	"net"
	"sync"
)

const (
	testSocketTimeout = 100 // in millisecs
)

// Warn about some methods that some day should be implemented...
func notImplWarn() { Warning.Printf("Mocked method. Not implemented.") }

// A mocked Zone that always returns the same records
// * it does not send/receive any mDNS query
type mockedZoneWithRecords struct {
	sync.RWMutex

	records []ZoneRecord

	// Statistics
	NumLookupsName   int
	NumLookupsInaddr int
}

func newMockedZoneWithRecords(zr []ZoneRecord) *mockedZoneWithRecords {
	return &mockedZoneWithRecords{records: zr}
}
func (mz *mockedZoneWithRecords) Domain() string { return DefaultLocalDomain }
func (mz *mockedZoneWithRecords) LookupName(name string) ([]ZoneRecord, error) {
	Debug.Printf("[mocked zone]: LookupName: returning records %s", mz.records)
	mz.Lock()
	defer mz.Unlock()

	mz.NumLookupsName += 1
	res := make([]ZoneRecord, 0)
	for _, r := range mz.records {
		if r.Name() == name {
			res = append(res, r)
		}
	}
	return res, nil
}

func (mz *mockedZoneWithRecords) LookupInaddr(inaddr string) ([]ZoneRecord, error) {
	Debug.Printf("[mocked zone]: LookupInaddr: returning records %s", mz.records)
	mz.Lock()
	defer mz.Unlock()

	mz.NumLookupsInaddr += 1
	res := make([]ZoneRecord, 0)
	for _, r := range mz.records {
		revIP, err := raddrToIP(inaddr)
		if err != nil {
			return nil, newParseError("lookup address", inaddr)
		}
		if r.IP().Equal(revIP) {
			res = append(res, r)
		}
	}
	return res, nil
}

func (mz *mockedZoneWithRecords) DomainLookupName(name string) ([]ZoneRecord, error) {
	return mz.LookupName(name)
}
func (mz *mockedZoneWithRecords) DomainLookupInaddr(inaddr string) ([]ZoneRecord, error) {
	return mz.LookupInaddr(inaddr)
}

// the following methods are not currently needed...
func (mz *mockedZoneWithRecords) AddRecord(ident string, name string, ip net.IP) error {
	notImplWarn()
	return nil
}
func (mz *mockedZoneWithRecords) DeleteRecord(ident string, ip net.IP) error {
	notImplWarn()
	return nil
}
func (mz *mockedZoneWithRecords) DeleteRecordsFor(ident string) error { notImplWarn(); return nil }
func (mz *mockedZoneWithRecords) Status() string                      { notImplWarn(); return "nothing" }
func (mz *mockedZoneWithRecords) ObserveName(name string, observer ZoneRecordObserver) error {
	notImplWarn()
	return nil
}
func (mz *mockedZoneWithRecords) ObserveInaddr(inaddr string, observer ZoneRecordObserver) error {
	notImplWarn()
	return nil
}
