package proxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

const (
	defaultCaFile   = "ca.pem"
	defaultKeyFile  = "key.pem"
	defaultCertFile = "cert.pem"
	dockerSock      = "/var/run/docker.sock"
	dockerSockUnix  = "unix://" + dockerSock
)

var (
	containerCreateRegexp = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/create$")
	containerStartRegexp  = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/(re)?start$")
	execCreateRegexp      = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/exec$")
)

type Config struct {
	ListenAddrs   []string
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

	client, err := docker.NewClient(dockerSockUnix)
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
	return net.Dial("unix", dockerSock)
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

func (proxy *Proxy) ListenAndServe() {
	listeners := []net.Listener{}
	addrs := []string{}
	for _, addr := range proxy.ListenAddrs {
		listener, normalisedAddr, err := proxy.listen(addr)
		if err != nil {
			Error.Fatalf("Cannot listen on %s: %s", addr, err)
		}
		listeners = append(listeners, listener)
		addrs = append(addrs, normalisedAddr)
	}

	for _, addr := range addrs {
		Info.Println("proxy listening on", addr)
	}

	errs := make(chan error)
	for _, listener := range listeners {
		go func(listener net.Listener) {
			errs <- (&http.Server{Handler: proxy}).Serve(listener)
		}(listener)
	}
	for range listeners {
		err := <-errs
		if err != nil {
			Error.Fatalf("Serve failed: %s", err)
		}
	}
}

func copyOwnerAndPermissions(from, to string) error {
	stat, err := os.Stat(from)
	if err != nil {
		return err
	}
	if err = os.Chmod(to, stat.Mode()); err != nil {
		return err
	}

	moreStat, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}

	if err = os.Chown(to, int(moreStat.Uid), int(moreStat.Gid)); err != nil {
		return err
	}

	return nil
}

func (proxy *Proxy) listen(protoAndAddr string) (net.Listener, string, error) {
	var (
		listener    net.Listener
		err         error
		proto, addr string
	)

	if protoAddrParts := strings.SplitN(protoAndAddr, "://", 2); len(protoAddrParts) == 2 {
		proto, addr = protoAddrParts[0], protoAddrParts[1]
	} else if strings.HasPrefix(protoAndAddr, "/") {
		proto, addr = "unix", protoAndAddr
	} else {
		proto, addr = "tcp", protoAndAddr
	}

	switch proto {
	case "tcp":
		listener, err = net.Listen(proto, addr)
		if err != nil {
			return nil, "", err
		}
		if proxy.TLSConfig.enabled() {
			listener = tls.NewListener(listener, proxy.TLSConfig.Config)
		}

	case "unix":
		os.Remove(addr) // remove socket from last invocation
		listener, err = net.Listen(proto, addr)
		if err != nil {
			return nil, "", err
		}
		if err = copyOwnerAndPermissions(dockerSock, addr); err != nil {
			return nil, "", err
		}

	default:
		Error.Fatalf("Invalid protocol format: %q", proto)
	}

	return listener, fmt.Sprintf("%s://%s", proto, addr), nil
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
