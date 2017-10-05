package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
)

// rewrite /etc/hosts, unlinking the file (so Docker does not modify it again) but
// leaving it with valid contents...
func (proxy *Proxy) RewriteEtcHosts(hostsPath, fqdn string, ips []*net.IPNet, extraHosts []string) error {
	hostsPathDir := path.Dir(hostsPath)
	mnt := "/container"
	mntHosts := path.Join(mnt, path.Base(hostsPath))
	var buf bytes.Buffer
	writeEtcHostsContents(&buf, fqdn, ips, extraHosts)
	contents := buf.String()
	cmdLine := fmt.Sprintf("echo '%s' > %s && rm -f %s && echo '%s' > %s", contents, mntHosts, mntHosts, contents, mntHosts)
	mounts := []string{hostsPathDir + ":" + mnt}
	return proxy.runTransientContainer([]string{"sh"}, []string{"-c", cmdLine}, mounts)
}

// we assume (for compatibility with the weave script) that fqdn has a dot
// between hostname and domain. domain may be blank.
func writeEtcHostsContents(w io.Writer, fqdn string, cidrs []*net.IPNet, extraHosts []string) {
	index := strings.Index(fqdn, ".")
	name := fqdn[:index]
	var hostnames string
	if index == len(fqdn)-1 { // dot comes at the end - no domain
		hostnames = name
	} else {
		if fqdn[len(fqdn)-1] == '.' { // strip a final trailing dot
			fqdn = fqdn[:len(fqdn)-1]
		}
		hostnames = fqdn + " " + name
	}

	fmt.Fprintln(w, "# created by Weave - BEGIN")
	fmt.Fprintln(w, "# container hostname")
	for _, cidr := range cidrs {
		fmt.Fprintf(w, "%-15s %s\n", cidr.IP, hostnames)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "# static names added with --add-host")
	for _, eh := range extraHosts {
		parts := strings.SplitN(eh, ":", 2)
		if len(parts) != 2 {
			continue
		}
		fmt.Fprintf(w, "%-15s %s\n", parts[1], parts[0])
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "# default localhost entries")
	fmt.Fprintln(w, "127.0.0.1       localhost")
	fmt.Fprintln(w, "::1             ip6-localhost ip6-loopback")
	fmt.Fprintln(w, "fe00::0         ip6-localnet")
	fmt.Fprintln(w, "ff00::0         ip6-mcastprefix")
	fmt.Fprintln(w, "ff02::1         ip6-allnodes")
	fmt.Fprintln(w, "ff02::2         ip6-allrouters")
	fmt.Fprintln(w, "# created by Weave - END")
}
