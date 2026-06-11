// Package metrics defines the Prometheus instrumentation shared by the HTTP
// layer and the background workers.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "xrayapi_http_requests_total",
		Help: "HTTP requests by route group and status class.",
	}, []string{"group", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "xrayapi_http_request_duration_seconds",
		Help:    "HTTP request latency by route group.",
		Buckets: prometheus.DefBuckets,
	}, []string{"group"})

	NodeUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "xrayapi_node_up",
		Help: "1 when the node's gRPC API is reachable, 0 otherwise.",
	}, []string{"node"})

	ReconcileErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "xrayapi_reconcile_errors_total",
		Help: "Failed reconcile operations (add/remove pushes) by node.",
	}, []string{"node"})

	TrafficCollected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "xrayapi_traffic_collected_bytes_total",
		Help: "Per-user traffic bytes ingested from nodes, by direction.",
	}, []string{"node", "direction"})

	UsersSuspended = promauto.NewCounter(prometheus.CounterOpts{
		Name: "xrayapi_users_suspended_total",
		Help: "Users auto-suspended for crossing their data limit.",
	})

	UsersExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "xrayapi_users_expired_total",
		Help: "Users expired by the sweeper.",
	})

	WebhookEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "xrayapi_webhook_events_total",
		Help: "Billing webhook deliveries by action and outcome.",
	}, []string{"action", "outcome"})
)

// Handler serves the Prometheus exposition endpoint.
func Handler() http.Handler { return promhttp.Handler() }
