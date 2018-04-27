package mflagext

import (
	"fmt"

	"github.com/weaveworks/common/mflag"
)

type listOpts struct {
	value      *[]string
	hasBeenSet bool
}

// ListVar creates an mflag.Var for repeated flags.
func ListVar(p *[]string, names []string, value []string, usage string) {
	*p = value
	mflag.Var(&listOpts{p, false}, names, usage)
}

// Set implements mflag.Var
func (opts *listOpts) Set(value string) error {
	if opts.hasBeenSet {
		(*opts.value) = append((*opts.value), value)
	} else {
		(*opts.value) = []string{value}
		opts.hasBeenSet = true
	}
	return nil
}

// String implements mflag.Var
func (opts *listOpts) String() string {
	return fmt.Sprintf("%v", []string(*opts.value))
}
