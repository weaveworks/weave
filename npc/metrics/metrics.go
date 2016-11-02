package metrics

import (
	log "github.com/Sirupsen/logrus"
	"net/http"
	"os"
	"strconv"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	blockedConnections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "weavenpc_blocked_connections_total",
			Help: "Connection attempts blocked by policy controller.",
		},
		[]string{"protocol", "dport"},
	)
)

func gatherMetrics() {
	pipe, err := os.Open("/var/log/ulogd.pcap")
	if err != nil {
		log.Fatalf("Failed to open pcap: %v", err)
	}

	reader, err := pcapgo.NewReader(pipe)
	if err != nil {
		log.Fatalf("Failed to read pcap header: %v", err)
	}

	for {
		data, _, err := reader.ReadPacketData()
		if err != nil {
			log.Fatalf("Failed to read pcap packet: %v", err)
		}

		packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)

		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			tcp, _ := tcpLayer.(*layers.TCP)
			if tcp.SYN && !tcp.ACK { // Only plain SYN constitutes a NEW TCP connection
				blockedConnections.With(prometheus.Labels{"protocol": "tcp", "dport": strconv.Itoa(int(tcp.DstPort))}).Inc()
				continue
			}
		}

		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			udp, _ := udpLayer.(*layers.UDP)
			blockedConnections.With(prometheus.Labels{"protocol": "udp", "dport": strconv.Itoa(int(udp.DstPort))}).Inc()
			continue
		}
	}
}

func Start(addr string) error {
	if err := prometheus.Register(blockedConnections); err != nil {
		return err
	}

	http.Handle("/metrics", promhttp.Handler())

	go func() {
		log.Infof("Serving /metrics on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("Failed to bind metrics server: %v", err)
		}
	}()

	go gatherMetrics()

	return nil
}
