package ipset

import (
	"os/exec"

	"github.com/pkg/errors"
)

type Name string

type Type string

const (
	ListSet = Type("list:set")
	HashIP  = Type("hash:ip")
)

type Interface interface {
	Create(ipsetName Name, ipsetType Type) error
	AddEntry(ipsetName Name, entry string) error
	DelEntry(ipsetName Name, entry string) error
	Flush(ipsetName Name) error
	Destroy(ipsetName Name) error

	FlushAll() error
	DestroyAll() error
}

type ipset struct {
	refCount
}

func New() Interface {
	return &ipset{refCount: newRefCount()}
}

func (i *ipset) Create(ipsetName Name, ipsetType Type) error {
	return doExec("create", string(ipsetName), string(ipsetType))
}

func (i *ipset) AddEntry(ipsetName Name, entry string) error {
	if i.inc(ipsetName, entry) > 1 { // already in the set
		return nil
	}
	return doExec("add", string(ipsetName), entry)
}

func (i *ipset) DelEntry(ipsetName Name, entry string) error {
	if i.dec(ipsetName, entry) > 0 { // still needed
		return nil
	}
	return doExec("del", string(ipsetName), entry)
}

func (i *ipset) Flush(ipsetName Name) error {
	return doExec("flush", string(ipsetName))
}

func (i *ipset) FlushAll() error {
	return doExec("flush")
}

func (i *ipset) Destroy(ipsetName Name) error {
	return doExec("destroy", string(ipsetName))
}

func (i *ipset) DestroyAll() error {
	return doExec("destroy")
}

func doExec(args ...string) error {
	if output, err := exec.Command("ipset", args...).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "ipset %v failed: %s", args, output)
	}
	return nil
}

// Reference-counting
type key struct {
	ipsetName Name
	entry     string
}

// note no locking is required as all operations are serialised in the controller
type refCount struct {
	ref map[key]int
}

func newRefCount() refCount {
	return refCount{ref: make(map[key]int)}
}

func (rc *refCount) inc(ipsetName Name, entry string) int {
	k := key{ipsetName, entry}
	rc.ref[k]++
	return rc.ref[k]
}

func (rc *refCount) dec(ipsetName Name, entry string) int {
	k := key{ipsetName, entry}
	rc.ref[k]--
	return rc.ref[k]
}
