/* weavehosts: rewrite /etc/hosts as necessary to put the weave address(es) at the top */
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"sort"
	"strings"
)

func main() {
	args := os.Args
	if len(args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: weavehosts hostpath hostname cidr [cidr ...]")
		os.Exit(1)
	}

	checkErr(updateHosts(args[1], args[2], args[3:]))

	os.Exit(0)
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func updateHosts(path, hostname string, cidrs []string) error {
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
	for _, cidr := range cidrs {
		ip, _, err := net.ParseCIDR(cidr)
		checkErr(err)
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
