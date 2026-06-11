package httpapi

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ImErdis/xray-api/internal/domain"
)

// handleSubscription serves the public, token-authenticated subscription. The
// format is chosen by ?format=v2ray|clash, else sniffed from the User-Agent
// (clients like Clash Verge identify themselves). It keeps serving even for
// suspended/expired users so apps don't error and the userinfo bar can show
// the quota/expiry state.
func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sub, err := s.users.SubscriptionByToken(r.Context(), token)
	if err != nil {
		// Avoid leaking which tokens exist; 404 for any lookup failure.
		writeError(w, err)
		return
	}

	s.setSubHeaders(w, sub.UserInfoHeader(), sub.User)

	format := r.URL.Query().Get("format")
	if format == "" {
		format = sniffFormat(r.UserAgent())
	}

	switch format {
	case "clash":
		body, err := sub.RenderClash()
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	default:
		body, err := sub.RenderV2Ray()
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}

func (s *Server) setSubHeaders(w http.ResponseWriter, userinfo string, u *domain.User) {
	w.Header().Set("Subscription-Userinfo", userinfo)
	if s.subCfg.updateIntervalHours > 0 {
		w.Header().Set("Profile-Update-Interval", strconv.Itoa(s.subCfg.updateIntervalHours))
	}
	title := s.subCfg.profileTitle
	if title == "" {
		title = u.Email
	}
	w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
	w.Header().Set("Content-Disposition", `inline; filename="`+u.Email+`"`)
}

// handleSubscriptionInfo returns JSON usage/status for the token — useful for
// building a customer portal without exposing the admin API.
func (s *Server) handleSubscriptionInfo(w http.ResponseWriter, r *http.Request) {
	sub, err := s.users.SubscriptionByToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, err)
		return
	}
	u := sub.User
	var total, expire int64
	if u.DataLimitBytes != nil {
		total = *u.DataLimitBytes
	}
	if u.ExpiresAt != nil {
		expire = u.ExpiresAt.Unix()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":    u.Email,
		"status":   u.Status,
		"upload":   u.UsedUploadBytes,
		"download": u.UsedDownloadBytes,
		"total":    total,
		"expire":   expire,
	})
}

func sniffFormat(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") || strings.Contains(ua, "meta"):
		return "clash"
	default:
		return "v2ray"
	}
}
