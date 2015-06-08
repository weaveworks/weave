package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"regexp"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

const (
	defaultCaFile   = "ca.pem"
	defaultKeyFile  = "key.pem"
	defaultCertFile = "cert.pem"
)

var (
	containerCreateRegexp = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/create$")
	containerStartRegexp  = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/(re)?start$")
	execCreateRegexp      = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/exec$")
)

type Proxy struct {
	Config

	dial           func() (net.Conn, error)
	client         *docker.Client
	dockerBridgeIP string
}

type Config struct {
	DockerAddr string
	ListenAddr string
	TLSConfig  TLSConfig
	Version    string
	WithDNS    bool
	WithIPAM   bool
}

func NewProxy(c Config) (*Proxy, error) {
	u, err := url.Parse(c.DockerAddr)
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

	p := &Proxy{
		Config: c,
		dial: func() (net.Conn, error) {
			return net.Dial(u.Scheme, targetAddr)
		},
	}

	if p.WithDNS {
		dockerBridgeIP, err := callWeave("docker-bridge-ip")
		if err != nil {
			return nil, err
		}
		p.dockerBridgeIP = string(dockerBridgeIP)
	}

	if err := p.TLSConfig.loadCerts(); err != nil {
		Error.Fatalf("Could not configure tls for proxy: %s", err)
	}

	p.client, err = docker.NewClient(p.DockerAddr)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	path := r.URL.Path
	var i interceptor
	switch {
	case containerCreateRegexp.MatchString(path):
		i = &createContainerInterceptor{proxy.client, proxy.dockerBridgeIP, proxy.WithDNS, proxy.WithIPAM}
	case containerStartRegexp.MatchString(path):
		i = &startContainerInterceptor{proxy.client, proxy.WithDNS, proxy.WithIPAM}
	case execCreateRegexp.MatchString(path):
		i = &createExecInterceptor{proxy.client, proxy.WithIPAM}
	default:
		i = &nullInterceptor{}
	}
	proxy.Intercept(i, w, r)
}

func (proxy *Proxy) ListenAndServe() error {
	listener, err := net.Listen("tcp", proxy.ListenAddr)
	if err != nil {
		return err
	}

	if proxy.TLSConfig.enabled() {
		listener = tls.NewListener(listener, proxy.TLSConfig.Config)
	}

	Info.Println("proxy listening")

	return (&http.Server{Handler: proxy}).Serve(listener)
}
