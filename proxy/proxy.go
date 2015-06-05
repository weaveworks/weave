package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
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
	switch {
	case containerCreateRegexp.MatchString(path):
		proxy.serveWithInterceptor(&createContainerInterceptor{proxy.client, proxy.WithDNS, proxy.dockerBridgeIP, proxy.WithIPAM}, w, r)
	case containerStartRegexp.MatchString(path):
		proxy.serveWithInterceptor(&startContainerInterceptor{proxy.client, proxy.WithDNS, proxy.WithIPAM}, w, r)
	case execCreateRegexp.MatchString(path):
		proxy.serveWithInterceptor(&createExecInterceptor{proxy.client, proxy.WithIPAM}, w, r)
	case strings.HasPrefix(path, "/status"):
		fmt.Fprintln(w, "weave proxy", proxy.Version)
		fmt.Fprintln(w, proxy.Status())
	default:
		proxy.serveWithInterceptor(&nullInterceptor{}, w, r)
	}
}

func (proxy *Proxy) serveWithInterceptor(i interceptor, w http.ResponseWriter, r *http.Request) {
	newClient(proxy.dial, i).ServeHTTP(w, r)
}

// Return status string
func (proxy *Proxy) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Listen address is", proxy.ListenAddr)
	fmt.Fprintln(&buf, "Docker address is", proxy.DockerAddr)
	switch {
	case proxy.TLSConfig.Verify:
		fmt.Fprintln(&buf, "TLS verify")
	case proxy.TLSConfig.Enabled:
		fmt.Fprintln(&buf, "TLS on")
	default:
		fmt.Fprintln(&buf, "TLS off")
	}
	fmt.Fprintln(&buf, "DNS", OnOff(proxy.WithDNS))
	fmt.Fprintln(&buf, "IPAM", OnOff(proxy.WithIPAM))
	return buf.String()
}

func (proxy *Proxy) ListenAndServe() error {
	listener, err := net.Listen("tcp", proxy.ListenAddr)
	if err != nil {
		return err
	}

	if proxy.TLSConfig.enabled() {
		listener = tls.NewListener(listener, proxy.TLSConfig.Config)
		Info.Println("TLS Enabled")
	}

	Info.Printf("Listening on %s", proxy.ListenAddr)
	Info.Printf("Proxying %s", proxy.DockerAddr)

	return (&http.Server{Handler: proxy}).Serve(listener)
}
