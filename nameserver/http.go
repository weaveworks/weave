package nameserver

import (
	"errors"
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// Parse a URL of the form PUT /name/<identifier>/<ip-address>
func parseUrl(url string) (identifier string, ipaddr string, err error) {
	parts := strings.Split(url, "/")
	if len(parts) != 4 {
		return "", "", errors.New("Unable to parse url: " + url)
	}
	return parts[2], parts[3], nil
}

func httpErrorAndLog(level *log.Logger, w http.ResponseWriter, msg string,
	status int, logmsg string, logargs ...interface{}) {
	http.Error(w, msg, status)
	level.Printf("[http] "+logmsg, logargs...)
}

func ListenHttp(domain string, db Zone, port int) {

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		} else {
			io.WriteString(w, "ok")
		}
	})

	http.HandleFunc("/name/", func(w http.ResponseWriter, r *http.Request) {

		reqError := func(msg string, logmsg string, logargs ...interface{}) {
			httpErrorAndLog(Warning, w, msg, http.StatusBadRequest,
				logmsg, logargs...)
		}

		switch r.Method {
		case "PUT":
			ident, ipStr, err := parseUrl(r.URL.Path)
			name := r.FormValue("fqdn")
			if ident == "" || ipStr == "" || name == "" {
				reqError("Invalid request", "Invalid request: %s, %s", r.URL, r.Form)
				return
			}

			ip := net.ParseIP(ipStr)
			if ip == nil {
				reqError("Invalid IP", "Invalid IP in request: %s", ipStr)
				return
			}

			if dns.IsSubDomain(domain, name) {
				Info.Printf("[http] Adding %s -> %s", name, ipStr)
				if err = db.AddRecord(ident, name, ip); err != nil {
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

		case "DELETE":
			ident, ipStr, err := parseUrl(r.URL.Path)
			if ident == "" || ipStr == "" {
				reqError("Invalid Request", "Invalid request: %s, %s", r.URL, r.Form)
				return
			}

			ip := net.ParseIP(ipStr)
			if ip == nil {
				reqError("Invalid IP in request", "Invalid IP in request: %s", ipStr)
				return
			}
			Info.Printf("[http] Deleting %s (%s)", ident, ipStr)
			if err = db.DeleteRecord(ident, ip); err != nil {
				if _, ok := err.(LookupError); !ok {
					httpErrorAndLog(
						Error, w, "Internal error", http.StatusInternalServerError,
						"Unexpected error from DB: %s", err)
					return
				}
			}
		default:
			msg := "Unexpected http method: " + r.Method
			reqError(msg, msg)
			return
		}
	})

	address := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(address, nil); err != nil {
		Error.Fatal("[http] Unable to create http listener: ", err)
	}
}
