package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	MessageTypeCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dhcp_message_type_total",
			Help: "Total number of DHCP messages by type",
		},
		[]string{"type"},
	)

	MessageLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dhcp_message_latency_seconds",
			Help:    "Latency of DHCP message processing in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	AvailableLeases = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dhcp_available_leases",
			Help: "Number of available DHCP leases",
		},
	)

	ActiveLeases = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dhcp_active_leases",
			Help: "Number of active DHCP leases",
		},
	)

	LeaseRenewals = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcp_lease_renewals_total",
		Help: "The total number of DHCP lease renewals",
	})

	ArpCheckFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcp_arp_check_failures_total",
		Help: "The total number of IP collisions detected by ARP check",
	})

	LeaseExpirations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcp_lease_expirations_total",
		Help: "Number of times a lease has expired and was cleaned up",
	})

	DHCPNAKs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dhcp_naks_total",
		Help: "Number of DHCP NAK messages sent",
	})
)

func StartMetricsServer(listenAddr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()
	return nil
}
