package nameserver

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"io"
	"log"
	"net"
	"net/http"
)

func httpErrorAndLog(level *log.Logger, w http.ResponseWriter, msg string,
	status int, logmsg string, logargs ...interface{}) {
	http.Error(w, msg, status)
	level.Printf("[http] "+logmsg, logargs...)
}

func ListenHttp(version string, server *DNSServer, domain string, db Zone, port int) {

	muxRouter := mux.NewRouter()

	muxRouter.Methods("GET").Path("/status").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln("weave DNS", version))
		io.WriteString(w, server.Status())
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

		if dns.IsSubDomain(domain, name) {
			Info.Printf("[http] Adding %s -> %s", name, ipStr)
			if err := db.AddRecord(idStr, name, ip); err != nil {
				if _, ok := err.(DuplicateError); !ok {
					httpErrorAndLog(
						Error, w, "Internal error", http.StatusInternalServerError,
						"Unexpected error from DB: %s", err)
					return
				} // oh, I already know this. whatever.
			}
		} else {
			Info.Printf("[http] Ignoring name %s, not in %s", name, domain)
		}
	})

	muxRouter.Methods("DELETE").Path("/name/{id:.+}/{ip:.+}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqError := func(msg string, logmsg string, logargs ...interface{}) {
			httpErrorAndLog(Warning, w, msg, http.StatusBadRequest, logmsg, logargs...)
		}

		vars := mux.Vars(r)
		idStr := vars["id"]
		ipStr := vars["ip"]

		ip := net.ParseIP(ipStr)
		if ip == nil {
			reqError("Invalid IP in request", "Invalid IP in request: %s", ipStr)
			return
		}
		Info.Printf("[http] Deleting %s (%s)", idStr, ipStr)
		if err := db.DeleteRecord(idStr, ip); err != nil {
			if _, ok := err.(LookupError); !ok {
				httpErrorAndLog(
					Error, w, "Internal error", http.StatusInternalServerError,
					"Unexpected error from DB: %s", err)
				return
			}
		}
	})

	http.Handle("/", muxRouter)

	address := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(address, nil); err != nil {
		Error.Fatal("[http] Unable to create http listener: ", err)
	}
}
