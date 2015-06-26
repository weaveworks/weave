package nameserver

import (
	"fmt"
	"net"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
)

func httpErrorAndLog(level *log.Logger, w http.ResponseWriter, msg string,
	status int, logmsg string, logargs ...interface{}) {
	http.Error(w, msg, status)
	level.Printf("[http] "+logmsg, logargs...)
}

func extractFQDN(r *http.Request) (string, string) {
	if fqdnStr := r.FormValue("fqdn"); fqdnStr != "" {
		return fqdnStr, fqdnStr
	}
	return "*", ""
}

func ServeHTTP(listener net.Listener, version string, server *DNSServer, dockerCli *docker.Client) {

	muxRouter := mux.NewRouter()

	muxRouter.Methods("GET").Path("/status").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "weave DNS", version)
		fmt.Fprintln(w, server.Status())
		fmt.Fprintln(w, server.Zone.Status())
	})

	muxRouter.Methods("GET").Path("/domain").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, server.Zone.Domain())
	})

	muxRouter.Methods("PUT").Path("/name/{id:.+}/{ip:.+}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqError := func(msg string, logmsg string, logargs ...interface{}) {
			httpErrorAndLog(Warning, w, msg, http.StatusBadRequest, logmsg, logargs...)
		}

		vars := mux.Vars(r)
		idStr := vars["id"]
		ipStr := vars["ip"]
		name := r.FormValue("fqdn")

		if name == "" {
			reqError("Invalid FQDN", "Invalid FQDN in request: %s, %s", r.URL, r.Form)
			return
		}

		ip := net.ParseIP(ipStr)
		if ip == nil {
			reqError("Invalid IP", "Invalid IP in request: %s", ipStr)
			return
		}

		domain := server.Zone.Domain()
		if !dns.IsSubDomain(domain, name) {
			Info.Printf("[http] Ignoring name %s, not in %s", name, domain)
			return
		}
		Info.Printf("[http] Adding %s -> %s", name, ipStr)
		if err := server.Zone.AddRecord(idStr, name, ip); err != nil {
			if _, ok := err.(DuplicateError); !ok {
				httpErrorAndLog(
					Error, w, "Internal error", http.StatusInternalServerError,
					"Unexpected error from DB: %s", err)
				return
			} // oh, I already know this. whatever.
		}

		if dockerCli != nil && dockerCli.IsContainerNotRunning(idStr) {
			Info.Printf("[http] '%s' is not running: removing", idStr)
			server.Zone.DeleteRecords(idStr, name, ip)
		}
	})

	muxRouter.Methods("DELETE").Path("/name/{id:.+}/{ip:.+}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		idStr := vars["id"]
		ipStr := vars["ip"]

		ip := net.ParseIP(ipStr)
		if ip == nil {
			httpErrorAndLog(
				Warning, w, "Invalid IP in request", http.StatusBadRequest,
				"Invalid IP in request: %s", ipStr)
			return
		}

		fqdnStr, fqdn := extractFQDN(r)

		Info.Printf("[http] Deleting ID %s, IP %s, FQDN %s", idStr, ipStr, fqdnStr)
		server.Zone.DeleteRecords(idStr, fqdn, ip)
	})

	muxRouter.Methods("DELETE").Path("/name/{id:.+}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		idStr := vars["id"]

		fqdnStr, fqdn := extractFQDN(r)

		Info.Printf("[http] Deleting ID %s, IP *, FQDN %s", idStr, fqdnStr)
		server.Zone.DeleteRecords(idStr, fqdn, net.IP{})
	})

	muxRouter.Methods("DELETE").Path("/name").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fqdnStr, fqdn := extractFQDN(r)

		Info.Printf("[http] Deleting ID *, IP *, FQDN %s", fqdnStr)
		server.Zone.DeleteRecords("", fqdn, net.IP{})
	})

	http.Handle("/", muxRouter)

	if err := http.Serve(listener, nil); err != nil {
		Error.Fatal("[http] Unable to serve http: ", err)
	}
}
