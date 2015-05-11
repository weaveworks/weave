package proxy

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	. "github.com/weaveworks/weave/common"
)

const (
	RAW_STREAM = "application/vnd.docker.raw-stream"
)

type proxy struct {
	Dial func() (net.Conn, error)
}

func targetNetwork(u *url.URL) string {
	return u.Scheme
}

func targetAddress(u *url.URL) (addr string) {
	switch u.Scheme {
	case "tcp":
		addr = u.Host
	case "unix":
		addr = u.Path
	}
	return
}

func NewProxy(targetUrl string) (*proxy, error) {
	u, err := url.Parse(targetUrl)
	if err != nil {
		return nil, err
	}
	return &proxy{
		Dial: func() (net.Conn, error) {
			return net.Dial(targetNetwork(u), targetAddress(u))
		},
	}, nil
}

func isWeaveStatus(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/weave")
}

func isCreateContainer(r *http.Request) bool {
	ok, err := regexp.MatchString("/v[0-9\\.]*/containers/create", r.URL.Path)
	return err == nil && ok
}

func isStartContainer(r *http.Request) bool {
	ok, err := regexp.MatchString("^/v[0-9\\.]*/containers/[^/]*/start$", r.URL.Path)
	return err == nil && ok
}

func (proxy *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	if isWeaveStatus(r) {
		proxy.WeaveStatus(w, r)
	} else if isCreateContainer(r) {
		proxy.CreateContainer(w, r)
	} else if isStartContainer(r) {
		proxy.StartContainer(w, r)
	} else {
		proxy.ProxyRequest(w, r)
	}
}

func (proxy *proxy) WeaveStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (proxy *proxy) CreateContainer(w http.ResponseWriter, r *http.Request) {
	NewClient(proxy.Dial, NullInterceptor).ServeHTTP(w, r)
}

func (proxy *proxy) StartContainer(w http.ResponseWriter, r *http.Request) {
	NewClient(proxy.Dial, StartContainerInterceptor(proxy.transport())).ServeHTTP(w, r)
}

func (proxy *proxy) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	NewClient(proxy.Dial, NullInterceptor).ServeHTTP(w, r)
}

// Supplied so that we can use with http.Client as a Transport
func (proxy *proxy) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := proxy.transport().RoundTrip(req)
	return res, err
}

func (proxy *proxy) transport() *http.Transport {
	return &http.Transport{
		Dial: func(string, string) (conn net.Conn, err error) {
			conn, err = proxy.Dial()
			return
		},
	}
}
