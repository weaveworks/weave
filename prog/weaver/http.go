package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/nameserver"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
	"net/http"
)

func HandleHTTP(muxRouter *mux.Router,
	router *weave.Router,
	allocator *ipam.Allocator,
	defaultSubnet address.CIDR,
	ns *nameserver.Nameserver,
	dnsserver *nameserver.DNSServer) {

	muxRouter.Methods("GET").Path("/status").Headers("Accept", "application/json").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json, _ := router.StatusJSON(version)
			w.Header().Set("Content-Type", "application/json")
			w.Write(json)
		})

	muxRouter.Methods("GET").Path("/status").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "weave router", version)
			fmt.Fprintln(w, router.Status())
			if allocator != nil {
				fmt.Fprintln(w, allocator.String())
				fmt.Fprintln(w, "Allocator default subnet:", defaultSubnet)
			}
			fmt.Fprintln(w, "")
			if dnsserver == nil {
				fmt.Fprintln(w, "WeaveDNS is disabled")
			} else {
				fmt.Fprintln(w, dnsserver.String())
				fmt.Fprintln(w, ns.String())
			}
		})

}
