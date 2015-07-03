package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	weavenet "github.com/weaveworks/weave/net"
)

func main() {
	if len(os.Args) <= 1 {
		os.Exit(0)
	}

	var (
		args         = os.Args[1:]
		notInExec    = true
		rewriteHosts = true
	)

	if args[0] == "-s" {
		notInExec = false
		rewriteHosts = false
		args = args[1:]
	}

	if args[0] == "-h" {
		rewriteHosts = false
		args = args[1:]
	}

	if notInExec {
		usr2 := make(chan os.Signal)
		signal.Notify(usr2, syscall.SIGUSR2)
		<-usr2
	}

	iface, err := weavenet.EnsureInterface("ethwe", -1)
	checkErr(err)

	if rewriteHosts {
		updateHosts(iface)
	}

	binary, err := exec.LookPath(args[0])
	checkErr(err)

	checkErr(syscall.Exec(binary, args, os.Environ()))
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Prepend the addresses to /etc/hosts, removing the docker address
func updateHosts(iface *net.Interface) {
	addrs, err := iface.Addrs()
	checkErr(err)
	if len(addrs) == 0 {
		return
	}
	hostname, err := os.Hostname()
	checkErr(err)

	hosts := parseHosts()

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
	for _, addr := range addrs {
		if addr, ok := addr.(*net.IPNet); ok {
			ip := addr.IP.String()
			hosts[ip] = append(hosts[ip], hostname)
		}
	}

	writeHosts(hosts)
}

func parseHosts() map[string][]string {
	f, err := os.Open("/etc/hosts")
	checkErr(err)
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
			ips[fields[0]] = fields[1:]
		}
	}
	checkErr(scanner.Err())
	return ips
}

func writeHosts(contents map[string][]string) {
	ips := []string{}
	for ip := range contents {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	buf := &bytes.Buffer{}
	for _, ip := range ips {
		if addrs := contents[ip]; len(addrs) > 0 {
			fmt.Fprintf(buf, "%s\t%s\n", ip, strings.Join(addrs, " "))
		}
	}
	checkErr(ioutil.WriteFile("/etc/hosts", buf.Bytes(), 644))
}
