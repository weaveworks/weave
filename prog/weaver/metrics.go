package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rajch/weave/ipam"
	"github.com/rajch/weave/nameserver"
	"github.com/rajch/weave/net/address"
	weave "github.com/rajch/weave/router"
)

func metricsHandler(router *weave.NetworkRouter, allocator *ipam.Allocator, ns *nameserver.Nameserver, dnsserver *nameserver.DNSServer) http.Handler {
	reg := prometheus.DefaultRegisterer
	reg.MustRegister(newMetrics(router, allocator, ns, dnsserver))
	return promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{})
}

type collector struct {
	router    *weave.NetworkRouter
	allocator *ipam.Allocator
	ns        *nameserver.Nameserver
	dnsserver *nameserver.DNSServer
}

type metric struct {
	*prometheus.Desc
	Collect func(WeaveStatus, *prometheus.Desc, chan<- prometheus.Metric)
}

func desc(fqName, help string, variableLabels ...string) *prometheus.Desc {
	return prometheus.NewDesc(fqName, help, variableLabels, prometheus.Labels{})
}

func intGauge(desc *prometheus.Desc, val int, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(val), labels...)
}
func uint64Counter(desc *prometheus.Desc, val uint64, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(desc, prometheus.CounterValue, float64(val), labels...)
}

var metrics = []metric{
	{desc("weave_connections", "Number of peer-to-peer connections.", "state", "type", "encryption"),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			counts := make(map[ /*state*/ string]map[ /*type*/ string]struct{ encrypted, unencrypted int })
			for _, state := range allConnectionStates {
				counts[state] = make(map[string]struct{ encrypted, unencrypted int })
			}
			for _, conn := range s.Router.Connections {
				typeName := "unknown"
				if t, ok := conn.Attrs["name"]; ok {
					typeName = t.(string)
				}
				c := counts[conn.State][typeName]
				if e, ok := conn.Attrs["encrypted"]; ok && e.(bool) {
					c.encrypted++
				} else {
					c.unencrypted++
				}
				counts[conn.State][typeName] = c
			}
			for state, stateCounts := range counts {
				for connType, count := range stateCounts {
					ch <- intGauge(desc, count.encrypted, state, connType, "yes")
					ch <- intGauge(desc, count.unencrypted, state, connType, "")
				}
			}
		}},
	{desc("weave_connection_terminations_total", "Number of peer-to-peer connections terminated."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			ch <- uint64Counter(desc, uint64(s.Router.TerminationCount))
		}},
	{desc("weave_ips", "Number of IP addresses.", "state"),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			if s.IPAM != nil {
				ch <- intGauge(desc, s.IPAM.ActiveIPs, "local-used")
			}
		}},
	{desc("weave_max_ips", "Size of IP address space used by allocator."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			if s.IPAM != nil {
				ch <- intGauge(desc, s.IPAM.RangeNumIPs)
			}
		}},
	{desc("weave_dns_entries", "Number of DNS entries."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			if s.DNS != nil {
				ch <- intGauge(desc, countDNSEntriesForPeer(s.Router.Name, s.DNS.Entries))
			}
		}},
	{desc("weave_flows", "Number of FastDP flows."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			if metrics := fastDPMetrics(s); metrics != nil {
				ch <- intGauge(desc, metrics.Flows)
			}
		}},
	{desc("weave_ipam_unreachable_count", "Number of unreachable peers."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			var count int
			for _, entry := range summariseIpamStats(s.IPAM) {
				if !entry.reachable {
					count++
				}
			}
			ch <- intGauge(desc, count)
		}},
	{desc("weave_ipam_unreachable_percentage", "Percentage of IP addresses owned  by unreachable peers."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			var totalUnreachable uint32
			for _, entry := range summariseIpamStats(s.IPAM) {
				if !entry.reachable {
					totalUnreachable += entry.ips
				}
			}
			percentage := float64(totalUnreachable) * 100.0 / float64(s.IPAM.RangeNumIPs)
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, percentage)
		}},
	{desc("weave_ipam_pending_allocates", "Number of pending allocates."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			if s.IPAM != nil {
				ch <- intGauge(desc, len(s.IPAM.PendingAllocates))
			}
		}},
	{desc("weave_ipam_pending_claims", "Number of pending claims."),
		func(s WeaveStatus, desc *prometheus.Desc, ch chan<- prometheus.Metric) {
			if s.IPAM != nil {
				ch <- intGauge(desc, len(s.IPAM.PendingClaims))
			}
		}},
}

func fastDPMetrics(s WeaveStatus) *weave.FastDPMetrics {
	if diagMap, ok := s.Router.OverlayDiagnostics.(map[string]interface{}); ok {
		if diag, ok := diagMap["fastdp"]; ok {
			if fastDPStats, ok := diag.(weave.FastDPStatus); ok {
				return fastDPStats.Metrics().(*weave.FastDPMetrics)
			}
		}
	}
	return nil
}

func newMetrics(router *weave.NetworkRouter, allocator *ipam.Allocator, ns *nameserver.Nameserver, dnsserver *nameserver.DNSServer) *collector {
	return &collector{
		router:    router,
		allocator: allocator,
		ns:        ns,
		dnsserver: dnsserver,
	}
}

func (m *collector) Collect(ch chan<- prometheus.Metric) {

	status := WeaveStatus{
		Router: weave.NewNetworkRouterStatus(m.router),
		IPAM:   ipam.NewStatus(m.allocator, address.CIDR{}),
		DNS:    nameserver.NewStatus(m.ns, m.dnsserver)}

	for _, metric := range metrics {
		metric.Collect(status, metric.Desc, ch)
	}
}

func (m *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range metrics {
		ch <- metric.Desc
	}
}
