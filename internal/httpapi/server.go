// Package httpapi exposes the admin REST API and the public subscription
// endpoint over net/http (chi router).
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ImErdis/xray-api/internal/config"
	"github.com/ImErdis/xray-api/internal/service"
	"github.com/ImErdis/xray-api/internal/store"
)

// Server wires the HTTP handlers to the service layer.
type Server struct {
	store          *store.Store
	nodes          *service.NodeService
	plans          *service.PlanService
	users          *service.UserService
	apiKeys        *service.APIKeyService
	billing        *service.BillingService
	log            *slog.Logger
	bootstrapKey   string
	billingSecret  string
	metricsEnabled bool
	limiter        *ipLimiter
	subCfg         subConfig
}

type subConfig struct {
	updateIntervalHours int
	profileTitle        string
}

// Deps bundles the services a Server needs.
type Deps struct {
	Store   *store.Store
	Nodes   *service.NodeService
	Plans   *service.PlanService
	Users   *service.UserService
	APIKeys *service.APIKeyService
	Billing *service.BillingService
	Log     *slog.Logger
	Config  *config.Config
}

func NewServer(d Deps) *Server {
	return &Server{
		store:          d.Store,
		nodes:          d.Nodes,
		plans:          d.Plans,
		users:          d.Users,
		apiKeys:        d.APIKeys,
		billing:        d.Billing,
		log:            d.Log,
		bootstrapKey:   d.Config.Auth.BootstrapAPIKey,
		billingSecret:  d.Config.Webhooks.BillingSecret,
		metricsEnabled: d.Config.Metrics.Enabled,
		limiter:        newIPLimiter(d.Config.RateLimit.PublicPerMinute),
		subCfg: subConfig{
			updateIntervalHours: d.Config.Subscription.ProfileUpdateIntervalHours,
			profileTitle:        d.Config.Subscription.ProfileTitle,
		},
	}
}

// HTTPServer builds an *http.Server bound to addr with sensible timeouts.
func (s *Server) HTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
