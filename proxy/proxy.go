package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"regexp"
	"strings"

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

type Config struct {
	ListenAddr    string
	NoDefaultIPAM bool
	TLSConfig     TLSConfig
	Version       string
	WithDNS       bool
	WithoutDNS    bool
}

type Proxy struct {
	Config
	client         *docker.Client
	dockerBridgeIP string
}

func NewProxy(c Config) (*Proxy, error) {
	p := &Proxy{Config: c}

	if err := p.TLSConfig.loadCerts(); err != nil {
		Error.Fatalf("Could not configure tls for proxy: %s", err)
	}

	client, err := docker.NewClient("unix:///var/run/docker.sock")
	if err != nil {
		return nil, err
	}
	p.client = client

	if !p.WithoutDNS {
		dockerBridgeIP, err := callWeave("docker-bridge-ip")
		if err != nil {
			return nil, err
		}
		p.dockerBridgeIP = string(dockerBridgeIP)
	}

	return p, nil
}

func (proxy *Proxy) Dial() (net.Conn, error) {
	return net.Dial("unix", "/var/run/docker.sock")
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	path := r.URL.Path
	var i interceptor
	switch {
	case containerCreateRegexp.MatchString(path):
		i = &createContainerInterceptor{proxy}
	case containerStartRegexp.MatchString(path):
		i = &startContainerInterceptor{proxy}
	case execCreateRegexp.MatchString(path):
		i = &createExecInterceptor{proxy}
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

	Info.Println("proxy listening on", proxy.ListenAddr)

	return (&http.Server{Handler: proxy}).Serve(listener)
}

func (proxy *Proxy) weaveCIDRsFromConfig(config *docker.Config) ([]string, bool) {
	for _, e := range config.Env {
		if strings.HasPrefix(e, "WEAVE_CIDR=") {
			if e[11:] == "none" {
				return nil, false
			}
			return strings.Fields(e[11:]), true
		}
	}
	return nil, !proxy.NoDefaultIPAM
}
