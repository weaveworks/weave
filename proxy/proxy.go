package proxy

import (
	"crypto/tls"
	"errors"
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
	containerCreateRegexp  = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/create$")
	containerStartRegexp   = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/(re)?start$")
	containerInspectRegexp = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/json$")
	execCreateRegexp       = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/[^/]*/exec$")
	execInspectRegexp      = regexp.MustCompile("^(/v[0-9\\.]*)?/exec/[^/]*/json$")

	ErrInvalidNetworkMode = errors.New("--net option")
	ErrWeaveCIDRNone      = errors.New("WEAVE_CIDR=none")
	ErrNoDefaultIPAM      = errors.New("--no-default-ipam option")
)

type Config struct {
	HostnameMatch       string
	HostnameReplacement string
	ListenAddrs         []string
	RewriteInspect      bool
	NoDefaultIPAM       bool
	NoRewriteHosts      bool
	TLSConfig           TLSConfig
	Version             string
	WithDNS             bool
	WithoutDNS          bool
}

type Proxy struct {
	Config
	client              *docker.Client
	dockerBridgeIP      string
	hostnameMatchRegexp *regexp.Regexp
	weaveWaitVolume     string
}

func NewProxy(c Config) (*Proxy, error) {
	p := &Proxy{Config: c}

	if err := p.TLSConfig.loadCerts(); err != nil {
		Log.Fatalf("Could not configure tls for proxy: %s", err)
	}

	client, err := docker.NewClient(dockerSockUnix)
	if err != nil {
		return nil, err
	}
	p.client = client

	if !p.WithoutDNS {
		dockerBridgeIP, stderr, err := callWeave("docker-bridge-ip")
		if err != nil {
			return nil, fmt.Errorf(string(stderr))
		}
		p.dockerBridgeIP = string(dockerBridgeIP)
	}

	p.hostnameMatchRegexp, err = regexp.Compile(c.HostnameMatch)
	if err != nil {
		err := fmt.Errorf("Incorrect hostname match '%s': %s", c.HostnameMatch, err.Error())
		return nil, err
	}

	if err = p.findWeaveWaitVolume(); err != nil {
		return nil, err
	}

	return p, nil
}

func (proxy *Proxy) Dial() (net.Conn, error) {
	return net.Dial("unix", dockerSock)
}

func (proxy *Proxy) findWeaveWaitVolume() error {
	container, err := proxy.client.InspectContainer("weaveproxy")
	if err != nil {
		return fmt.Errorf("Could not find the weavewait volume: %s", err)
	}

	if container.Volumes == nil {
		return fmt.Errorf("Could not find the weavewait volume")
	}

	volume, ok := container.Volumes["/w"]
	if !ok {
		return fmt.Errorf("Could not find the weavewait volume")
	}

	proxy.weaveWaitVolume = volume
	return nil
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Log.Infof("%s %s", r.Method, r.URL)
	path := r.URL.Path
	var i interceptor
	switch {
	case containerCreateRegexp.MatchString(path):
		i = &createContainerInterceptor{proxy}
	case containerStartRegexp.MatchString(path):
		i = &startContainerInterceptor{proxy}
	case containerInspectRegexp.MatchString(path):
		i = &inspectContainerInterceptor{proxy}
	case execCreateRegexp.MatchString(path):
		i = &createExecInterceptor{proxy}
	case execInspectRegexp.MatchString(path):
		i = &inspectExecInterceptor{proxy}
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
			Log.Fatalf("Cannot listen on %s: %s", addr, err)
		}
		listeners = append(listeners, listener)
		addrs = append(addrs, normalisedAddr)
	}

	for _, addr := range addrs {
		Log.Infoln("proxy listening on", addr)
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
			Log.Fatalf("Serve failed: %s", err)
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
		Log.Fatalf("Invalid protocol format: %q", proto)
	}

	return listener, fmt.Sprintf("%s://%s", proto, addr), nil
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
