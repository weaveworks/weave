package ipam

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/weaveworks/weave/common"
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

func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
	common.Warning.Println(err.Error())
}

func invalidIP(w http.ResponseWriter, ip string) {
	badRequest(w, fmt.Errorf("Invalid IP in request: %s", ip))
}

// HandleHTTP wires up ipams HTTP endpoints to the provided mux.
func (alloc *Allocator) HandleHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/ip/", func(w http.ResponseWriter, r *http.Request) {
		var closedChan = w.(http.CloseNotifier).CloseNotify()

		switch r.Method {
		case "PUT": // caller supplies an address to reserve for a container
			ident, ipStr, err := parseURLWithIP(r.URL.Path)
			if err != nil {
				badRequest(w, err)
			} else if ip := net.ParseIP(ipStr); ip == nil {
				invalidIP(w, ipStr)
			} else if err = alloc.Claim(ident, ip, closedChan); err != nil {
				badRequest(w, fmt.Errorf("Unable to claim IP address %s: %s", ip, err))
			}
		case "GET": // caller requests one address for a container
			ident, err := parseURL(r.URL.Path)
			if err != nil {
				badRequest(w, err)
			} else if newAddr := alloc.GetFor(ident, closedChan); newAddr != nil {
				fmt.Fprintf(w, "%s/%d", newAddr, alloc.universeLen)
			} else {
				badRequest(w, fmt.Errorf("Allocator shutting down"))
			}
		case "DELETE": // opposite of PUT for one specific address or all addresses
			ident, ipStr, err := parseURLWithIP(r.URL.Path)
			if err != nil {
				badRequest(w, err)
			} else if ipStr == "*" {
				alloc.ContainerDied(ident)
			} else if ip := net.ParseIP(ipStr); ip == nil {
				invalidIP(w, ipStr)
			} else if err = alloc.Free(ident, ip); err != nil {
				badRequest(w, err)
			}
		default:
			http.Error(w, "Verb not handled", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/tombstone-self", func(w http.ResponseWriter, r *http.Request) {
		alloc.Shutdown()
	})
	mux.HandleFunc("/peer/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "DELETE":
			ident, err := parseURL(r.URL.Path)
			if err != nil {
				badRequest(w, err)
				return
			}

			if err := alloc.TombstonePeer(ident); err != nil {
				badRequest(w, err)
				return
			}

			w.WriteHeader(204)
		default:
			http.Error(w, "Verb not handled", http.StatusBadRequest)
		}
	})
}
