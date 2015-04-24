package nameserver

import (
	. "github.com/weaveworks/weave/common"
	"net"
)

// Warn about some methods that some day should be implemented...
func notImplWarn() { Warning.Printf("Mocked method. Not implemented.") }

// A mocked Zone that always returns the same record
type mockedZone struct {
	record ZoneRecord

	// Statistics
	NumLookupsName   int
	NumLookupsInaddr int
}

func NewMockedZone(zr ZoneRecord) *mockedZone { return &mockedZone{record: zr} }
func (mz mockedZone) Domain() string          { return DefaultLocalDomain }
func (mz mockedZone) LookupName(name string) ([]ZoneRecord, error) {
	Debug.Printf("[mocked zone]: LookupName: returning record %s", mz.record)
	mz.NumLookupsName += 1
	return []ZoneRecord{mz.record}, nil
}
func (mz mockedZone) LookupInaddr(inaddr string) ([]ZoneRecord, error) {
	Debug.Printf("[mocked zone]: LookupInaddr: returning record %s", mz.record)
	mz.NumLookupsInaddr += 1
	return []ZoneRecord{mz.record}, nil
}

// the following methods are not currently needed...
func (mz mockedZone) Status() string                                       { notImplWarn(); return "nothing" }
func (mz mockedZone) AddRecord(ident string, name string, ip net.IP) error { notImplWarn(); return nil }
func (mz mockedZone) DeleteRecord(ident string, ip net.IP) error           { notImplWarn(); return nil }
func (mz mockedZone) DeleteRecordsFor(ident string) error                  { notImplWarn(); return nil }
func (mz mockedZone) DomainLookupName(name string) ([]ZoneRecord, error) {
	notImplWarn()
	return nil, nil
}
func (mz mockedZone) DomainLookupInaddr(inaddr string) ([]ZoneRecord, error) {
	notImplWarn()
	return nil, nil
}
func (mz mockedZone) ObserveName(name string, observer ZoneRecordObserver) error {
	notImplWarn()
	return nil
}
func (mz mockedZone) ObserveInaddr(inaddr string, observer ZoneRecordObserver) error {
	notImplWarn()
	return nil
}
