package ipam

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/net/address"
)

func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
	common.Log.Warningln("[allocator]:", err.Error())
}

func extractCIDR(w http.ResponseWriter, vars map[string]string) (address.CIDR, bool) {
	cidrStr := vars["ip"] + "/" + vars["prefixlen"]
	subnetAddr, cidr, err := address.ParseCIDR(cidrStr)
	if err != nil {
		badRequest(w, err)
		return address.CIDR{}, false
	}
	if cidr.Start != subnetAddr {
		badRequest(w, fmt.Errorf("Invalid subnet %s - bits after network prefix are not all zero", cidrStr))
		return address.CIDR{}, false
	}
	return cidr, true
}

func doAllocate(alloc *Allocator, dockerCli *docker.Client, w http.ResponseWriter, r *http.Request, vars map[string]string, subnet address.CIDR) {
	closedChan := w.(http.CloseNotifier).CloseNotify()
	ident := vars["id"]
	addr, err := alloc.Allocate(ident, subnet.HostRange(), closedChan)
	if err != nil {
		if _, ok := err.(*errorCancelled); ok { // cancellation is not really an error
			common.Log.Infoln("[allocator]:", err.Error())
			fmt.Fprint(w, "cancelled")
			return
		}
		badRequest(w, err)
		return
	}
	if r.FormValue("check-alive") == "true" && dockerCli != nil && dockerCli.IsContainerNotRunning(ident) {
		common.Log.Infof("[allocator] '%s' is not running: freeing %s", ident, addr)
		alloc.Free(ident, addr)
		fmt.Fprint(w, "cancelled")
		return
	}

	fmt.Fprintf(w, "%s/%d", addr, subnet.PrefixLen)
}

// HandleHTTP wires up ipams HTTP endpoints to the provided mux.
func (alloc *Allocator) HandleHTTP(router *mux.Router, defaultSubnet address.CIDR, dockerCli *docker.Client) {
	router.Methods("PUT").Path("/ip/{id}/{ip}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ident := vars["id"]
		ipStr := vars["ip"]
		noErrorOnUnknown := r.FormValue("noErrorOnUnknown") == "true"
		if ip, err := address.ParseIP(ipStr); err != nil {
			badRequest(w, err)
			return
		} else if err := alloc.Claim(ident, ip, noErrorOnUnknown); err != nil {
			badRequest(w, fmt.Errorf("Unable to claim: %s", err))
			return
		}

		w.WriteHeader(204)
	})

	router.Methods("GET").Path("/ip/{id}/{ip}/{prefixlen}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if subnet, ok := extractCIDR(w, vars); ok {
			addr, err := alloc.Lookup(vars["id"], subnet.HostRange())
			if err != nil {
				http.NotFound(w, r)
				return
			}
			fmt.Fprintf(w, "%s/%d", addr, subnet.PrefixLen)
		}
	})

	router.Methods("GET").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr, err := alloc.Lookup(mux.Vars(r)["id"], defaultSubnet.HostRange())
		if err != nil {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "%s/%d", addr, defaultSubnet.PrefixLen)
	})

	router.Methods("POST").Path("/ip/{id}/{ip}/{prefixlen}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if subnet, ok := extractCIDR(w, vars); ok {
			doAllocate(alloc, dockerCli, w, r, vars, subnet)
		}
	})

	router.Methods("POST").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doAllocate(alloc, dockerCli, w, r, mux.Vars(r), defaultSubnet)
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
