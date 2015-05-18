package proxy

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type Proxy struct {
	Dial           func() (net.Conn, error)
	client         *docker.Client
	withDNS        bool
	dockerBridgeIP string
}

func NewProxy(targetURL string, withDNS bool) (*Proxy, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	var dockerBridgeIP []byte
	if withDNS {
		dockerBridgeIP, err = callWeave("docker-bridge-ip")
		if err != nil {
			return nil, err
		}
	}

	client, err := docker.NewClient(targetURL)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		Dial: func() (net.Conn, error) {
			return net.Dial(targetNetwork(u), targetAddress(u))
		},
		client:         client,
		withDNS:        withDNS,
		dockerBridgeIP: string(dockerBridgeIP),
	}, nil
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

func isWeaveStatus(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/weave")
}

func isCreateContainer(r *http.Request) bool {
	ok, err := regexp.MatchString("/v[0-9\\.]*/containers/create", r.URL.Path)
	return err == nil && ok
}

func isStartContainer(r *http.Request) bool {
	ok, err := regexp.MatchString("^/v[0-9\\.]*/containers/[^/]*/(re)?start$", r.URL.Path)
	return err == nil && ok
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	switch {
	case isCreateContainer(r):
		proxy.createContainer(w, r)
	case isStartContainer(r):
		proxy.startContainer(w, r)
	case isWeaveStatus(r):
		proxy.weaveStatus(w, r)
	default:
		proxy.proxyRequest(w, r)
	}
}

func (proxy *Proxy) weaveStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (proxy *Proxy) createContainer(w http.ResponseWriter, r *http.Request) {
	newClient(proxy.Dial, &createContainerInterceptor{proxy.client, proxy.withDNS, proxy.dockerBridgeIP}).ServeHTTP(w, r)
}

func (proxy *Proxy) startContainer(w http.ResponseWriter, r *http.Request) {
	newClient(proxy.Dial, &startContainerInterceptor{proxy.client, proxy.withDNS}).ServeHTTP(w, r)
}

func (proxy *Proxy) proxyRequest(w http.ResponseWriter, r *http.Request) {
	newClient(proxy.Dial, &nullInterceptor{}).ServeHTTP(w, r)
}
