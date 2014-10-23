package weavedns

import (
	"errors"
	"fmt"
	"github.com/miekg/dns"
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
	level.Printf(logmsg, logargs...)
}

func ListenHttp(domain string, db Zone, port int) {
	http.HandleFunc("/name/", func(w http.ResponseWriter, r *http.Request) {

		reqError := func(msg string, logmsg string, logargs ...interface{}) {
			httpErrorAndLog(Warning, w, msg, http.StatusBadRequest,
				logmsg, logargs...)
		}

		switch r.Method {
		case "PUT":
			identifier, weaveIPstr, err := parseUrl(r.URL.Path)
			name := r.FormValue("fqdn")
			prefix := r.FormValue("routing_prefix")
			localIP := r.FormValue("local_ip")

			if identifier == "" || weaveIPstr == "" || name == "" || prefix == "" || localIP == "" {
				reqError("Invalid request", "Invalid request: %s, %s", r.URL, r.Form)
				return
			}

			ip := net.ParseIP(localIP)
			if ip == nil {
				reqError("Invalid IP in request", "Invalid IP in request: %s", localIP)
				return
			}

			weaveCIDR := weaveIPstr + "/" + prefix
			weaveIP, subnet, err := net.ParseCIDR(weaveCIDR)
			if err != nil {
				reqError("Invalid CIDR", "Invalid CIDR in request: %s", weaveCIDR)
				return
			}

			if dns.IsSubDomain(domain, name) {
				Info.Printf("Adding %s (%s) -> %s", name, localIP, weaveCIDR)
				err = db.AddRecord(identifier, name, ip, weaveIP, subnet)
				if err != nil {
					dup, ok := err.(DuplicateError)
					if !ok {
						httpErrorAndLog(
							Error, w, "Internal error", http.StatusInternalServerError,
							"Unexpected error from DB: %s", err)
						return
					} else if dup.Ident != identifier {
						http.Error(w, err.Error(), http.StatusConflict)
						return
					} // else we are golden
				}
			} else {
				Info.Printf("Ignoring name %s, not in %s", name, domain)
			}

		case "DELETE":
			identifier, weaveIPstr, err := parseUrl(r.URL.Path)
			if identifier == "" || weaveIPstr == "" {
				reqError("Invalid Request", "Invalid request: %s, %s", r.URL, r.Form)
				return
			}

			weaveIP := net.ParseIP(weaveIPstr)
			if weaveIP == nil {
				reqError("Invalid IP in request", "Invalid IP in request: %s", weaveIPstr)
				return
			}
			Info.Printf("Deleting %s (%s)", identifier, weaveIPstr)
			err = db.DeleteRecord(identifier, weaveIP)
			if err != nil {
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
	err := http.ListenAndServe(address, nil)
	if err != nil {
		Error.Fatal("Unable to create http listener: ", err)
	}
}
