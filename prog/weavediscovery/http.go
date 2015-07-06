package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	. "github.com/weaveworks/weave/common"
	weavenet "github.com/weaveworks/weave/net"
)

const (
	defaultHTTPPort = 6789                    // default port for the Discovery API
	defaultWeaveURL = "http://127.0.0.1:6784" // default URL for the router API
)

type WeaveClient struct {
	url url.URL
}

// Create a new Weave API client
func NewWeaveClient(u string) (*WeaveClient, error) {
	fullURL, err := url.Parse(u)
	if err != nil {
		Log.Error(err)
		return nil, err
	}

	Log.Printf("[client] Starting Weave client: API at %s", fullURL)
	cli := WeaveClient{
		url: *fullURL,
	}

	return &cli, nil
}

// Join a new peer
func (w *WeaveClient) Join(host, port string) {
	connectURL := w.url
	connectURL.Path = "/connect"

	Log.Printf("[client] Notifying about '%s' (API: '%s')", host, connectURL.String())
	_, err := http.PostForm(connectURL.String(), url.Values{"peer": {host}})
	if err != nil {
		Log.Warningf("[client] Could not notify about '%s': %s", host, err)
	}
}

// Forget about a peer
func (w *WeaveClient) Forget(host, port string) {
	forgetURL := w.url
	forgetURL.Path = "/forget"

	Log.Printf("[client] Forgetting about '%s' (API: '%s')", host, forgetURL.String())
	_, err := http.PostForm(forgetURL.String(), url.Values{"peer": {host}})
	if err != nil {
		Log.Warningf("[client] Could not forget about '%s': %s", host, err)
	}
}

//////////////////////////////////////////////////////////////////////////////////////////

type DiscoveryHTTPConfig struct {
	Manager   *DiscoveryManager
	Iface     string
	Port      int
	Wait      int
	Heartbeat time.Duration
	TTL       time.Duration
}

type DiscoveryHTTP struct {
	dm         *DiscoveryManager
	defaultHb  time.Duration
	defaultTTL time.Duration
	listener   net.Listener
}

func NewDiscoveryHTTP(config DiscoveryHTTPConfig) *DiscoveryHTTP {
	var httpIP string
	if config.Iface == "" {
		httpIP = "0.0.0.0"
	} else {
		Log.Infoln("[http] Waiting for HTTP interface", config.Iface, "to come up")
		httpIface, err := weavenet.EnsureInterface(config.Iface, config.Wait)
		if err != nil {
			Log.Fatal(err)
		}
		Log.Infoln("[http] Interface", config.Iface, "is up")

		addrs, err := httpIface.Addrs()
		if err != nil {
			Log.Fatal(err)
		}

		if len(addrs) == 0 {
			Log.Fatal("[http] No addresses on HTTP interface")
		}

		ip, _, err := net.ParseCIDR(addrs[0].String())
		if err != nil {
			Log.Fatal(err)
		}

		httpIP = ip.String()

	}
	httpAddr := net.JoinHostPort(httpIP, strconv.Itoa(config.Port))

	httpListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		Log.Fatal("[http] Unable to create HTTP listener: ", err)
	}
	Log.Infoln("[http] HTTP API listening on", httpAddr)

	return &DiscoveryHTTP{
		dm:         config.Manager,
		listener:   httpListener,
		defaultHb:  config.Heartbeat,
		defaultTTL: config.TTL,
	}
}

// Start the HTTP API
func (dh *DiscoveryHTTP) Start() {
	httpErrorAndLog := func(w http.ResponseWriter, msg string, status int, logmsg string, logargs ...interface{}) {
		http.Error(w, msg, status)
		Log.Warningf("[http] "+logmsg, logargs...)
	}

	go func() {
		muxRouter := mux.NewRouter()

		// Join a endpoint.
		// Parameters provided in the form: "url", "hb", "ttl"
		muxRouter.Methods("PUT").Path("/endpoint").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqError := func(msg string, logmsg string, logargs ...interface{}) {
				httpErrorAndLog(w, msg, http.StatusBadRequest, logmsg, logargs...)
			}

			var err error

			hb := dh.defaultHb
			hbParam := r.FormValue("hb")
			if hbParam != "" {
				if hb, err = time.ParseDuration(hbParam); err != nil {
					reqError("Invalid heartbeat", "Invalid heartbeat: %v", err)
					return
				}
			}
			if hb < 1*time.Second {
				reqError("Invalid heartbeat", "Heartbeat should be at least one second")
				return
			}

			ttl := dh.defaultTTL
			ttlParam := r.FormValue("ttl")
			if ttlParam != "" {
				if ttl, err = time.ParseDuration(ttlParam); err != nil {
					reqError("Invalid TTL", "Invalid TTL: %v", err)
					return
				}
			}
			if ttl <= hb {
				reqError("Invalid TTL", "TTL must be strictly superior to the heartbeat value")
				return
			}

			urlParam := r.FormValue("url")
			if urlParam == "" {
				reqError("Invalid URL", "Invalid URL in request: %s, %s", r.URL, r.Form)
				return
			}

			err = dh.dm.Join(urlParam, hb, ttl)
			if err != nil {
				Log.Warning(err)
			}
		})

		// Leave a endpoint.
		// Parameters provided in the form: "url"
		muxRouter.Methods("DELETE").Path("/endpoint").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqError := func(msg string, logmsg string, logargs ...interface{}) {
				httpErrorAndLog(w, msg, http.StatusBadRequest, logmsg, logargs...)
			}

			urlParam := r.FormValue("url")
			if urlParam == "" {
				reqError("Invalid URL", "Invalid URL in request: %s, %s", r.URL, r.Form)
				return
			}

			err := dh.dm.Leave(urlParam)
			if err != nil {
				Log.Warning(err)
			}
		})

		// Report status
		muxRouter.Methods("GET").Path("/status").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "weave Discovery", version)
			fmt.Fprint(w, dh.Status())
			fmt.Fprint(w, dh.dm.Status())
		})

		http.Handle("/", muxRouter)
		if err := http.Serve(dh.listener, nil); err != nil {
			Log.Fatal("[http] Unable to serve http: ", err)
		}
	}()

}

func (dh *DiscoveryHTTP) Stop() error {
	Log.Debugf("[http] Stopping HTTP API")
	dh.listener.Close()
	return nil
}

func (dh *DiscoveryHTTP) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Listen address", dh.listener.Addr())
	return buf.String()
}
