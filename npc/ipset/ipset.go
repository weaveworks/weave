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
}

func New() Interface {
	return &ipset{}
}

func (i *ipset) Create(ipsetName Name, ipsetType Type) error {
	return doExec("create", string(ipsetName), string(ipsetType))
}

func (i *ipset) AddEntry(ipsetName Name, entry string) error {
	return doExec("add", string(ipsetName), entry)
}

func (i *ipset) DelEntry(ipsetName Name, entry string) error {
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
