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
	Dial           func() (net.Conn, error)
	version        string
	client         *docker.Client
	dockerAddr     string
	dockerBridgeIP string
	listenAddr     string
	tlsConfig      *TLSConfig
	withDNS        bool
	withIPAM       bool
}

func NewProxy(version, dockerAddr, listenAddr string, withDNS, withIPAM bool, tlsConfig *TLSConfig) (*Proxy, error) {
	u, err := url.Parse(dockerAddr)
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

	if err := tlsConfig.loadCerts(); err != nil {
		Error.Fatalf("Could not configure tls for proxy: %s", err)
	}

	client, err := docker.NewClient(dockerAddr)
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
		version:        version,
		client:         client,
		dockerAddr:     dockerAddr,
		listenAddr:     listenAddr,
		dockerBridgeIP: string(dockerBridgeIP),
		tlsConfig:      tlsConfig,
		withDNS:        withDNS,
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
	case strings.HasPrefix(path, "/status"):
		fmt.Fprintln(w, "weave proxy", proxy.version)
		fmt.Fprintln(w, proxy.Status())
	default:
		proxy.serveWithInterceptor(&nullInterceptor{}, w, r)
	}
}

func (proxy *Proxy) serveWithInterceptor(i interceptor, w http.ResponseWriter, r *http.Request) {
	newClient(proxy.Dial, i).ServeHTTP(w, r)
}

// Return status string
func (proxy *Proxy) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Listen address is", proxy.listenAddr)
	fmt.Fprintln(&buf, "Docker address is", proxy.dockerAddr)
	if proxy.withDNS {
		fmt.Fprintln(&buf, "DNS on")
	} else {
		fmt.Fprintln(&buf, "DNS off")
	}
	if proxy.withIPAM {
		fmt.Fprintln(&buf, "IPAM on")
	} else {
		fmt.Fprintln(&buf, "IPAM off")
	}
	return buf.String()
}

func (proxy *Proxy) ListenAndServe() error {
	listener, err := net.Listen("tcp", proxy.listenAddr)
	if err != nil {
		return err
	}

	if proxy.tlsConfig.enabled() {
		listener = tls.NewListener(listener, proxy.tlsConfig.Config)
		Info.Println("TLS Enabled")
	}

	Info.Printf("Listening on %s", proxy.listenAddr)
	Info.Printf("Proxying %s", proxy.dockerAddr)

	return (&http.Server{Handler: proxy}).Serve(listener)
}
