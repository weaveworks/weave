package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct{ proxy *Proxy }

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) error {
	if r.StatusCode < 200 || r.StatusCode >= 300 { // Docker didn't do the start
		return nil
	}

	container, err := inspectContainerInPath(i.proxy.client, r.Request.URL.Path)
	if err != nil {
		return err
	}

	cidrs, err := i.proxy.weaveCIDRsFromConfig(container.Config, container.HostConfig)
	if err != nil {
		Log.Infof("Leaving container %s alone because %s", container.ID, err)
		return nil
	}
	Log.Infof("Attaching container %s with WEAVE_CIDR \"%s\" to weave network", container.ID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, "--or-die", container.ID)
	if _, stderr, err := callWeave(args...); err != nil {
		Log.Warningf("Attaching container %s to weave network failed: %s", container.ID, string(stderr))
		return errors.New(string(stderr))
	} else if len(stderr) > 0 {
		Log.Warningf("Attaching container %s to weave network: %s", container.ID, string(stderr))
	}

	if !i.proxy.NoRewriteHosts {
		ips, err := weaveContainerIPs(container)
		if err != nil {
			return err
		}
		if len(ips) > 0 {
			if err := updateHosts(container.HostsPath, container.Config.Hostname, ips); err != nil {
				return err
			}
		}
	}

	return i.proxy.client.KillContainer(docker.KillContainerOptions{ID: container.ID, Signal: docker.SIGUSR2})
}

func weaveContainerIPs(container *docker.Container) ([]net.IP, error) {
	stdout, stderr, err := callWeave("ps", container.ID)
	if err != nil || len(stderr) > 0 {
		return nil, errors.New(string(stderr))
	}
	if len(stdout) <= 0 {
		return nil, nil
	}

	fields := strings.Fields(string(stdout))
	if len(fields) <= 2 {
		return nil, nil
	}

	var ips []net.IP
	for _, cidr := range fields[2:] {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

func updateHosts(path, hostname string, ips []net.IP) error {
	hosts, err := parseHosts(path)
	if err != nil {
		return err
	}

	// Remove existing ips pointing to our hostname
	toRemove := []string{}
	for ip, addrs := range hosts {
		for _, addr := range addrs {
			if addr == hostname {
				toRemove = append(toRemove, ip)
				break
			}
		}
	}
	for _, ip := range toRemove {
		delete(hosts, ip)
	}

	// Add the weave ip(s)
	for _, ip := range ips {
		ipStr := ip.String()
		hosts[ipStr] = append(hosts[ipStr], hostname)
	}

	return writeHosts(path, hosts)
}

func parseHosts(path string) (map[string][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ips := map[string][]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Remove any comments
		if i := strings.IndexByte(line, '#'); i != -1 {
			line = line[:i]
		}

		fields := strings.Fields(line)
		if len(fields) > 0 {
			ips[fields[0]] = append(ips[fields[0]], fields[1:]...)
		}
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	return ips, nil
}

func writeHosts(path string, contents map[string][]string) error {
	ips := []string{}
	for ip := range contents {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "# modified by weave")
	for _, ip := range ips {
		if addrs := contents[ip]; len(addrs) > 0 {
			fmt.Fprintf(buf, "%s\t%s\n", ip, strings.Join(uniqueStrs(addrs), " "))
		}
	}
	return ioutil.WriteFile(path, buf.Bytes(), 644)
}

func uniqueStrs(s []string) []string {
	m := map[string]struct{}{}
	result := []string{}
	for _, str := range s {
		if _, ok := m[str]; !ok {
			m[str] = struct{}{}
			result = append(result, str)
		}
	}
	sort.Strings(result)
	return result
}
