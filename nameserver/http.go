package nameserver

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/miekg/dns"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/address"
)

func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
	common.Log.Warningf("[gossipdns]: %v", err)
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
			hostname  = r.FormValue("fqdn")
			ip, err   = address.ParseIP(ipStr)
		)
		if err != nil {
			badRequest(w, err)
			return
		}

		if err := n.AddEntry(dns.Fqdn(hostname), container, n.ourName, ip); err != nil {
			badRequest(w, fmt.Errorf("Unable to add entry: %v", err))
			return
		}

		if r.FormValue("check-alive") == "true" && n.docker.IsContainerNotRunning(container) {
			common.Log.Infof("[gossipdns] '%s' is not running: removing", container)
			if err := n.Delete(hostname, container, ipStr, ip); err != nil {
				common.Log.Infof("[gossipdns] failed to remove: %v", err)
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
			badRequest(w, err)
			return
		} else if !ok {
			ipStr = "*"
		}

		if err := n.Delete(hostname, container, ipStr, ip); err != nil {
			badRequest(w, fmt.Errorf("Unable to delete entries: %v", err))
			return
		}
		w.WriteHeader(204)
	}
	router.Methods("DELETE").Path("/name/{container}/{ip}").HandlerFunc(deleteHandler)
	router.Methods("DELETE").Path("/name/{container}").HandlerFunc(deleteHandler)
	router.Methods("DELETE").Path("/name").HandlerFunc(deleteHandler)
}
