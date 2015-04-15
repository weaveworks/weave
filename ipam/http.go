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
	common.Warning.Println(err.Error())
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
			badRequest(w, fmt.Errorf("Unable to claim IP address %s: %s", ip, err))
			return
		}

		w.WriteHeader(204)
	})

	router.Methods("POST").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closedChan := w.(http.CloseNotifier).CloseNotify()
		ident := mux.Vars(r)["id"]
		newAddr, err := alloc.Allocate(ident, closedChan)
		if err != nil {
			badRequest(w, err)
			return
		}

		fmt.Fprintf(w, "%s/%d", newAddr.String(), alloc.prefixLen)
	})

	router.Methods("DELETE").Path("/ip/{id}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ident := mux.Vars(r)["id"]
		if err := alloc.Free(ident); err != nil {
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
