package httpapi

import (
	"io"
	"net/http"

	"github.com/ImErdis/xray-api/internal/metrics"
	"github.com/ImErdis/xray-api/internal/service"
)

const maxWebhookBody = 1 << 20 // 1 MiB

// handleBillingWebhook receives signed billing events. The raw body's
// HMAC-SHA256 (hex) must arrive in X-Webhook-Signature. 404 while no secret is
// configured so the endpoint isn't probeable.
func (s *Server) handleBillingWebhook(w http.ResponseWriter, r *http.Request) {
	if s.billingSecret == "" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil || len(body) > maxWebhookBody {
		badRequest(w, "unreadable or oversized body")
		return
	}
	if !service.VerifySignature(s.billingSecret, body, r.Header.Get("X-Webhook-Signature")) {
		metrics.WebhookEvents.WithLabelValues("unknown", "bad_signature").Inc()
		writeJSON(w, http.StatusUnauthorized, errorBody{
			Error: errorDetail{Code: "invalid_signature", Message: "signature verification failed"},
		})
		return
	}

	res, err := s.billing.Process(r.Context(), body)
	if err != nil {
		metrics.WebhookEvents.WithLabelValues("unknown", "error").Inc()
		s.log.Warn("billing webhook failed", "err", err)
		writeError(w, err)
		return
	}

	outcome := "processed"
	if res.Duplicate {
		outcome = "duplicate"
	}
	metrics.WebhookEvents.WithLabelValues(res.Action, outcome).Inc()
	s.log.Info("billing webhook", "action", res.Action, "outcome", outcome)

	resp := map[string]any{"action": res.Action, "duplicate": res.Duplicate}
	if res.User != nil {
		resp["user"] = s.userResponse(res.User)
	}
	writeJSON(w, http.StatusOK, resp)
}
