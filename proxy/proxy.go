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

var (
	containerCreateRegexp = regexp.MustCompile("/v[0-9\\.]*/containers/create")
	containerStartRegexp  = regexp.MustCompile("^/v[0-9\\.]*/containers/[^/]*/(re)?start$")
	execCreateRegexp      = regexp.MustCompile("^/v[0-9\\.]*/containers/[^/]*/exec$")
)

type Proxy struct {
	Dial           func() (net.Conn, error)
	client         *docker.Client
	withDNS        bool
	dockerBridgeIP string
	withIPAM       bool
}

func NewProxy(targetURL string, withDNS, withIPAM bool) (*Proxy, error) {
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

	targetAddr := ""
	switch u.Scheme {
	case "tcp":
		targetAddr = u.Host
	case "unix":
		targetAddr = u.Path
	}

	return &Proxy{
		Dial: func() (net.Conn, error) {
			return net.Dial(u.Scheme, targetAddr)
		},
		client:         client,
		withDNS:        withDNS,
		dockerBridgeIP: string(dockerBridgeIP),
		withIPAM:       withIPAM,
	}, nil
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	path := r.URL.Path
	switch {
	case containerCreateRegexp.MatchString(path):
		proxy.serveWithInterceptor(&createContainerInterceptor{proxy.client, proxy.withDNS, proxy.dockerBridgeIP, proxy.withIPAM}, w, r)
	case containerStartRegexp.MatchString(path):
		proxy.serveWithInterceptor(&startContainerInterceptor{proxy.client, proxy.withDNS, proxy.withIPAM}, w, r)
	case execCreateRegexp.MatchString(path):
		proxy.serveWithInterceptor(&createExecInterceptor{proxy.client, proxy.withIPAM}, w, r)
	case strings.HasPrefix(path, "/weave"):
		w.WriteHeader(http.StatusOK)
	default:
		proxy.serveWithInterceptor(&nullInterceptor{}, w, r)
	}
}

func (proxy *Proxy) serveWithInterceptor(i interceptor, w http.ResponseWriter, r *http.Request) {
	newClient(proxy.Dial, i).ServeHTTP(w, r)
}
