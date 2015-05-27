package proxy

import (
	"crypto/tls"
	"net"
)

func (proxy *Proxy) listener(addr string) (net.Listener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	if proxy.tlsConfig.enabled() {
		listener = tls.NewListener(listener, proxy.tlsConfig.Config)
	}
	return listener, nil
}
