package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/weaveworks/mesh"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/nameserver"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
)

var rootTemplate = template.New("root").Funcs(map[string]interface{}{
	"countDNSEntries": func(entries []nameserver.EntryStatus) int {
		count := 0
		for _, entry := range entries {
			if entry.Tombstone == 0 {
				count++
			}
		}
		return count
	},
	"printList": func(list []string) string {
		if len(list) == 0 {
			return "none"
		}
		return strings.Join(list, ", ")
	},
	"printIPAMRanges": func(router weave.NetworkRouterStatus, status ipam.Status) string {
		var buffer bytes.Buffer

		type stats struct {
			ips       uint32
			nickname  string
			reachable bool
		}

		peerStats := make(map[string]*stats)

		for _, entry := range status.Entries {
			s, found := peerStats[entry.Peer]
			if !found {
				s = &stats{nickname: entry.Nickname, reachable: entry.IsKnownPeer}
				peerStats[entry.Peer] = s
			}
			s.ips += entry.Size
		}

		printOwned := func(name string, nickName string, reachable bool, ips uint32) {
			reachableStr := ""
			if !reachable {
				reachableStr = "- unreachable!"
			}
			percentageRanges := float32(ips) * 100.0 / float32(status.RangeNumIPs)

			displayName := name + "(" + nickName + ")"
			fmt.Fprintf(&buffer, "%-37v %8d IPs (%04.1f%% of total) %s\n",
				displayName, ips, percentageRanges, reachableStr)
		}

		// print the local info first
		if ourStats := peerStats[router.Name]; ourStats != nil {
			printOwned(router.Name, ourStats.nickname, true, ourStats.ips)
		}

		// and then the rest
		for peer, stats := range peerStats {
			if peer != router.Name {
				printOwned(peer, stats.nickname, stats.reachable, stats.ips)
			}
		}

		return buffer.String()
	},
	"allIPAMOwnersUnreachable": func(status ipam.Status) bool {
		for _, entry := range status.Entries {
			if entry.Size > 0 && entry.IsKnownPeer {
				return false
			}
		}
		return true
	},
	"printConnectionCounts": func(conns []mesh.LocalConnectionStatus) string {
		counts := make(map[string]int)
		for _, conn := range conns {
			counts[conn.State]++
		}
		return printCounts(counts, []string{"established", "pending", "retrying", "failed", "connecting"})
	},
	"printPeerConnectionCounts": func(peers []mesh.PeerStatus) string {
		counts := make(map[string]int)
		for _, peer := range peers {
			for _, conn := range peer.Connections {
				if conn.Established {
					counts["established"]++
				} else {
					counts["pending"]++
				}
			}
		}
		return printCounts(counts, []string{"established", "pending"})
	},
	"printState": func(enabled bool) string {
		if enabled {
			return "enabled"
		}
		return "disabled"
	},
	"trimSuffix": strings.TrimSuffix,
})

// Print counts in a specified order
func printCounts(counts map[string]int, keys []string) string {
	var stringCounts []string
	for _, key := range keys {
		if count, ok := counts[key]; ok {
			stringCounts = append(stringCounts, fmt.Sprintf("%d %s", count, key))
		}
	}
	return strings.Join(stringCounts, ", ")
}

// Strip escaped newlines from template
func escape(template string) string {
	return strings.Replace(template, "\\\n", "", -1)
}

// Define a named template panicking on error
func defTemplate(name string, text string) *template.Template {
	return template.Must(rootTemplate.New(name).Parse(escape(text)))
}

var statusTemplate = defTemplate("status", `\
        Version: {{.Version}}

        Service: router
       Protocol: {{.Router.Protocol}} \
{{if eq .Router.ProtocolMinVersion .Router.ProtocolMaxVersion}}\
{{.Router.ProtocolMaxVersion}}\
{{else}}\
{{.Router.ProtocolMinVersion}}..{{.Router.ProtocolMaxVersion}}\
{{end}}
           Name: {{.Router.Name}}({{.Router.NickName}})
     Encryption: {{printState .Router.Encryption}}
  PeerDiscovery: {{printState .Router.PeerDiscovery}}
        Targets: {{len .Router.Targets}}
    Connections: {{len .Router.Connections}}{{with printConnectionCounts .Router.Connections}} ({{.}}){{end}}
          Peers: {{len .Router.Peers}}{{with printPeerConnectionCounts .Router.Peers}} (with {{.}} connections){{end}}
 TrustedSubnets: {{printList .Router.TrustedSubnets}}
{{if .IPAM}}\

        Service: ipam
{{if .IPAM.Entries}}\
{{if allIPAMOwnersUnreachable .IPAM}}\
         Status: all IP ranges owned by unreachable peers - use 'rmpeer' if they are dead
{{else if len .IPAM.PendingAllocates}}\
         Status: waiting for IP range grant from peers
{{else}}\
         Status: ready
{{end}}\
{{else if .IPAM.Paxos}}\
         Status: awaiting consensus (quorum: {{.IPAM.Paxos.Quorum}}, known: {{.IPAM.Paxos.KnownNodes}})
{{else}}\
         Status: idle
{{end}}\
          Range: {{.IPAM.Range}}
  DefaultSubnet: {{.IPAM.DefaultSubnet}}
{{end}}\
{{if .DNS}}\

        Service: dns
         Domain: {{.DNS.Domain}}
       Upstream: {{printList .DNS.Upstream}}
            TTL: {{.DNS.TTL}}
        Entries: {{countDNSEntries .DNS.Entries}}
{{end}}\
`)

