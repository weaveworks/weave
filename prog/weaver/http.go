package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam"
	"github.com/weaveworks/weave/nameserver"
	"github.com/weaveworks/weave/net/address"
	weave "github.com/weaveworks/weave/router"
	"net"
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
	"printState": func(enabled bool) string {
		if enabled {
			return "enabled"
		}
		return "disabled"
	},
	"connectionType": func(directPeers []string, address string) string {
		addressHost, _, _ := net.SplitHostPort(address)
		for _, directPeer := range directPeers {
			_, _, err := net.SplitHostPort(directPeer)
			if err == nil && directPeer == address || directPeer == addressHost {
				return "direct"
			}
		}
		return "discovered"
	},
	"trimSuffix": strings.TrimSuffix,
})

// Strip escaped newlines from template
func escape(template string) string {
	return strings.Replace(template, "\\\n", "", -1)
}

// Define a named template panicking on error
func defTemplate(name string, text string) *template.Template {
	return template.Must(rootTemplate.New(name).Parse(escape(text)))
}

var statusTemplate = defTemplate("status", `\
       Service: router
          Name: {{.Router.Name}}
      NickName: {{.Router.NickName}}
    Encryption: {{printState .Router.Encryption}}
 PeerDiscovery: {{printState .Router.PeerDiscovery}}
         Peers: {{len .Router.Peers}}
   DirectPeers: {{len .Router.ConnectionMaker.DirectPeers}}
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
       Address: {{.DNS.Address}}
           TTL: {{.DNS.TTL}}
       Entries: {{countDNSEntries .DNS.Entries}}
{{end}}\
`)

var peersTemplate = defTemplate("peers", `\
{{range .Router.Peers}}\
{{.Name}}({{.NickName}})
{{range .Connections}}\
   {{if .Outbound}}->{{else}}<-{{end}} \
{{printf "%-21v" .Address}} \
{{$nameNickName := printf "%v(%v)" .Name .NickName}}\
{{printf "%-32v" $nameNickName}} \
{{if .Established}}established{{else}}unestablished{{end}}
{{end}}\
{{end}}\
`)

var connectionsTemplate = defTemplate("connectionsTemplate", `\
{{$directPeers := .Router.ConnectionMaker.DirectPeers}}\
{{$ourself := .Router.Name}}\
{{range .Router.Peers}}\
{{if eq .Name $ourself}}\
{{range .Connections}}\
{{if .Outbound}}->{{else}}<-{{end}} \
{{printf "%-21v" .Address}} \
{{$nameNickName := printf "%v(%v)" .Name .NickName}}\
{{printf "%-32v" $nameNickName}} \
{{$connectionType := connectionType $directPeers .Address}}\
{{printf "%-10v" $connectionType}} \
{{if .Established}}established{{else}}unestablished{{end}}
{{end}}\
{{end}}\
{{end}}\
{{range .Router.ConnectionMaker.Reconnects}}\
-> \
{{printf "%-21v" .Address}} \
                                 \
{{$connectionType := connectionType $directPeers .Address}}\
{{printf "%-10v" $connectionType}} \
{{if .Attempting}}\
{{if .LastError}}\
retrying({{.LastError}})
{{else}}\
connecting
{{end}}\
{{else}}\
failed({{.LastError}}), retry: {{.TryAfter}}
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
	Router *weave.Status      `json:"Router,omitempty"`
	IPAM   *ipam.Status       `json:"IPAM,omitempty"`
	DNS    *nameserver.Status `json:"DNS,omitempty"`
}

func NewWeaveStatus(
	router *weave.Router,
	allocator *ipam.Allocator,
	defaultSubnet address.CIDR,
	ns *nameserver.Nameserver,
	dnsserver *nameserver.DNSServer) WeaveStatus {

	return WeaveStatus{
		weave.NewStatus(router),
		ipam.NewStatus(allocator, defaultSubnet),
		nameserver.NewStatus(ns, dnsserver)}

}

func HandleHTTP(muxRouter *mux.Router,
	router *weave.Router,
	allocator *ipam.Allocator,
	defaultSubnet address.CIDR,
	ns *nameserver.Nameserver,
	dnsserver *nameserver.DNSServer) {

	muxRouter.Methods("GET").Path("/report").Headers("Accept", "application/json").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			weaveStatus := NewWeaveStatus(router, allocator, defaultSubnet, ns, dnsserver)
			json, _ := json.MarshalIndent(weaveStatus, "", "    ")
			w.Header().Set("Content-Type", "application/json")
			w.Write(json)
		})

	muxRouter.Methods("GET").Path("/report").Queries("format", "{format}").HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			format := mux.Vars(r)["format"]

			formatTemplate, err := template.New("format").Parse(format)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}

			weaveStatus := NewWeaveStatus(router, allocator, defaultSubnet, ns, dnsserver)
			formatTemplate.Execute(w, weaveStatus)
		})

	defHandler := func(path string, template *template.Template) {
		muxRouter.Methods("GET").Path(path).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				weaveStatus := NewWeaveStatus(router, allocator, defaultSubnet, ns, dnsserver)

				err := template.Execute(w, weaveStatus)
				if err != nil {
					Log.Error(err)
					return
				}
			})
	}

	defHandler("/status", statusTemplate)
	defHandler("/status/peers", peersTemplate)
	defHandler("/status/connections", connectionsTemplate)
	defHandler("/status/dns", dnsEntriesTemplate)

}
