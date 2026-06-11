package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router builds the full route tree: public endpoints (health, subscription)
// and the API-key-protected /api/v1 surface.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.requestLogger)
	r.Use(middleware.Recoverer)

	// Public.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/sub/{token}", s.handleSubscription)
	r.Get("/sub/{token}/info", s.handleSubscriptionInfo)

	// Admin API.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authMiddleware)

		r.Get("/openapi.yaml", s.handleOpenAPI)
		r.Get("/system/stats", s.handleSystemStats)

		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", s.handleListNodes)
			r.Post("/", s.handleCreateNode)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", s.handleGetNode)
				r.Patch("/", s.handleUpdateNode)
				r.Delete("/", s.handleDeleteNode)
				r.Post("/reconcile", s.handleReconcileNode)
				r.Get("/inbounds", s.handleListInbounds)
				r.Post("/inbounds", s.handleCreateInbound)
			})
		})

		r.Route("/inbounds/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetInbound)
			r.Patch("/", s.handleUpdateInbound)
			r.Delete("/", s.handleDeleteInbound)
		})

		r.Route("/plans", func(r chi.Router) {
			r.Get("/", s.handleListPlans)
			r.Post("/", s.handleCreatePlan)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", s.handleGetPlan)
				r.Patch("/", s.handleUpdatePlan)
				r.Delete("/", s.handleDeletePlan)
			})
		})

		r.Route("/users", func(r chi.Router) {
			r.Get("/", s.handleListUsers)
			r.Post("/", s.handleCreateUser)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", s.handleGetUser)
				r.Patch("/", s.handleUpdateUser)
				r.Delete("/", s.handleDeleteUser)
				r.Put("/inbounds", s.handleSetUserInbounds)
				r.Post("/suspend", s.handleSuspendUser)
				r.Post("/resume", s.handleResumeUser)
				r.Post("/reset-traffic", s.handleResetTraffic)
				r.Post("/rotate-sub-token", s.handleRotateSubToken)
				r.Get("/usage", s.handleUserUsage)
			})
		})

		r.Route("/api-keys", func(r chi.Router) {
			r.Get("/", s.handleListAPIKeys)
			r.Post("/", s.handleCreateAPIKey)
			r.Delete("/{id}", s.handleRevokeAPIKey)
		})
	})

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// requestLogger logs each request at debug/info via slog.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Debug("http",
			"method", r.Method, "path", r.URL.Path,
			"status", ww.Status(), "bytes", ww.BytesWritten())
	})
}
