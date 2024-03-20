package ipset

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Name string

type Type string

type UID string

const (
	ListSet = Type("list:set")
	HashIP  = Type("hash:ip")
	HashNet = Type("hash:net")

	DestroyRetrySleepMs = 100
)

type Interface interface {
	Create(ipsetName Name, ipsetType Type) error
	AddEntry(user UID, ipsetName Name, entry string, comment string) error
	DelEntry(user UID, ipsetName Name, entry string) error
	EntryExists(user UID, ipsetName Name, entry string) bool
	Exists(ipsetName Name) (bool, error)
	Flush(ipsetName Name) error
	Destroy(ipsetName Name) error

	List(prefix string) ([]Name, error)

	FlushAll() error
	DestroyAll() error
}

type entryKey struct {
	ipsetName Name
	entry     string
}

type ipset struct {
	*log.Logger
	enableComments bool
	maxListSize    int
	// List of users per ipset entry. User is either a namespace or a pod.
	// There might be multiple users for the same ipset & entry pair because
	// events from k8s API server might be out of order causing duplicate IPs:
	// https://github.com/rajch/weave/issues/2792.
	users map[entryKey]map[UID]struct{}
}

func New(logger *log.Logger, maxListSize int) Interface {
	ips := &ipset{
		Logger:         logger,
		enableComments: true,
		maxListSize:    maxListSize,
		users:          make(map[entryKey]map[UID]struct{}),
	}

	// Check for comment support

	// To prevent from a race when more than one process check for the support
	// we append a random nonce to the test ipset name. The final name is
	// shorter than 31 chars (max ipset name).
	nonce := make([]byte, 4)
	rand.Read(nonce)
	testIpsetName := Name("weave-test-comment" + hex.EncodeToString(nonce))

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
	if ipsetType == ListSet && i.maxListSize > 0 {
		args = append(args, "size", fmt.Sprintf("%d", i.maxListSize))
	}
	if i.enableComments {
		args = append(args, "comment")
	}
	return doExec(args...)
}

func (i *ipset) AddEntry(user UID, ipsetName Name, entry string, comment string) error {
	i.Logger.Printf("adding entry %s to %s of %s", entry, ipsetName, user)

	if !i.addUser(user, ipsetName, entry) { // already in the set
		return nil
	}

	i.Logger.Printf("added entry %s to %s of %s", entry, ipsetName, user)

	args := []string{"add", string(ipsetName), entry}
	if i.enableComments {
		args = append(args, "comment", comment)
	}
	return doExec(args...)
}

func (i *ipset) DelEntry(user UID, ipsetName Name, entry string) error {
	i.Logger.Printf("deleting entry %s from %s of %s", entry, ipsetName, user)

	if !i.delUser(user, ipsetName, entry) { // still needed
		return nil
	}

	i.Logger.Printf("deleted entry %s from %s of %s", entry, ipsetName, user)

	return doExec("del", string(ipsetName), entry)
}

func (i *ipset) EntryExists(user UID, ipsetName Name, entry string) bool {
	return i.existUser(user, ipsetName, entry)
}

// Dummy way to check whether a given ipset exists.
func (i *ipset) Exists(name Name) (bool, error) {
	sets, err := i.List(string(name))
	if err != nil {
		return false, err
	}
	for _, s := range sets {
		if s == name {
			return true, nil
		}
	}
	return false, nil
}

func (i *ipset) Flush(ipsetName Name) error {
	i.removeSetFromUsers(ipsetName)
	return doExec("flush", string(ipsetName))
}

func (i *ipset) FlushAll() error {
	i.users = make(map[entryKey]map[UID]struct{})
	return doExec("flush")
}

func (i *ipset) Destroy(ipsetName Name) error {
	i.removeSetFromUsers(ipsetName)
	err := doExec("destroy", string(ipsetName))
	if err != nil {
		time.Sleep(DestroyRetrySleepMs * time.Millisecond)
		return doExec("destroy", string(ipsetName))
	}
	return err
}

func (i *ipset) DestroyAll() error {
	i.users = make(map[entryKey]map[UID]struct{})
	err := doExec("destroy")
	if err != nil {
		time.Sleep(DestroyRetrySleepMs * time.Millisecond)
		return doExec("destroy")
	}
	return err
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

// Returns true if entry does not exist in ipset (entry has to be inserted into ipset).
func (i *ipset) addUser(user UID, ipsetName Name, entry string) bool {
	k := entryKey{ipsetName, entry}
	add := false

	if i.users[k] == nil {
		i.users[k] = make(map[UID]struct{})
	}
	if len(i.users[k]) == 0 {
		add = true
	}
	i.users[k][user] = struct{}{}

	return add
}

// Returns true if user is the last owner of entry (entry has to be removed from ipset).
func (i *ipset) delUser(user UID, ipsetName Name, entry string) bool {
	k := entryKey{ipsetName, entry}

	oneLeft := len(i.users[k]) == 1
	delete(i.users[k], user)

	if len(i.users[k]) == 0 {
		delete(i.users, k)
	}

	return oneLeft && (len(i.users[k]) == 0)
}

func (i *ipset) existUser(user UID, ipsetName Name, entry string) bool {
	_, ok := i.users[entryKey{ipsetName, entry}][user]
	return ok
}

func (i *ipset) removeSetFromUsers(ipsetName Name) {
	for k := range i.users {
		if k.ipsetName == ipsetName {
			delete(i.users, k)
		}
	}
}

func doExec(args ...string) error {
	if output, err := exec.Command("ipset", args...).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "ipset %v failed: %s", args, output)
	}
	return nil
}
