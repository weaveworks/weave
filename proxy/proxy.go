package proxy

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
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

	ErrInvalidNetworkMode = errors.New("--net option")
	ErrWeaveCIDRNone      = errors.New("WEAVE_CIDR=none")
	ErrNoDefaultIPAM      = errors.New("--no-default-ipam option")
)

type Proxy struct {
	Config

	dial           func() (net.Conn, error)
	client         *docker.Client
	dockerBridgeIP string
}

type Config struct {
	DockerAddr    string
	ListenAddr    string
	NoDefaultIPAM bool
	TLSConfig     TLSConfig
	Version       string
	WithDNS       bool
	WithoutDNS    bool
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

	if !p.WithoutDNS {
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

func (proxy *Proxy) weaveCIDRsFromConfig(config *docker.Config, hostConfig *docker.HostConfig) ([]string, error) {
	if hostConfig != nil &&
		hostConfig.NetworkMode != "" &&
		hostConfig.NetworkMode != "bridge" {
		return nil, ErrInvalidNetworkMode
	}
	for _, e := range config.Env {
		if strings.HasPrefix(e, "WEAVE_CIDR=") {
			if e[11:] == "none" {
				return nil, ErrWeaveCIDRNone
			}
			return strings.Fields(e[11:]), nil
		}
	}
	if proxy.NoDefaultIPAM {
		return nil, ErrNoDefaultIPAM
	}
	return nil, nil
}
