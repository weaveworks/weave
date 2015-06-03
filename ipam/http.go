package ipam

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
)

func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
	common.Warning.Println("[allocator]:", err.Error())
}

// HandleHTTP wires up ipams HTTP endpoints to the provided mux.
func (alloc *Allocator) HandleHTTP(router *mux.Router) {
	router.Methods("PUT").Path("/ip/{id}/{ip}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closedChan := w.(http.CloseNotifier).CloseNotify()
		vars := mux.Vars(r)
		ident := vars["id"]
		ipStr := vars["ip"]
		if ip, err := address.ParseIP(ipStr); err != nil {
			badRequest(w, err)
			return
		} else if err := alloc.Claim(ident, ip, closedChan); err != nil {
			badRequest(w, fmt.Errorf("Unable to claim: %s", err))
			return
		}

		w.WriteHeader(204)
	})

	router.Methods("GET").Path("/ip/{id}/{ip}/{prefixlen}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		cidr := vars["ip"] + "/" + vars["prefixlen"]
		_, subnet, err := address.ParseCIDR(cidr)
		if err != nil {
			badRequest(w, err)
			return
		}
		addr, err := alloc.Lookup(vars["id"], subnet)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "%s/%d", addr.String(), subnet.PrefixLen)
	})

	router.Methods("GET").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if alloc.defaultSubnet.Blank() {
			badRequest(w, fmt.Errorf("No subnet specified"))
			return
		}
		addr, err := alloc.Lookup(mux.Vars(r)["id"], alloc.defaultSubnet)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "%s/%d", addr.String(), alloc.defaultSubnet.PrefixLen)
	})

	router.Methods("POST").Path("/ip/{id}/{ip}/{prefixlen}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closedChan := w.(http.CloseNotifier).CloseNotify()
		vars := mux.Vars(r)
		ident := vars["id"]
		cidrStr := vars["ip"] + "/" + vars["prefixlen"]
		_, cidr, err := address.ParseCIDR(cidrStr)
		if err != nil {
			badRequest(w, err)
			return
		}
		addr, err := alloc.Allocate(ident, cidr, closedChan)
		if err != nil {
			badRequest(w, fmt.Errorf("Unable to allocate: %s", err))
			return
		}

		fmt.Fprintf(w, "%s/%d", addr.String(), cidr.PrefixLen)
	})

	router.Methods("POST").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if alloc.defaultSubnet.Blank() {
			badRequest(w, fmt.Errorf("No subnet specified"))
			return
		}
		closedChan := w.(http.CloseNotifier).CloseNotify()
		ident := mux.Vars(r)["id"]
		newAddr, err := alloc.Allocate(ident, alloc.defaultSubnet, closedChan)
		if err != nil {
			badRequest(w, err)
			return
		}

		fmt.Fprintf(w, "%s/%d", newAddr.String(), alloc.defaultSubnet.PrefixLen)
	})

	router.Methods("DELETE").Path("/ip/{id}/{ip}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ident := vars["id"]
		ipStr := vars["ip"]
		if ip, err := address.ParseIP(ipStr); err != nil {
			badRequest(w, err)
			return
		} else if err := alloc.Free(ident, ip); err != nil {
			badRequest(w, fmt.Errorf("Unable to free: %s", err))
			return
		}

		w.WriteHeader(204)
	})

	router.Methods("DELETE").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ident := mux.Vars(r)["id"]
		if err := alloc.Delete(ident); err != nil {
			badRequest(w, err)
			return
		}

		w.WriteHeader(204)
	})

	router.Methods("DELETE").Path("/peer").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alloc.Shutdown()
		w.WriteHeader(204)
	})

	router.Methods("DELETE").Path("/peer/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ident := mux.Vars(r)["id"]
		if err := alloc.AdminTakeoverRanges(ident); err != nil {
			badRequest(w, err)
			return
		}

		w.WriteHeader(204)
	})
}
