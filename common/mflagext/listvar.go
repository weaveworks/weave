package mflagext

import (
	"fmt"

	"github.com/weaveworks/docker/pkg/mflag"
)

type listOpts struct {
	value      *[]string
	hasBeenSet bool
}

func ListVar(p *[]string, names []string, value []string, usage string) {
	*p = value
	mflag.Var(&listOpts{p, false}, names, usage)
}

func (opts *listOpts) Set(value string) error {
	if opts.hasBeenSet {
		(*opts.value) = append((*opts.value), value)
	} else {
		(*opts.value) = []string{value}
		opts.hasBeenSet = true
	}
	return nil
}

func (opts *listOpts) String() string {
	return fmt.Sprintf("%v", []string(*opts.value))
}