var targetsTemplate = defTemplate("targetsTemplate", `\
{{range .Router.Targets}}{{.}}
{{end}}\
`)

var connectionsTemplate = defTemplate("connectionsTemplate", `\
{{range .Router.Connections}}\
{{if .Outbound}}->{{else}}<-{{end}} {{printf "%-21v" .Address}} {{printf "%-11v" .State}} {{.Info}}
{{end}}\
`)

var peersTemplate = defTemplate("peers", `\
{{range .Router.Peers}}\
{{.Name}}({{.NickName}})
{{range .Connections}}\
   {{if .Outbound}}->{{else}}<-{{end}} {{printf "%-21v" .Address}} \
{{$nameNickName := printf "%v(%v)" .Name .NickName}}{{printf "%-37v" $nameNickName}} \
{{if .Established}}established{{else}}pending{{end}}
{{end}}\
{{end}}\
`)

var dnsEntriesTemplate = defTemplate("dnsEntries", `\
{{$domain := printf ".%v" .DNS.Domain}}\
{{range .DNS.Entries}}\
{{if eq .Tombstone 0}}\
{{$hostname := trimSuffix .Hostname $domain}}\
{{printf "%-12v" $hostname}} {{printf "%-15v" .Address}} {{printf "%12.12v" .ContainerID}} {{.Origin}}
{{end}}\
{{end}}\
`)

var ipamTemplate = defTemplate("ipamTemplate", `{{printIPAMRanges .Router .IPAM}}`)

type WeaveStatus struct {
	Version string
	Router  *weave.NetworkRouterStatus `json:"Router,omitempty"`
	IPAM    *ipam.Status               `json:"IPAM,omitempty"`
	DNS     *nameserver.Status         `json:"DNS,omitempty"`
}

func HandleHTTP(muxRouter *mux.Router, version string, router *weave.NetworkRouter, allocator *ipam.Allocator, defaultSubnet address.CIDR, ns *nameserver.Nameserver, dnsserver *nameserver.DNSServer) {
	status := func() WeaveStatus {
		return WeaveStatus{
			version,
			weave.NewNetworkRouterStatus(router),
			ipam.NewStatus(allocator, defaultSubnet),
			nameserver.NewStatus(ns, dnsserver)}
	}
	muxRouter.Methods("GET").Path("/report").Headers("Accept", "application/json").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json, err := json.MarshalIndent(status(), "", "    ")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				common.Log.Error("Error during report marshalling: ", err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(json)
		})

	muxRouter.Methods("GET").Path("/report").Queries("format", "{format}").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			funcs := template.FuncMap{
				"json": func(v interface{}) string {
					a, _ := json.Marshal(v)
					return string(a)
				},
			}
			formatTemplate, err := template.New("format").Funcs(funcs).Parse(mux.Vars(r)["format"])
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := formatTemplate.Execute(w, status()); err != nil {
				http.Error(w, "error during template execution", http.StatusInternalServerError)
				common.Log.Error(err)
			}
		})

	defHandler := func(path string, template *template.Template) {
		muxRouter.Methods("GET").Path(path).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if err := template.Execute(w, status()); err != nil {
					http.Error(w, "error during template execution", http.StatusInternalServerError)
					common.Log.Error(err)
				}
			})
	}

	defHandler("/status", statusTemplate)
	defHandler("/status/targets", targetsTemplate)
	defHandler("/status/connections", connectionsTemplate)
	defHandler("/status/peers", peersTemplate)
	defHandler("/status/dns", dnsEntriesTemplate)
	defHandler("/status/ipam", ipamTemplate)

}
