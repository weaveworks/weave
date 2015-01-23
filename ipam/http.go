package ipam

import (
	"errors"
	"fmt"
	. "github.com/zettio/weave/common"
	"io"
	"log"
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

func httpErrorAndLog(level *log.Logger, w http.ResponseWriter, msg string,
	status int, logmsg string, logargs ...interface{}) {
	http.Error(w, msg, status)
	level.Printf(logmsg, logargs...)
}

func (alloc *Allocator) HandleHttp() {
	http.HandleFunc("/ip/", func(w http.ResponseWriter, r *http.Request) {
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
