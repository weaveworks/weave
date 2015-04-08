package ipam

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
)

// Parse a URL of the form /xxx/<identifier>
func parseURL(url string) (identifier string, err error) {
	parts := strings.Split(url, "/")
	if len(parts) != 3 {
		return "", errors.New("Unable to parse url: " + url)
	}
	return parts[2], nil
}

// Parse a URL of the form /xxx/<identifier>/<ip-address>
func parseURLWithIP(url string) (identifier string, ipaddr string, err error) {
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

// HandleHTTP wires up ipams HTTP endpoints to the provided mux.
func (alloc *Allocator) HandleHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/ip/", func(w http.ResponseWriter, r *http.Request) {
		var closedChan = w.(http.CloseNotifier).CloseNotify()

		switch r.Method {
		case "PUT": // caller supplies an address to reserve for a container
			ident, ipStr, err := parseURLWithIP(r.URL.Path)
			if err != nil {
				httpErrorAndLog(common.Warning, w, "Invalid request", http.StatusBadRequest, err.Error())
			} else if ip := net.ParseIP(ipStr); ip == nil {
				httpErrorAndLog(common.Warning, w, "Invalid IP", http.StatusBadRequest,
					"Invalid IP in request: %s", ipStr)
			} else if err = alloc.Claim(ident, ip, closedChan); err != nil {
				httpErrorAndLog(common.Warning, w, "Unsuccessful claim: "+err.Error(), http.StatusBadRequest, "Unable to claim IP address %s: %s", ip, err)
			}
		case "GET": // caller requests one address for a container
			ident, err := parseURL(r.URL.Path)
			if err != nil {
				httpErrorAndLog(common.Warning, w, "Invalid request", http.StatusBadRequest, err.Error())
			} else if newAddr := alloc.GetFor(ident, closedChan); newAddr != nil {
				io.WriteString(w, fmt.Sprintf("%s/%d", newAddr, alloc.universeLen))
			} else {
				httpErrorAndLog(
					common.Error, w, "No free addresses", http.StatusServiceUnavailable,
					"No free addresses")
			}
		case "DELETE": // opposite of PUT for one specific address or all addresses
			ident, ipStr, err := parseURLWithIP(r.URL.Path)
			if err != nil {
				httpErrorAndLog(common.Warning, w, "Invalid request", http.StatusBadRequest, err.Error())
			} else if ipStr == "*" {
				alloc.ContainerDied(ident)
			} else if ip := net.ParseIP(ipStr); ip == nil {
				httpErrorAndLog(common.Warning, w, "Invalid IP", http.StatusBadRequest,
					"Invalid IP in request: %s", ipStr)
				return
			} else if err = alloc.Free(ident, ip); err != nil {
				httpErrorAndLog(common.Warning, w, "Invalid Free", http.StatusBadRequest, err.Error())
			}
		default:
			http.Error(w, "Verb not handled", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/tombstone-self", func(w http.ResponseWriter, r *http.Request) {
		alloc.Shutdown()
	})
	mux.HandleFunc("/peer", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			peers := alloc.ListPeers()
			json.NewEncoder(w).Encode(peers)
		default:
			http.Error(w, "Verb not handled", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/peer/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "DELETE": // opposite of PUT for one specific address or all addresses
			ident, err := parseURL(r.URL.Path)
			if err != nil {
				httpErrorAndLog(common.Warning, w, "Invalid request", http.StatusBadRequest, err.Error())
				return
			}

			peername, err := router.PeerNameFromString(ident)
			if err != nil {
				httpErrorAndLog(common.Warning, w, "Invalid peername", http.StatusBadRequest, err.Error())
				return
			}

			if err := alloc.TombstonePeer(peername); err != nil {
				httpErrorAndLog(common.Warning, w, "Cannot remove peer", http.StatusBadRequest,
					err.Error())
				return
			}

			w.WriteHeader(204)
		default:
			http.Error(w, "Verb not handled", http.StatusBadRequest)
		}
	})
}
