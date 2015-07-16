package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/nameserver"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
	"net/http"
	"strings"
	"text/template"
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
	"printConnectionCounts": func(conns []weave.LocalConnectionStatus) string {
		counts := make(map[string]int)
		for _, conn := range conns {
			counts[conn.State]++
		}
		return printCounts(counts, []string{"established", "pending", "retrying", "failed", "connecting"})
	},
	"printPeerConnectionCounts": func(peers []weave.PeerStatus) string {
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
         Peers: {{len .Router.Peers}}{{with printPeerConnectionCounts .Router.Peers}} (with {{.}} connections between them){{end}}
{{if .IPAM}}\

       Service: ipam
{{if .IPAM.Entries}}\
     Consensus: achieved
{{else if .IPAM.Paxos}}\
     Consensus: waiting (quorum: {{.IPAM.Paxos.Quorum}}, known: {{.IPAM.Paxos.KnownNodes}})
{{else}}\
     Consensus: deferred
{{end}}\
         Range: {{.IPAM.Range}}
 DefaultSubnet: {{.IPAM.DefaultSubnet}}
{{end}}\
{{if .DNS}}\

       Service: dns
        Domain: {{.DNS.Domain}}
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
{{$nameNickName := printf "%v(%v)" .Name .NickName}}{{printf "%-32v" $nameNickName}} \
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

type WeaveStatus struct {
	Version string
	Router  *weave.Status      `json:"Router,omitempty"`
	IPAM    *ipam.Status       `json:"IPAM,omitempty"`
	DNS     *nameserver.Status `json:"DNS,omitempty"`
}

func HandleHTTP(muxRouter *mux.Router, version string, router *weave.Router, allocator *ipam.Allocator, defaultSubnet address.CIDR, ns *nameserver.Nameserver, dnsserver *nameserver.DNSServer) {
	status := func() WeaveStatus {
		return WeaveStatus{
			version,
			weave.NewStatus(router),
			ipam.NewStatus(allocator, defaultSubnet),
			nameserver.NewStatus(ns, dnsserver)}
	}
	muxRouter.Methods("GET").Path("/report").Headers("Accept", "application/json").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json, _ := json.MarshalIndent(status(), "", "    ")
			w.Header().Set("Content-Type", "application/json")
			w.Write(json)
		})

	muxRouter.Methods("GET").Path("/report").Queries("format", "{format}").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			formatTemplate, err := template.New("format").Parse(mux.Vars(r)["format"])
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := formatTemplate.Execute(w, status()); err != nil {
				http.Error(w, "error during template execution", http.StatusInternalServerError)
				Log.Error(err)
			}
		})

	defHandler := func(path string, template *template.Template) {
		muxRouter.Methods("GET").Path(path).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if err := template.Execute(w, status()); err != nil {
					http.Error(w, "error during template execution", http.StatusInternalServerError)
					Log.Error(err)
				}
			})
	}

	defHandler("/status", statusTemplate)
	defHandler("/status/targets", targetsTemplate)
	defHandler("/status/connections", connectionsTemplate)
	defHandler("/status/peers", peersTemplate)
	defHandler("/status/dns", dnsEntriesTemplate)

}
