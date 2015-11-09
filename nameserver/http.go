package nameserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/miekg/dns"

	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/net/address"
)

const (
	// Maximum message size allowed from peer.
	maxMessageSize = 512

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var websocketsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// observation request
type ObserveRequest struct {
	Name string
}

// updates message
type ObserveUpdate struct {
	Addresses []address.Address
}

func (n *Nameserver) badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
	n.infof("%v", err)
}

func (n *Nameserver) HandleHTTP(router *mux.Router, dockerCli *docker.Client) {
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

		if !dns.IsSubDomain(n.domain, hostname) {
			n.infof("Ignoring registration %s %s %s (not a subdomain of %s)", hostname, ipStr, container, n.domain)
			return
		}

		if err := n.AddEntry(hostname, container, n.ourName, ip); err != nil {
			n.badRequest(w, fmt.Errorf("Unable to add entry: %v", err))
			return
		}

		if r.FormValue("check-alive") == "true" && dockerCli != nil && dockerCli.IsContainerNotRunning(container) {
			n.infof("container '%s' is not running: removing", container)
			if err := n.Delete(hostname, container, ipStr, ip); err != nil {
				n.infof("failed to remove: %v", err)
			}
		}

		w.WriteHeader(204)
	})

	router.Path("/name/ws").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.debugf("[websocket][%s] trying to upgrade connection", r.RemoteAddr)
		ws, err := websocketsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			n.errorf("[websocket][%s] upgrade failed: %s", r.RemoteAddr, err)
			return
		}
		defer ws.Close()

		// write writes a message with the given message type and payload.
		send := func(mt int, payload []byte) error {
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			return ws.WriteMessage(mt, payload)
		}

		sendAddresses := func(as []address.Address) error {
			update := ObserveUpdate{Addresses: as}
			updateJSON, err := json.Marshal(update)
			if err != nil {
				n.errorf("[websocket][%s] encoding update mesage: %s", r.RemoteAddr, err)
				return err
			}
			n.debugf("[websocket][%s] sending '%s'", r.RemoteAddr, updateJSON)
			if err := send(websocket.TextMessage, updateJSON); err != nil {
				n.errorf("[websocket][%s] sending update mesage: %s", r.RemoteAddr, err)
				return err
			}
			return nil
		}

		// wait (for a reasonable time) for an observation request
		ws.SetReadLimit(maxMessageSize)
		ws.SetReadDeadline(time.Now().Add(pongWait))
		ws.SetPongHandler(func(string) error {
			n.debugf("[websocket][%s] PONG", r.RemoteAddr)
			ws.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				n.errorf("[websocket][%s] reading mesage: %s", r.RemoteAddr, err)
				return
			}

			// decode the request
			var m ObserveRequest
			if err := json.Unmarshal(message, &m); err == io.EOF {
				n.debugf("[websocket][%s] EOF", r.RemoteAddr)
				send(websocket.CloseMessage, []byte{})
				return
			} else if err != nil {
				n.errorf("[websocket][%s] could not decode watch request: %s", r.RemoteAddr, err)
			}

			// we do not let observe names that do not currently exist
			fullName := fqdnWithDomain(m.Name, n.domain)
			addrs := n.Lookup(fullName)
			if len(addrs) == 0 {
				n.errorf("[websocket][%s] cannot observe '%s': it does not exist", r.RemoteAddr, fullName)
				send(websocket.CloseMessage, []byte{})
				return
			}
			if err := sendAddresses(addrs); err != nil {
				return
			}

			// create an observer
			n.debugf("[websocket][%s] installing observer for %s", r.RemoteAddr, m.Name)
			updates, err := n.Observe(m.Name, r.RemoteAddr)
			if err != nil {
				n.errorf("[websocket][%s] could not install observer: %s", r.RemoteAddr, err)
				send(websocket.CloseMessage, []byte{})
				return
			}
			defer n.Forget(m.Name, r.RemoteAddr)

			// loop waiting for updates for than name, and forwarding those updates to the client
			n.debugf("[websocket][%s] waiting for %s (%d secs ping interval)", r.RemoteAddr, m.Name, pingPeriod/time.Second)
			ticker := time.NewTicker(pingPeriod)
			defer ticker.Stop()
			for {
				select {
				case addresses, ok := <-updates:
					if !ok {
						n.debugf("[websocket][%s] closing connection: name disappeared", r.RemoteAddr)
						send(websocket.CloseMessage, []byte{})
						return
					}
					if err := sendAddresses(addresses); err != nil {
						return
					}
				case <-ticker.C:
					n.debugf("[websocket][%s] PING", r.RemoteAddr)
					if err := send(websocket.PingMessage, []byte{}); err != nil {
						n.errorf("[websocket][%s] when sending PING: %s", r.RemoteAddr, err)
						return
					}
				}
			}
		}
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
