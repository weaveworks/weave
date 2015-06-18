package proxy

import (
	"crypto/tls"
	"net"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/homedir"
	"github.com/docker/docker/pkg/tlsconfig"
)

type TLSConfig struct {
	Enabled, Verify bool
	tlsconfig.Options
	server *tls.Config
	client *tls.Config
}

func (c *TLSConfig) enabled() bool {
	if c == nil {
		return false
	}
	return c.Enabled || c.Verify
}

func (c *TLSConfig) loadCerts() error {
	if !c.enabled() {
		return nil
	}

	dockerCertPath := os.Getenv("DOCKER_CERT_PATH")
	if dockerCertPath == "" {
		dockerCertPath = filepath.Join(homedir.Get(), ".docker")
	}

	if c.CAFile == "" {
		c.CAFile = filepath.Join(dockerCertPath, defaultCaFile)
	}
	if c.CertFile == "" {
		c.CertFile = filepath.Join(dockerCertPath, defaultCertFile)
	}
	if c.KeyFile == "" {
		c.KeyFile = filepath.Join(dockerCertPath, defaultKeyFile)
	}

	var err error
	c.InsecureSkipVerify = !c.Verify
	if c.Verify {
		c.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if c.client, err = tlsconfig.Client(c.Options); err != nil {
		return err
	}

	c.server, err = tlsconfig.Server(c.Options)
	return err
}

func (c *TLSConfig) Dial(scheme, targetAddr string) (net.Conn, error) {
	return tls.Dial(scheme, targetAddr, c.client)
}
