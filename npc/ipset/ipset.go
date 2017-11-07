package ipset

import (
	"log"
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
	AddEntry(ipsetName Name, entry string, comment string) error
	AddEntryIfNotExist(ipsetName Name, entry string, comment string) error
	DelEntry(ipsetName Name, entry string) error
	DelEntryIfExists(ipsetName Name, entry string) error
	Exist(ipsetName Name, entry string) bool
	Flush(ipsetName Name) error
	Destroy(ipsetName Name) error

	List(prefix string) ([]Name, error)

	FlushAll() error
	DestroyAll() error
}

type ipset struct {
	refCount
	*log.Logger
	enableComments bool
}

func New(logger *log.Logger) Interface {
	ips := &ipset{refCount: newRefCount(), Logger: logger, enableComments: true}

	// Check for comment support
	testIpsetName := Name("weave-test-comment")
	// Clear it out if it already exists
	_ = ips.Destroy(testIpsetName)
	// Test for comment support
	if err := ips.Create(testIpsetName, HashIP); err != nil {
		ips.Logger.Printf("failed to create %s; disabling comment support", testIpsetName)
		ips.enableComments = false
	}
	// If it was created, destroy it
	_ = ips.Destroy(testIpsetName)

	return ips
}

func (i *ipset) Create(ipsetName Name, ipsetType Type) error {
	args := []string{"create", string(ipsetName), string(ipsetType)}
	if i.enableComments {
		args = append(args, "comment")
	}
	return doExec(args...)
}

func (i *ipset) AddEntry(ipsetName Name, entry string, comment string) error {
	i.Logger.Printf("adding entry %s to %s", entry, ipsetName)
	if i.inc(ipsetName, entry) > 1 { // already in the set
		return nil
	}
	args := []string{"add", string(ipsetName), entry}
	if i.enableComments {
		args = append(args, "comment", comment)
	}
	return doExec(args...)
}

// AddEntryIfNotExist does the same as AddEntry but bypasses the ref counting.
// Should be used only with "default-allow" ipsets.
func (i *ipset) AddEntryIfNotExist(ipsetName Name, entry string, comment string) error {
	if i.count(ipsetName, entry) == 1 {
		return nil
	}
	return i.AddEntry(ipsetName, entry, comment)
}

func (i *ipset) DelEntry(ipsetName Name, entry string) error {
	i.Logger.Printf("deleting entry %s from %s", entry, ipsetName)
	if i.dec(ipsetName, entry) > 0 { // still needed
		return nil
	}
	return doExec("del", string(ipsetName), entry)
}

// DelEntryIfExists does the same as DelEntry but bypasses the ref counting.
// Should be used only with "default-allow" ipsets.
func (i *ipset) DelEntryIfExists(ipsetName Name, entry string) error {
	if i.count(ipsetName, entry) == 0 {
		return nil
	}
	return i.DelEntry(ipsetName, entry)
}

func (i *ipset) Exist(ipsetName Name, entry string) bool {
	return i.count(ipsetName, entry) > 0
}

func (i *ipset) Flush(ipsetName Name) error {
	i.removeSet(ipsetName)
	return doExec("flush", string(ipsetName))
}

func (i *ipset) FlushAll() error {
	i.refCount = newRefCount()
	return doExec("flush")
}

func (i *ipset) Destroy(ipsetName Name) error {
	i.removeSet(ipsetName)
	return doExec("destroy", string(ipsetName))
}

func (i *ipset) DestroyAll() error {
	i.refCount = newRefCount()
	return doExec("destroy")
}

// Fetch a list of all existing sets with a given prefix
func (i *ipset) List(prefix string) ([]Name, error) {
	output, err := exec.Command("ipset", "list", "-name", "-output", "plain").Output()
	if err != nil {
		return nil, err
	}

	var selected []Name
	sets := strings.Split(string(output), "\n")
	for _, v := range sets {
		if strings.HasPrefix(v, prefix) {
			selected = append(selected, Name(v))
		}
	}

	return selected, err
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

func (rc *refCount) count(ipsetName Name, entry string) int {
	return rc.ref[key{ipsetName, entry}]
}

func (rc *refCount) removeSet(ipsetName Name) {
	for k := range rc.ref {
		if k.ipsetName == ipsetName {
			delete(rc.ref, k)
		}
	}
}
