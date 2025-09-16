// Adapted from k8s.io/client-go/kubernetes/fake to use a modified ObjectTracker
package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/testing"

	weavetracker "github.com/rajch/weave/testing/kubernetes/testing"
)

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func NewSimpleClientset(objects ...runtime.Object) *fake.Clientset {
	o := weavetracker.NewObjectTracker(scheme, codecs.UniversalDecoder())
	for _, obj := range objects {
		if err := o.Add(obj); err != nil {
			panic(err)
		}
	}

	ret := fake.Clientset{}
	ret.AddReactor("*", "*", weavetracker.ObjectReaction(o))
	ret.AddWatchReactor("*", testing.DefaultWatchReactor(watch.NewFake(), nil))

	return &ret
}
