package nameserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/miekg/dns"

	"github.com/weaveworks/weave/net/address"
)

func (n *Nameserver) badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
	n.infof("%v", err)
}

func (n *Nameserver) HandleHTTP(router *mux.Router) {
	router.Methods("GET").Path("/domain").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, n.domain)
	})

	router.Methods("PUT").Path("/name/{container}/{ip}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			vars      = mux.Vars(r)
			container = vars["container"]
			ipStr     = vars["ip"]
			hostname  = dns.Fqdn(r.FormValue("fqdn"))
			ip, err   = address.ParseIP(ipStr)
		)
		if err != nil {
			n.badRequest(w, err)
			return
		}

		if err := n.AddEntry(hostname, container, n.ourName, ip); err != nil {
			n.badRequest(w, fmt.Errorf("Unable to add entry: %v", err))
			return
		}

		if r.FormValue("check-alive") == "true" && n.docker.IsContainerNotRunning(container) {
			n.infof("container '%s' is not running: removing", container)
			if err := n.Delete(hostname, container, ipStr, ip); err != nil {
				n.infof("failed to remove: %v", err)
			}
		}

		w.WriteHeader(204)
	})

	deleteHandler := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		hostname := r.FormValue("fqdn")
		if hostname == "" {
			hostname = "*"
		} else {
			hostname = dns.Fqdn(hostname)
		}

		container, ok := vars["container"]
		if !ok {
			container = "*"
		}

		ipStr, ok := vars["ip"]
		ip, err := address.ParseIP(ipStr)
		if ok && err != nil {
			n.badRequest(w, err)
			return
		} else if !ok {
			ipStr = "*"
		}

		if err := n.Delete(hostname, container, ipStr, ip); err != nil {
			n.badRequest(w, fmt.Errorf("Unable to delete entries: %v", err))
			return
		}
		w.WriteHeader(204)
	}
	router.Methods("DELETE").Path("/name/{container}/{ip}").HandlerFunc(deleteHandler)
	router.Methods("DELETE").Path("/name/{container}").HandlerFunc(deleteHandler)
	router.Methods("DELETE").Path("/name").HandlerFunc(deleteHandler)

	router.Methods("GET").Path("/name").Headers("Accept", "application/json").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.RLock()
		defer n.RUnlock()
		if err := json.NewEncoder(w).Encode(n.entries); err != nil {
			n.badRequest(w, fmt.Errorf("Error marshalling response: %v", err))
		}
	})
}
