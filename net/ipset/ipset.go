package ipset

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/weaveworks/weave/common"
	"k8s.io/apimachinery/pkg/types"
)

type Name string

type Type string

const (
	ListSet = Type("list:set")
	HashIP  = Type("hash:ip")
	HashNet = Type("hash:net")
)

type Interface interface {
	Create(ipsetName Name, ipsetType Type) error
	AddEntry(user types.UID, ipsetName Name, entry string, comment string) error
	DelEntry(user types.UID, ipsetName Name, entry string) error
	Exist(user types.UID, ipsetName Name, entry string) bool
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
	// https://github.com/weaveworks/weave/issues/2792.
	users map[entryKey]map[types.UID]struct{}
}

var resyncOnDelete bool

func init() {

	// check if Kernel version is in between 4.2 and 4.10. There is a kerenl bug
	// due to which ipset delete sometimes ends up in deleting unintended entries
	// https://bugzilla.netfilter.org/show_bug.cgi?id=1119
	// if Kernel version has this issue then we need to workaround by resyncing
	// ipset to the expected list of entries
	kernelVersion, err := exec.Command("uname", "-r").Output()
	if err != nil {
		common.Log.Fatalf("Failed to get Kernel version")
	}
	splitVersion := strings.SplitN(string(kernelVersion[:]), ".", 3)
	if splitVersion[0] == "4" {
		majorVersion, err := strconv.Atoi(splitVersion[1])
		if err != nil {
			common.Log.Fatalf("Failed to process Kernel major version")
		}
		if majorVersion >= 2 && majorVersion <= 10 {
			resyncOnDelete = true
		}
	}
}

func New(logger *log.Logger, maxListSize int) Interface {
	ips := &ipset{
		Logger:         logger,
		enableComments: true,
		maxListSize:    maxListSize,
		users:          make(map[entryKey]map[types.UID]struct{}),
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

func (i *ipset) AddEntry(user types.UID, ipsetName Name, entry string, comment string) error {
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

func (i *ipset) DelEntry(user types.UID, ipsetName Name, entry string) error {
	i.Logger.Printf("deleting entry %s from %s of %s", entry, ipsetName, user)

	if !i.delUser(user, ipsetName, entry) { // still needed
		return nil
	}

	i.Logger.Printf("deleted entry %s from %s of %s", entry, ipsetName, user)

	if resyncOnDelete {
		return i.safeDelEntry(user, ipsetName, entry)
	}

	return doExec("del", string(ipsetName), entry)
}

// logic to workaround Kernel bug https://bugzilla.netfilter.org/show_bug.cgi?id=1119
func (i *ipset) safeDelEntry(user types.UID, ipsetName Name, entry string) error {
	oldEntries, err := listEntries(ipsetName)
	if err != nil {
		return err
	}
	err = doExec("del", string(ipsetName), entry)
	if err != nil && !strings.Contains(err.Error(), "Element cannot be deleted from the set: it's not added") {
		return err
	}
	newEntries, err := listEntries(ipsetName)
	if err != nil {
		return err
	}
	expectedEntries := make([]string, 0)
	for _, oe := range oldEntries {
		if !strings.Contains(oe, entry) {
			expectedEntries = append(expectedEntries, oe)
		}
	}
	for _, ee := range expectedEntries {
		exists := false
		for _, ne := range newEntries {
			if strings.Compare(ee, ne) == 0 {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		args := make([]string, 0)
		if i.enableComments {
			splitEntry := strings.Split(ee, " comment ")
			args = append(args, "add", string(ipsetName), splitEntry[0])
			args = append(args, "comment", splitEntry[1])
		} else {
			args = append(args, "add", string(ipsetName), ee)
		}
		err = doExec(args...)
		return err
	}
	return nil
}

func (i *ipset) Exist(user types.UID, ipsetName Name, entry string) bool {
	return i.existUser(user, ipsetName, entry)
}

func (i *ipset) Flush(ipsetName Name) error {
	i.removeSetFromUsers(ipsetName)
	return doExec("flush", string(ipsetName))
}

func (i *ipset) FlushAll() error {
	i.users = make(map[entryKey]map[types.UID]struct{})
	return doExec("flush")
}

func (i *ipset) Destroy(ipsetName Name) error {
	i.removeSetFromUsers(ipsetName)
	return doExec("destroy", string(ipsetName))
}

func (i *ipset) DestroyAll() error {
	i.users = make(map[entryKey]map[types.UID]struct{})
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

// Returns true if entry does not exist in ipset (entry has to be inserted into ipset).
func (i *ipset) addUser(user types.UID, ipsetName Name, entry string) bool {
	k := entryKey{ipsetName, entry}
	add := false

	if i.users[k] == nil {
		i.users[k] = make(map[types.UID]struct{})
	}
	if len(i.users[k]) == 0 {
		add = true
	}
	i.users[k][user] = struct{}{}

	return add
}

// Returns true if user is the last owner of entry (entry has to be removed from ipset).
func (i *ipset) delUser(user types.UID, ipsetName Name, entry string) bool {
	k := entryKey{ipsetName, entry}

	oneLeft := len(i.users[k]) == 1
	delete(i.users[k], user)

	if len(i.users[k]) == 0 {
		delete(i.users, k)
	}

	return oneLeft && (len(i.users[k]) == 0)
}

func (i *ipset) existUser(user types.UID, ipsetName Name, entry string) bool {
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

func listEntries(ipsetName Name) ([]string, error) {
	output, err := exec.Command("ipset", "list", string(ipsetName)).CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "list ipset %s failed: %s", ipsetName, output)
	}
	r := regexp.MustCompile("(?m)^(.*\n)*Members:\n")
	list := r.ReplaceAllString(string(output[:]), "")
	return strings.Split(list, "\n"), nil
}

func doExec(args ...string) error {
	if output, err := exec.Command("ipset", args...).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "ipset %v failed: %s", args, output)
	}
	return nil
}
