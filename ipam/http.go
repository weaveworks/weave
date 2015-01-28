package ipam

import (
	"errors"
	"fmt"
	. "github.com/zettio/weave/common"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// Parse a URL of the form /xxx/<identifier>
func parseUrl(url string) (identifier string, err error) {
	parts := strings.Split(url, "/")
	if len(parts) != 3 {
		return "", errors.New("Unable to parse url: " + url)
	}
	return parts[2], nil
}

// Parse a URL of the form /xxx/<identifier>/<ip-address>
func parseUrlWithIP(url string) (identifier string, ipaddr string, err error) {
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

func (alloc *Allocator) HandleHttp() {
	http.HandleFunc("/ip/", func(w http.ResponseWriter, r *http.Request) {
		reqError := func(msg string, logmsg string, logargs ...interface{}) {
			httpErrorAndLog(Warning, w, msg, http.StatusBadRequest,
				logmsg, logargs...)
		}

		switch r.Method {
		case "PUT":
			ident, ipStr, err := parseUrlWithIP(r.URL.Path)
			if err != nil {
				reqError("Invalid request", err.Error())
				return
			}
			ip := net.ParseIP(ipStr)
			if ip == nil {
				reqError("Invalid IP", "Invalid IP in request: %s", ipStr)
				return
			}
			if err = alloc.Claim(ident, ip); err != nil {
				reqError("Invalid claim", "Unable to perform IP claim: %s", err)
				return
			}
		case "GET":
			ident, err := parseUrl(r.URL.Path)
			if err != nil {
				httpErrorAndLog(Warning, w, "Invalid request", http.StatusBadRequest, err.Error())
			} else if newAddr := alloc.AllocateFor(ident); newAddr != nil {
				io.WriteString(w, fmt.Sprintf("%s/%d", newAddr, alloc.universeLen))
			} else {
				httpErrorAndLog(
					Error, w, "Internal error", http.StatusInternalServerError,
					"No free addresses")
			}
		}
	})
}

func ListenHttp(port int, alloc *Allocator) {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintln(alloc))
	})
	alloc.HandleHttp()

	address := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(address, nil); err != nil {
		Error.Fatal("Unable to create http listener: ", err)
	}
}
