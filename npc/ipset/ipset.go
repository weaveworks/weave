package ipset

import (
	"bytes"
	"os/exec"
	"strings"

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

	List(prefix string) ([]Name, error)

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
	err, _ := doExec("create", string(ipsetName), string(ipsetType))
	return err
}

func (i *ipset) AddEntry(ipsetName Name, entry string) error {
	if i.inc(ipsetName, entry) > 1 { // already in the set
		return nil
	}
	err, _ := doExec("add", string(ipsetName), entry)
	return err
}

func (i *ipset) DelEntry(ipsetName Name, entry string) error {
	if i.dec(ipsetName, entry) > 0 { // still needed
		return nil
	}
	err, _ := doExec("del", string(ipsetName), entry)
	return err
}

func (i *ipset) Flush(ipsetName Name) error {
	i.removeSet(ipsetName)
	err, _ := doExec("flush", string(ipsetName))
	return err
}

func (i *ipset) FlushAll() error {
	i.refCount = newRefCount()
	err, _ := doExec("flush")
	return err
}

func (i *ipset) Destroy(ipsetName Name) error {
	i.removeSet(ipsetName)
	err, _ := doExec("destroy", string(ipsetName))
	return err
}

func (i *ipset) DestroyAll() error {
	i.refCount = newRefCount()
	err, _ := doExec("destroy")
	return err
}

// Fetch a list of all existing sets
func (i *ipset) List(prefix string) ([]Name, error) {
	err, output := doExec("list","-name","-output","plain")

	var selected []Name
	if err == nil && len(output) > 0 {
		output = bytes.TrimRight(output, "\n")
		sets := strings.Split(string(output[:]), "\n")

		plen := len(prefix)
		for _, v := range sets {
			if (plen <= len(v)) && (prefix == v[:len(prefix)]) {
				selected = append(selected, Name(v))
			}
		}

	}

	return selected, err
}

func doExec(args ...string) (error, []byte) {
	output, err := exec.Command("ipset", args...).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "ipset %v failed: %s", args, output), output
	}
	return nil, output
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

func (rc *refCount) removeSet(ipsetName Name) {
	for k := range rc.ref {
		if k.ipsetName == ipsetName {
			delete(rc.ref, k)
		}
	}
}
